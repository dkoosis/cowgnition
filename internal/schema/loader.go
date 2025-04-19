// Package schema provides functionality for loading, validating, and error reporting
// against JSON Schema definitions, with specialized support for the Model Context
// Protocol (MCP). It handles schema acquisition from multiple sources (embedded,
// local file, or remote URL), caching, conditional requests with ETags, and detailed
// error reporting with human-friendly diagnostics. This package acts as the validation
// foundation for ensuring messages conform to the MCP specification, enabling robust
// protocol compliance while providing clear feedback for troubleshooting.
package schema

// file: internal/schema/loader.go

import (
	"context"
	"fmt"
	"io"
	"net/http" // Required for URL parsing
	"os"
	"path/filepath"
	"strings" // Required for strings.HasPrefix

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config" // Required for SchemaConfig
	// Ensure logging is imported if v.logger is used directly, or passed if needed.
)

// --- Constants ---
const (
	defaultSchemaBaseURL = "https://raw.githubusercontent.com/modelcontextprotocol/specification/main/schema/"
	schemaFilePrefix     = "mcp-schema-"
	schemaFileSuffix     = ".json"
)

// loadSchemaData orchestrates loading schema data based on configuration and target version.
// It prioritizes OverrideSource, then Cache, then Network Fetch.
// Returns the schema data, the source it was loaded from (for logging), and error.
// Assumes validator lock is held by caller if modifying shared state (like ETag).
func (v *SchemaValidator) loadSchemaData(ctx context.Context, cfg config.SchemaConfig, targetVersion string) (data []byte, sourceInfo string, err error) {
	v.logger.Info("🔍 Starting schema loading process...")

	// 1. Handle OverrideSource
	if cfg.OverrideSource != "" {
		v.logger.Info("🔄 Using override schema source.", "source", cfg.OverrideSource)
		if strings.HasPrefix(cfg.OverrideSource, "http://") || strings.HasPrefix(cfg.OverrideSource, "https://") {
			// Fetch from override URL (no caching logic applied here)
			data, _, err = v.fetchSchemaFromURL(ctx, cfg.OverrideSource, false) // Pass 'false' for useCacheHeaders
			sourceInfo = fmt.Sprintf("override_url: %s", cfg.OverrideSource)
		} else {
			// Load from override file path
			data, err = v.loadSchemaFromFile(cfg.OverrideSource)
			sourceInfo = fmt.Sprintf("override_file: %s", cfg.OverrideSource)
		}
		if err != nil {
			return nil, sourceInfo, errors.Wrapf(err, "failed to load from override source: %s", cfg.OverrideSource)
		}
		v.logger.Info("✅ Successfully loaded schema from override source.", "source", cfg.OverrideSource, "size_bytes", len(data))
		return data, sourceInfo, nil
	}

	// 2. Determine Target Paths/URLs based on targetVersion
	if targetVersion == "" {
		return nil, "config", errors.New("Target schema version cannot be empty when override source is not used")
	}

	cacheDir, err := expandPath(cfg.CacheDir) // Expand ~
	if err != nil {
		v.logger.Warn("⚠️ Invalid cache directory path, caching disabled.", "path", cfg.CacheDir, "error", err)
		cacheDir = "" // Disable caching if path is bad
	}

	targetCacheFilename := schemaFilePrefix + targetVersion + schemaFileSuffix
	targetCachePath := ""
	if cacheDir != "" {
		targetCachePath = filepath.Join(cacheDir, targetCacheFilename)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultSchemaBaseURL
	}
	// Ensure baseURL ends with a slash
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	targetFetchURL := baseURL + targetVersion + "/schema.json"

	v.logger.Info("🎯 Targeting schema version.",
		"version", targetVersion,
		"cachePath", targetCachePath,
		"fetchURL", targetFetchURL,
	)

	// 3. Try Loading from Cache
	var cachedData []byte
	var cacheErr error
	if targetCachePath != "" {
		cachedData, cacheErr = v.loadSchemaFromFile(targetCachePath)
		if cacheErr == nil {
			v.logger.Info("✅ Found and loaded schema from local cache.", "path", targetCachePath, "size_bytes", len(cachedData))
			// If cache is found, we still might check the network with caching headers.
		} else {
			v.logger.Info("ℹ️ Schema not found in cache or cache read failed.", "path", targetCachePath, "error", cacheErr)
			// Continue to fetch, cacheErr is not fatal here.
		}
	} else {
		v.logger.Info("ℹ️ Cache directory not configured or invalid, skipping cache load.")
	}

	// 4. Fetch from Network (Conditional)
	v.logger.Info("🌐 Attempting to fetch schema from network.", "url", targetFetchURL)
	fetchedData, httpStatus, fetchErr := v.fetchSchemaFromURL(ctx, targetFetchURL, true) // Pass 'true' to use caching headers

	if fetchErr != nil {
		v.logger.Warn("❌ Network fetch failed.", "url", targetFetchURL, "error", fetchErr)
		// If fetch fails, rely on cache if it was loaded successfully
		if cacheErr == nil && cachedData != nil {
			v.logger.Warn("⚠️ Using potentially stale cached schema due to network fetch failure.", "cachePath", targetCachePath)
			return cachedData, fmt.Sprintf("stale_cache: %s", targetCachePath), nil // Return cached data, nil error (but log warning)
		}
		// If fetch failed AND cache wasn't available/readable, return the fetch error
		return nil, fmt.Sprintf("fetch_error: %s", targetFetchURL), errors.Wrapf(fetchErr, "failed to fetch schema and no valid cache available for version %s", targetVersion)
	}

	// Handle HTTP Status
	switch httpStatus {
	case http.StatusOK:
		v.logger.Info("✅ Successfully fetched schema from network (HTTP 200).", "url", targetFetchURL, "size_bytes", len(fetchedData))
		// Save the newly fetched data to cache
		if targetCachePath != "" {
			v.tryCacheSchemaToFile(targetCachePath, fetchedData) // Log caching attempt details
		}
		return fetchedData, fmt.Sprintf("network_fetch: %s", targetFetchURL), nil // Return fetched data
	case http.StatusNotModified:
		v.logger.Info("🔄 Schema not modified on server (HTTP 304).", "url", targetFetchURL)
		// Use cached data if it was loaded successfully
		if cacheErr == nil && cachedData != nil {
			v.logger.Info("✅ Using up-to-date cached schema.", "cachePath", targetCachePath)
			return cachedData, fmt.Sprintf("cache_validated: %s", targetCachePath), nil
		}
		// If we got 304 but cache failed to load earlier, this is an error state
		return nil, fmt.Sprintf("cache_error: %s", targetCachePath), NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Schema not modified (HTTP 304) but failed to load from cache file: %s", targetCachePath),
			cacheErr, // Include original cache read error
		).WithContext("path", targetCachePath)
	default:
		// Should not happen if fetchSchemaFromURL is correct, but handle defensively
		v.logger.Error("❌ Unexpected HTTP status from fetch.", "url", targetFetchURL, "status", httpStatus)
		// Attempt to use cache as a last resort
		if cacheErr == nil && cachedData != nil {
			v.logger.Warn("⚠️ Using potentially stale cached schema due to unexpected HTTP status.", "cachePath", targetCachePath, "httpStatus", httpStatus)
			return cachedData, fmt.Sprintf("stale_cache: %s", targetCachePath), nil
		}
		// Otherwise, fail
		return nil, fmt.Sprintf("fetch_error: %s", targetFetchURL), NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("unexpected HTTP status %d fetching schema version %s", httpStatus, targetVersion),
			nil,
		).WithContext("url", targetFetchURL).WithContext("statusCode", httpStatus)
	}
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil // No expansion needed
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get home directory for path expansion")
	}
	return filepath.Join(homeDir, path[1:]), nil
}

// loadSchemaFromFile loads schema from a local file path.
// Does not require lock.
func (v *SchemaValidator) loadSchemaFromFile(filePath string) ([]byte, error) {
	absPath, err := filepath.Abs(filePath) // Get absolute path for clarity
	if err != nil {
		absPath = filePath // Use original path in logs if Abs fails
	}
	v.logger.Debug("Reading schema file.", "path", absPath)
	// #nosec G304 -- Path is derived from config or cache logic, should be controlled.
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Log specific error type (NotFound vs other)
		if os.IsNotExist(err) {
			v.logger.Debug("Schema file does not exist.", "path", absPath)
		} else {
			v.logger.Warn("Error reading schema file.", "path", absPath, "error", err.Error())
		}
		// Wrap error for context, but don't classify as ValidationError yet.
		return nil, errors.Wrapf(err, "failed to read schema file: %s", absPath)
	}
	v.logger.Debug("Schema file read successfully.", "path", absPath, "size_bytes", len(data))
	return data, nil
}

// fetchSchemaFromURL fetches the schema from a URL, handling caching headers.
// Returns data, HTTP status code, and error.
// Assumes validator lock is held by caller if modifying shared ETag/LastModified state.
// Adds User-Agent header.
func (v *SchemaValidator) fetchSchemaFromURL(ctx context.Context, fetchURL string, useCacheHeaders bool) ([]byte, int, error) {
	v.logger.Debug("🌐 Preparing HTTP request for schema.", "url", fetchURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		v.logger.Error("❌ Failed to create HTTP request.", "url", fetchURL, "error", err.Error())
		return nil, 0, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to create HTTP request for schema URL",
			errors.Wrap(err, "http.NewRequestWithContext failed"),
		).WithContext("url", fetchURL)
	}

	// Set standard headers
	req.Header.Set("Accept", "application/json, text/plain, */*") // More permissive Accept
	req.Header.Set("User-Agent", "CowGnition-Schema-Loader/0.1.0 (github.com/dkoosis/cowgnition)")

	// Add caching headers IF requested AND they exist from previous successful requests.
	// Lock is assumed held by caller (Initialize).
	if useCacheHeaders {
		if v.schemaETag != "" {
			req.Header.Set("If-None-Match", v.schemaETag)
			v.logger.Debug("Using cached ETag for conditional request.", "etag", v.schemaETag)
		}
		if v.schemaLastModified != "" {
			req.Header.Set("If-Modified-Since", v.schemaLastModified)
			v.logger.Debug("Using cached Last-Modified for conditional request.", "lastModified", v.schemaLastModified)
		}
	}

	v.logger.Debug("🌐 Sending HTTP request.", "url", fetchURL, "method", req.Method)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		// Clear potentially stale cache headers on network error ONLY IF they were used.
		if useCacheHeaders {
			v.schemaETag = ""
			v.schemaLastModified = ""
		}
		v.logger.Error("❌ Network error fetching schema.", "url", fetchURL, "error", err.Error())
		return nil, 0, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to fetch schema from URL",
			errors.Wrap(err, "httpClient.Do failed"),
		).WithContext("url", fetchURL)
	}
	defer func() {
		// Drain and close body to allow connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			v.logger.Warn("⚠️ Error closing response body.", "error", closeErr.Error())
		}
	}()

	v.logger.Debug("📡 Received HTTP response.",
		"url", fetchURL,
		"status", resp.Status,
		"statusCode", resp.StatusCode,
		"etag", resp.Header.Get("ETag"),
		"lastModified", resp.Header.Get("Last-Modified"))

	// Handle non-OK statuses (except 304).
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		bodyBytes, _ := io.ReadAll(resp.Body) // Read body for context, ignore read error.
		// Clear potentially stale cache headers on server error status ONLY IF they were used.
		if useCacheHeaders {
			v.schemaETag = ""
			v.schemaLastModified = ""
		}
		v.logger.Error("❌ Schema URL returned error status.", "url", fetchURL, "status", resp.Status)
		return nil, resp.StatusCode, NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Failed to fetch schema: HTTP status %d", resp.StatusCode),
			nil, // No underlying Go error, it's an HTTP error status.
		).WithContext("url", fetchURL).
			WithContext("statusCode", resp.StatusCode).
			WithContext("responseBody", calculatePreview(bodyBytes)) // Use preview helper
	}

	// If status is 304, return immediately with no data and the status code.
	// Cache headers remain unchanged (lock assumed held by caller).
	if resp.StatusCode == http.StatusNotModified {
		v.logger.Debug("Schema not modified (HTTP 304).", "url", fetchURL)
		return nil, http.StatusNotModified, nil
	}

	// Status must be 200 OK here. Update ETag/Last-Modified from response headers.
	// Lock assumed held by caller.
	if useCacheHeaders {
		newETag := resp.Header.Get("ETag")
		newLastModified := resp.Header.Get("Last-Modified")
		if newETag != v.schemaETag || newLastModified != v.schemaLastModified {
			v.logger.Debug("📝 Updating cache headers from response.", "etag", newETag, "lastModified", newLastModified)
			v.schemaETag = newETag                 // Update validator's state.
			v.schemaLastModified = newLastModified // Update validator's state.
		}
	}

	// Read the response body.
	v.logger.Debug("📥 Reading response body.", "url", fetchURL)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		// Clear potentially stale cache headers if read fails after OK status ONLY IF they were used.
		if useCacheHeaders {
			v.schemaETag = ""
			v.schemaLastModified = ""
		}
		v.logger.Error("❌ Failed to read response body.", "url", fetchURL, "error", err.Error())
		return nil, resp.StatusCode, NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to read schema from HTTP response",
			errors.Wrap(err, "io.ReadAll failed"),
		).WithContext("url", fetchURL)
	}

	v.logger.Debug("✅ Successfully downloaded schema.", "url", fetchURL, "size_bytes", len(data))
	return data, http.StatusOK, nil
}

// tryCacheSchemaToFile attempts to write data to the specified cache file path.
// Logs info on success, warning on failure.
// Does not require lock.
func (v *SchemaValidator) tryCacheSchemaToFile(targetCachePath string, data []byte) {
	if targetCachePath == "" {
		v.logger.Debug("Cache path is empty, skipping caching.")
		return
	}
	v.logger.Info("📥 Caching schema to local file.", "path", targetCachePath)

	dir := filepath.Dir(targetCachePath)
	// Use 0750 permissions: Owner rwx, Group rx, Other ---.
	if err := os.MkdirAll(dir, 0750); err != nil {
		v.logger.Warn("⚠️ Failed to create directory for schema cache.", "path", dir, "error", err.Error())
		return
	}

	// Use 0600 permissions: Owner rw, Group ---, Other ---.
	if err := os.WriteFile(targetCachePath, data, 0600); err != nil {
		v.logger.Warn("⚠️ Failed to write schema cache file.", "path", targetCachePath, "error", err.Error())
	} else {
		v.logger.Info("✅ Successfully cached schema.", "path", targetCachePath, "size_bytes", len(data))
	}
}
