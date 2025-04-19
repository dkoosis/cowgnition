// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

// file: internal/schema/loader.go

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
)

// SchemaSource defines where to load the schema from.
// nolint:revive // Keep semantic naming consistent with package, will refactor in future.
type SchemaSource struct {
	// URL is the remote location of the schema, if applicable.
	URL string
	// FilePath is the local file path of the schema, if applicable (also used for caching URL fetches).
	FilePath string
	// Embedded is the embedded schema content, if applicable.
	Embedded []byte
}

// loadSchemaData orchestrates loading schema data from the configured source.
// It requires the main lock (`v.mu`) to be held by the caller (e.g., Initialize).
// because it modifies cache headers (`v.schemaETag`, `v.schemaLastModified`).
// nolint:gocyclo
func (v *SchemaValidator) loadSchemaData(ctx context.Context) ([]byte, error) {
	// 1. Try embedded schema first.
	if data, err := v.loadSchemaFromEmbedded(); err == nil {
		return data, nil
	} // Error is not possible currently, but good practice.

	// 2. Try local file second.
	// We load it here but might discard if URL provides newer data or indicates no change.
	var initialFileData []byte
	var initialFileErr error
	if v.source.FilePath != "" {
		initialFileData, initialFileErr = v.loadSchemaFromFile(v.source.FilePath)
		if initialFileErr != nil {
			v.logger.Warn("Failed to load schema from file initially, will try URL if configured.",
				"path", v.source.FilePath,
				"error", initialFileErr)
			// Don't return error yet, URL might succeed.
		} else {
			v.logger.Debug("Successfully loaded schema from file initially.",
				"path", v.source.FilePath,
				"size", len(initialFileData))
			// If no URL is configured, this file data is definitive.
			if v.source.URL == "" {
				// v.extractSchemaVersion(initialFileData). // Version extracted in Initialize now.
				return initialFileData, nil
			}
		}
	}

	// 3. Try URL third (if configured).
	if v.source.URL != "" {
		// Pass lock-acquired context to HTTP fetch.
		data, status, err := v.fetchSchemaFromURL(ctx)
		if err != nil {
			// If URL fetch fails completely, return the error.
			// We don't fall back to potentially stale file data in this case.
			// fetchSchemaFromURL already wraps the error and adds URL context.
			return nil, err
		}

		switch status {
		case http.StatusOK:
			// Successfully fetched new data from URL.
			v.logger.Debug("Successfully loaded schema from URL.", "url", v.source.URL, "size", len(data))
			// v.extractSchemaVersion(data). // Version extracted in Initialize now.
			v.tryCacheSchemaToFile(data) // Attempt to cache the freshly downloaded data.
			return data, nil
		case http.StatusNotModified:
			// Schema hasn't changed on the server.
			v.logger.Info("Schema not modified since last fetch (HTTP 304)", "url", v.source.URL)
			// Use the data we loaded from the file earlier (if successful).
			if initialFileErr == nil {
				v.logger.Debug("Using initially loaded file data as schema is unchanged.", "path", v.source.FilePath)
				// Ensure version is extracted from file data if we haven't done it yet.
				// if v.schemaVersion == "" {.
				// 	v.extractSchemaVersion(initialFileData). // Done in Initialize now.
				// }.
				return initialFileData, nil
			}
			// If file load failed BUT we got 304, it implies a previous successful load must have happened.
			// Signal to the caller (Initialize) to use the already compiled schemas.
			v.logger.Warn("Schema unchanged (304) but failed to load local file cache. Signaling to use previously compiled schemas if available.",
				"path", v.source.FilePath,
				"fileError", initialFileErr)
			return nil, nil // Signal "use existing compiled".
		default:
			// Should not happen if fetchSchemaFromURL is correct, but handle defensively.
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				fmt.Sprintf("unexpected HTTP status %d from URL fetch", status),
				nil,
			).WithContext("url", v.source.URL).WithContext("statusCode", status) // Added URL context here too.
		}
	}

	// 4. If only FilePath was specified and it failed initially.
	if v.source.FilePath != "" && v.source.URL == "" && initialFileErr != nil {
		// Wrap the initial file error properly if it hasn't been wrapped yet.
		var validationErr *ValidationError
		if errors.As(initialFileErr, &validationErr) {
			// If it's already a validation error, check its cause for os.IsNotExist.
			if os.IsNotExist(errors.Cause(initialFileErr)) {
				// Re-wrap with the correct code if needed.
				return nil, NewValidationError(ErrSchemaNotFound, "Schema file not found and no fallback URL configured", initialFileErr).
					WithContext("path", v.source.FilePath)
			}
			// Otherwise, return the existing validation error.
			return nil, initialFileErr
		}
		// Wrap the raw file error.
		// Use ErrSchemaNotFound if the underlying error is os.IsNotExist.
		code := ErrSchemaLoadFailed // Default code.
		msg := "Failed to load schema from file"
		if os.IsNotExist(errors.Cause(initialFileErr)) {
			code = ErrSchemaNotFound // Use the correct code.
			msg = "Schema file not found and no fallback URL configured"
		}
		return nil, NewValidationError(code, msg, initialFileErr).
			WithContext("path", v.source.FilePath)
	}

	// 5. If we get here, no source was configured or loadable.
	return nil, NewValidationError(
		ErrSchemaNotFound,
		"No valid schema source configured or loadable",
		nil,
	).WithContext("sourcesChecked", map[string]interface{}{
		"embedded": len(v.source.Embedded) > 0,
		"filePath": v.source.FilePath,
		"url":      v.source.URL,
	})
}

// loadSchemaFromEmbedded loads schema from embedded bytes.
// Does not require lock.
func (v *SchemaValidator) loadSchemaFromEmbedded() ([]byte, error) {
	if len(v.source.Embedded) > 0 {
		v.logger.Debug("Loading schema from embedded data.", "size", len(v.source.Embedded))
		// Version extraction happens later in Initialize.
		return v.source.Embedded, nil
	}
	return nil, errors.New("no embedded schema provided") // Internal signal, not user-facing error.
}

// loadSchemaFromFile loads schema from a local file path.
// Does not require lock.
func (v *SchemaValidator) loadSchemaFromFile(filePath string) ([]byte, error) {
	// #nosec G304 -- File path is determined internally or validated by caller context.
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Wrap error for context, but don't classify as ValidationError yet.
		return nil, errors.Wrapf(err, "failed to read schema file: %s", filePath)
	}
	// Version extraction happens later in Initialize.
	return data, nil
}

// fetchSchemaFromURL fetches the schema from a URL, handling caching headers.
// Returns data, HTTP status code, and error.
// It requires the main lock (`v.mu`) to be held by the caller.
// because it reads and potentially modifies cache headers (`v.schemaETag`, `v.schemaLastModified`).
func (v *SchemaValidator) fetchSchemaFromURL(ctx context.Context) ([]byte, int, error) {
	v.logger.Debug("Attempting to load schema from URL.", "url", v.source.URL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.source.URL, nil)
	if err != nil {
		// *** MODIFIED: Added WithContext ***
		return nil, 0, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to create HTTP request for schema URL",
			errors.Wrap(err, "http.NewRequestWithContext failed"),
		).WithContext("url", v.source.URL)
	}

	// Add caching headers IF THEY EXIST from previous successful requests.
	// Lock is held by caller (Initialize).
	if v.schemaETag != "" {
		req.Header.Set("If-None-Match", v.schemaETag)
		v.logger.Debug("Using cached ETag for conditional request", "etag", v.schemaETag)
	}
	if v.schemaLastModified != "" {
		req.Header.Set("If-Modified-Since", v.schemaLastModified)
		v.logger.Debug("Using cached Last-Modified for conditional request", "lastModified", v.schemaLastModified)
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		// Clear potentially stale cache headers on network error.
		v.schemaETag = ""
		v.schemaLastModified = ""
		// *** MODIFIED: Added WithContext ***
		return nil, 0, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to fetch schema from URL",
			errors.Wrap(err, "httpClient.Do failed"),
		).WithContext("url", v.source.URL)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Assuming 'v' is the SchemaValidator with a logger.
			v.logger.Warn("Error closing response body in fetchSchemaFromURL", "error", err)
		}
	}()

	// Handle non-OK statuses before reading body (except 304).
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		bodyBytes, _ := io.ReadAll(resp.Body) // Read body for context, ignore read error.
		// Clear potentially stale cache headers on server error status.
		v.schemaETag = ""
		v.schemaLastModified = ""
		// *** MODIFIED: Ensure WithContext is present ***
		return nil, resp.StatusCode, NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Failed to fetch schema: HTTP status %d", resp.StatusCode),
			nil, // No underlying Go error, it's an HTTP error status.
		).WithContext("url", v.source.URL). // Ensure URL context is added.
							WithContext("statusCode", resp.StatusCode).
							WithContext("responseBody", string(bodyBytes))
	}

	// If status is 304, return immediately with no data and the status code.
	// Cache headers remain unchanged.
	if resp.StatusCode == http.StatusNotModified {
		return nil, http.StatusNotModified, nil
	}

	// Status must be 200 OK here. Update ETag/Last-Modified from response headers.
	// Lock is held by caller (Initialize).
	newETag := resp.Header.Get("ETag")
	newLastModified := resp.Header.Get("Last-Modified")
	if newETag != v.schemaETag || newLastModified != v.schemaLastModified {
		v.logger.Debug("Received new schema HTTP cache headers", "etag", newETag, "lastModified", newLastModified)
		v.schemaETag = newETag                 // Update validator's state.
		v.schemaLastModified = newLastModified // Update validator's state.
	}

	// Read the response body.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		// Clear potentially stale cache headers if read fails after OK status.
		v.schemaETag = ""
		v.schemaLastModified = ""
		// *** MODIFIED: Added WithContext ***
		return nil, resp.StatusCode, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to read schema from HTTP response",
			errors.Wrap(err, "io.ReadAll failed"),
		).WithContext("url", v.source.URL)
	}

	return data, http.StatusOK, nil
}

// tryCacheSchemaToFile attempts to write data to the configured cache file path.
// Logs info on success, warning on failure.
// Does not require lock.
func (v *SchemaValidator) tryCacheSchemaToFile(data []byte) {
	if v.source.FilePath == "" {
		return // No cache file path configured.
	}

	dir := filepath.Dir(v.source.FilePath)
	// Use 0750 permissions: Owner rwx, Group rx, Other ---.
	if err := os.MkdirAll(dir, 0750); err != nil {
		v.logger.Warn("Failed to create directory for schema cache file",
			"path", dir,
			"error", err)
		return // Can't create directory, so can't write file.
	}

	// Use 0600 permissions: Owner rw, Group ---, Other --- (Gosec G306 fix).
	if err := os.WriteFile(v.source.FilePath, data, 0600); err != nil {
		v.logger.Warn("Failed to cache fetched schema to local file",
			"path", v.source.FilePath,
			"error", err)
	} else {
		v.logger.Info("Cached fetched schema to local file", "path", v.source.FilePath)
	}
}
