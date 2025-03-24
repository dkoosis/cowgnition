// internal/rtm/transport.go
package rtm

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Response represents a generic RTM API response.
type Response struct {
	Status string `xml:"stat,attr"`
	Error  *struct {
		Code    string `xml:"code,attr"`
		Message string `xml:"msg,attr"`
	} `xml:"err"`
}

// Constants for response status.
const (
	statusOK   = "ok"
	statusFail = "fail"
)

// APIError represents an error returned by the RTM API.
type APIError struct {
	Code    int
	Message string
}

// Error implements the error interface for APIError.
func (e APIError) Error() string {
	return fmt.Sprintf("RTM API error %d: %s", e.Code, e.Message)
}

// GetError returns error code and message from a response.
func (r Response) GetError() (string, string) {
	if r.Error == nil {
		return "", ""
	}
	return r.Error.Code, r.Error.Message
}

// callMethod calls an RTM API method with the provided parameters.
func (c *Client) callMethod(ctx context.Context, method string, params url.Values) ([]byte, error) {
	// Apply rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("callMethod: rate limit error: %w", err)
	}

	// Add required parameters
	if params == nil {
		params = url.Values{}
	}
	params.Set("method", method)
	params.Set("api_key", c.APIKey)
	params.Set("format", "rest")

	// Add authentication token if available
	if c.AuthToken != "" {
		params.Set("auth_token", c.AuthToken)
	}

	// Generate signature
	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("callMethod: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("callMethod: failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit errors (HTTP 503)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("callMethod: service temporarily unavailable due to rate limiting")
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("callMethod: failed to read response body: %w", err)
	}

	// Check for API errors
	if err := c.checkResponseForError(body); err != nil {
		return nil, err
	}

	return body, nil
}

// checkResponseForError checks if the RTM API response contains an error.
func (c *Client) checkResponseForError(response []byte) error {
	var respStruct struct {
		Stat string `xml:"stat,attr"`
		Err  struct {
			Code string `xml:"code,attr"`
			Msg  string `xml:"msg,attr"`
		} `xml:"err"`
	}

	if err := xml.Unmarshal(response, &respStruct); err != nil {
		return fmt.Errorf("checkResponseForError: failed to parse response: %w", err)
	}

	if respStruct.Stat == "fail" {
		return fmt.Errorf("checkResponseForError: RTM API error %s: %s", respStruct.Err.Code, respStruct.Err.Msg)
	}

	return nil
}

// prepareRequest prepares the request parameters by adding common fields and generating a signature.
// It adds the API key, format, authentication token (if available), and generates a signature.
func (c *Client) prepareRequest(params url.Values) url.Values {
	if params == nil {
		params = url.Values{}
	}

	// Add API key and format
	params.Set("api_key", c.APIKey)
	params.Set("format", "rest")

	// Add authentication token if available
	if c.AuthToken != "" {
		params.Set("auth_token", c.AuthToken)
	}

	// Generate signature
	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	return params
}

// createRequest creates an HTTP request based on the client's configuration.
// It supports both GET and POST methods based on the client's configuration.
func (c *Client) createRequest(ctx context.Context, params url.Values) (*http.Request, error) {
	var req *http.Request
	var err error

	if c.usePOST {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL,
			strings.NewReader(params.Encode()))
		if err != nil {
			return nil, fmt.Errorf("createRequest: error creating POST request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet,
			c.APIURL+"?"+params.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("createRequest: error creating GET request: %w", err)
		}
	}

	return req, nil
}

// sendRequest sends an HTTP request and handles common error scenarios.
// It returns the response body as a byte array or an error if the request fails.
func (c *Client) sendRequest(req *http.Request) ([]byte, error) {
	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sendRequest: error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit errors (HTTP 503)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, APIError{
			Code:    503,
			Message: "Service temporarily unavailable due to rate limiting",
		}
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sendRequest: HTTP error: status code %d (HTTP status: %d)",
			resp.StatusCode, resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sendRequest: error reading response body: %w", err)
	}

	return body, nil
}

// processResponse processes the API response and checks for API errors.
// It unmarshals the response into the provided result interface if it's not nil.
func (c *Client) processResponse(body []byte, result interface{}) error {
	// Parse response
	var respStruct Response
	if err := xml.Unmarshal(body, &respStruct); err != nil {
		return fmt.Errorf("processResponse: error parsing response: %w", err)
	}

	// Check for API errors
	if respStruct.Status == statusFail && respStruct.Error != nil {
		code, err := strconv.Atoi(respStruct.Error.Code)
		if err != nil {
			code = 0
		}
		return APIError{
			Code:    code,
			Message: respStruct.Error.Message,
		}
	}

	// Unmarshal into result if provided
	if result != nil {
		if err := xml.Unmarshal(body, result); err != nil {
			return fmt.Errorf("processResponse: error unmarshaling response: %w", err)
		}
	}

	return nil
}

// DoWithContext executes an API request with the given context, parameters and unmarshals the result.
// This is the primary method for executing API requests with context support.
func (c *Client) DoWithContext(ctx context.Context, params url.Values, result interface{}) ([]byte, error) {
	// Apply rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("DoWithContext: rate limit error: %w", err)
	}

	// Prepare request parameters
	params = c.prepareRequest(params)

	// Create request
	req, err := c.createRequest(ctx, params)
	if err != nil {
		return nil, err
	}

	// Send request and get response
	body, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	// Process response
	if err := c.processResponse(body, result); err != nil {
		return nil, err
	}

	return body, nil
}

// Do executes an API request with the given parameters and unmarshals the result.
// It uses the background context and delegates to DoWithContext.
func (c *Client) Do(params url.Values, result interface{}) ([]byte, error) {
	ctx := context.Background()
	return c.DoWithContext(ctx, params, result)
}

// createMultipartForm creates a multipart form with file content.
// It's used for file upload operations that require multipart/form-data requests.
func createMultipartForm(params url.Values, fileField, fileName string, fileContent io.Reader) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	for key, values := range params {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return nil, "", fmt.Errorf("error writing form field: %w", err)
			}
		}
	}

	// Add file
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, "", fmt.Errorf("error creating form file: %w", err)
	}
	if _, err := io.Copy(part, fileContent); err != nil {
		return nil, "", fmt.Errorf("error copying file content: %w", err)
	}

	// Close writer
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("error closing multipart writer: %w", err)
	}

	return body, writer.FormDataContentType(), nil
}

// processUploadResponse processes the response from an upload request.
// It checks for API errors and converts the XML response to a map for easier handling.
func processUploadResponse(respBody []byte) (map[string]interface{}, error) {
	// Parse response
	var respStruct Response
	if err := xml.Unmarshal(respBody, &respStruct); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	// Check for API errors
	if respStruct.Status == statusFail && respStruct.Error != nil {
		code, err := strconv.Atoi(respStruct.Error.Code)
		if err != nil {
			code = 0
		}
		return nil, APIError{
			Code:    code,
			Message: respStruct.Error.Message,
		}
	}

	// Simple XML to map conversion for the result
	result := make(map[string]interface{})
	var mapData map[string]interface{}
	if err := xml.Unmarshal(respBody, &mapData); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	// Extract relevant data
	for k, v := range mapData {
		if k != "stat" && k != "err" {
			result[k] = v
		}
	}

	return result, nil
}

// Upload uploads a file to RTM with the given parameters.
// It uses the background context and delegates to UploadWithContext.
func (c *Client) Upload(params url.Values, fileField, fileName string, fileContent io.Reader) (map[string]interface{}, error) {
	return c.UploadWithContext(context.Background(), params, fileField, fileName, fileContent)
}

// UploadWithContext uploads a file to RTM with the given parameters and context.
// It creates a multipart/form-data request with the file content and handles the response.
func (c *Client) UploadWithContext(ctx context.Context, params url.Values, fileField, fileName string, fileContent io.Reader) (map[string]interface{}, error) {
	// Apply rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("UploadWithContext: rate limit error: %w", err)
	}

	// Prepare request parameters
	params = c.prepareRequest(params)

	// Create multipart form
	body, contentType, err := createMultipartForm(params, fileField, fileName, fileContent)
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit errors (HTTP 503)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("service temporarily unavailable due to rate limiting")
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: status code %d", resp.StatusCode)
	}

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return processUploadResponse(respBody)
}
