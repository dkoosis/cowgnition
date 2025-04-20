// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// file: internal/schema/loader.go
//
// Schema loading logic:
// 1. The system first checks if a schema override URI is specified in the configuration.
// 2. If an override URI is provided:
//   - For file:// URIs: The schema is loaded from the local filesystem.
//   - For http(s):// URIs: The schema is fetched from the remote server.
//   - If loading fails with non-404 errors, retry with modified User-Agent.
//   - Error handling accounts for file not found, HTTP errors, and invalid schema content.
//
// 3. If override URI loading fails:
//   - Try standard file locations (working directory, config dirs, system-wide locations).
//   - If all external sources fail, an error is returned, signaling the caller to use the embedded schema.
//
// This file specifically handles the robust multi-source loading logic, supporting both file system
// and HTTP(S) loading with appropriate error context, fallbacks, and platform-specific path handling.
package schema

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// loadSchemaFromMultipleSources attempts to load a schema from multiple sources in priority order.
// It no longer attempts to fetch from a hardcoded official URL as a fallback.
// If override and local files fail, it returns an error to signal fallback to embedded schema.
func loadSchemaFromMultipleSources(ctx context.Context, cfg config.SchemaConfig, logger logging.Logger, httpClient *http.Client) ([]byte, string, error) {
	// 1. Try override URI from config if provided.
	if cfg.SchemaOverrideURI != "" {
		logger.Info("Attempting to load schema from override URI.", "uri", cfg.SchemaOverrideURI)
		data, err := loadSchemaFromURI(ctx, cfg.SchemaOverrideURI, logger, httpClient)
		if err == nil {
			return data, fmt.Sprintf("override: %s", cfg.SchemaOverrideURI), nil
		}
		// Log error but continue to fallbacks.
		logger.Warn("Failed to load schema from override URI, falling back to local files.",
			"uri", cfg.SchemaOverrideURI, "error", err)

		// If the error is not a 404, we might want to retry with modified settings.
		if !isNotFoundError(err) {
			// Try again with different User-Agent if it's an HTTP URL.
			if strings.HasPrefix(cfg.SchemaOverrideURI, "http") {
				modifiedClient := &http.Client{
					Timeout: httpClient.Timeout,
					Transport: &userAgentModifyingTransport{
						base:      http.DefaultTransport,
						userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
					},
				}

				logger.Info("Retrying schema fetch with modified User-Agent.", "uri", cfg.SchemaOverrideURI)
				data, retryErr := loadSchemaFromURI(ctx, cfg.SchemaOverrideURI, logger, modifiedClient)
				if retryErr == nil {
					return data, fmt.Sprintf("override (retry): %s", cfg.SchemaOverrideURI), nil
				}
				logger.Warn("Still failed to load schema from override URI with modified User-Agent.",
					"uri", cfg.SchemaOverrideURI, "error", retryErr)
			}
		}
	}

	// 2. Try standard local file locations.
	localPaths := getStandardSchemaLocations()
	for _, path := range localPaths {
		uri := "file://" + path
		logger.Debug("Trying local schema file.", "path", path)
		data, err := loadSchemaFromURI(ctx, uri, logger, httpClient)
		if err == nil {
			logger.Info("Successfully loaded schema from local file.", "path", path)
			return data, fmt.Sprintf("local file: %s", path), nil
		} else if !os.IsNotExist(errors.Cause(err)) {
			// Log errors other than "not found".
			logger.Warn("Failed to load schema from local file.", "path", path, "error", err)
		}
	}

	// 3. REMOVED: Fallback fetch from hardcoded official URL removed.

	// 4. Indicate failure to load from any external source.
	// The caller (validator.Initialize) should now use the embedded schema.
	logger.Warn("Failed to load schema from any configured/standard external source.")
	return nil, "", NewValidationError(
		ErrSchemaLoadFailed,
		"Failed to load schema from any external source (override URI or local files)",
		nil, // No specific underlying cause for *this* aggregate failure.
	)
}

// loadSchemaFromURI attempts to load schema data from a given URI,
// which can be a file path (file://...) or an HTTP(S) URL.
func loadSchemaFromURI(ctx context.Context, uri string, logger logging.Logger, httpClient *http.Client) ([]byte, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid schema override URI: %s", uri)
	}

	logger.Info("Loading schema from URI.", "uri", uri, "scheme", parsedURI.Scheme)

	switch parsedURI.Scheme {
	case "file":
		// Convert file URI path to OS-specific path.
		// Handle potential leading slash on Windows paths.
		filePath := parsedURI.Path
		if os.PathSeparator == '\\' && strings.HasPrefix(filePath, "/") {
			filePath = strings.TrimPrefix(filePath, "/")
		}
		logger.Debug("Reading schema file.", "path", filePath)
		data, err := os.ReadFile(filePath) // #nosec G304 -- URI comes from config/flag or standard paths.
		if err != nil {
			logger.Error("Failed to read schema file.", "path", filePath, "error", err)
			// Return the os error wrapped appropriately, possibly indicating ErrSchemaNotFound.
			code := ErrSchemaLoadFailed
			if os.IsNotExist(err) {
				code = ErrSchemaNotFound
			}
			return nil, NewValidationError(
				code,
				"Failed to read schema file",
				errors.Wrapf(err, "failed to read schema file: %s", filePath),
			).WithContext("uri", uri)
		}
		logger.Debug("Schema file read successfully.", "path", filePath, "size_bytes", len(data))
		return data, nil

	case "http", "https":
		logger.Debug("Fetching schema from URL.", "url", uri)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			logger.Error("Failed to create HTTP request for schema URL.", "url", uri, "error", err)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to create HTTP request for schema URL",
				errors.Wrap(err, "http.NewRequestWithContext failed"),
			).WithContext("url", uri)
		}
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("User-Agent", "CowGnition-Schema-Loader/0.1.0 (schema fetch)") // Updated agent name.

		// Use the provided httpClient which has a timeout.
		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Error("Network error fetching schema.", "url", uri, "error", err)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to fetch schema from URL",
				errors.Wrap(err, "httpClient.Do failed"),
			).WithContext("url", uri)
		}
		// Ensure body is closed.
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Warn("Error closing schema response body.", "url", uri, "error", closeErr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body) // Read body for logging context.
			code := ErrSchemaLoadFailed
			if resp.StatusCode == http.StatusNotFound {
				code = ErrSchemaNotFound
			}
			logger.Error("Schema URL returned error status.",
				"url", uri,
				"status", resp.Status,
				"statusCode", resp.StatusCode,
				"responseBody", calculatePreview(bodyBytes))
			return nil, NewValidationError(
				code,
				fmt.Sprintf("Failed to fetch schema: HTTP status %d", resp.StatusCode),
				nil,
			).WithContext("url", uri).
				WithContext("statusCode", resp.StatusCode).
				WithContext("responseBody", calculatePreview(bodyBytes))
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read response body from schema URL.", "url", uri, "error", err)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to read schema from HTTP response",
				errors.Wrap(err, "io.ReadAll failed"),
			).WithContext("url", uri)
		}
		logger.Debug("Successfully downloaded schema.", "url", uri, "size_bytes", len(data))
		return data, nil

	default:
		logger.Error("Unsupported schema URI scheme.", "uri", uri, "scheme", parsedURI.Scheme)
		return nil, NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Unsupported schema URI scheme: %s", parsedURI.Scheme),
			nil,
		).WithContext("uri", uri)
	}
}

// Custom transport to modify User-Agent.
type userAgentModifyingTransport struct {
	base      http.RoundTripper
	userAgent string
}

func (t *userAgentModifyingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context()) // Clone request to avoid modifying original.
	req2.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req2)
}

// getStandardSchemaLocations returns a list of potential locations for the schema file.
func getStandardSchemaLocations() []string {
	var locations []string

	// Current working directory.
	if cwd, err := os.Getwd(); err == nil {
		locations = append(locations, filepath.Join(cwd, "schema.json"))
		locations = append(locations, filepath.Join(cwd, "internal", "schema", "schema.json"))
	}

	// User config directory.
	if configDir, err := os.UserConfigDir(); err == nil {
		locations = append(locations, filepath.Join(configDir, "cowgnition", "schema.json"))
	}

	// Home directory.
	if homeDir, err := os.UserHomeDir(); err == nil {
		locations = append(locations, filepath.Join(homeDir, ".config", "cowgnition", "schema.json"))
		// Consider adding ~/.cowgnition/schema.json as well if desired.
	}

	// System-wide locations (less common for user apps, adjust if needed).
	locations = append(locations, "/etc/cowgnition/schema.json") // Linux example.

	// Add platform-specific paths if necessary.

	return locations
}

// isNotFoundError checks if the error is a "not found" type error.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTP 404 in ValidationError context.
	var validErr *ValidationError
	if errors.As(err, &validErr) {
		if ctx, exists := validErr.Context["statusCode"]; exists {
			if statusCode, ok := ctx.(int); ok && statusCode == http.StatusNotFound {
				return true
			}
		}
		// Check if the ValidationError itself indicates schema not found.
		if validErr.Code == ErrSchemaNotFound {
			return true
		}
	}

	// Check underlying os error for file not found.
	return os.IsNotExist(errors.Cause(err))
}
