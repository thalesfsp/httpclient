// Package httpclient provides a pre-configured HTTP client with additional features
// such as retry logic, metrics, and logging. It is designed to simplify making HTTP
// requests and handling responses, especially in applications that require robust
// error handling and retry mechanisms.
//
//nolint:lll,mnd
package httpclient

import (
	"bytes"
	"context"
	"expvar"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/thalesfsp/customerror"
	"github.com/thalesfsp/httpclient/internal/logging"
	"github.com/thalesfsp/httpclient/internal/metrics"
	"github.com/thalesfsp/httpclient/internal/shared"
	"github.com/thalesfsp/status"
	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/validation"
)

// DefaultMetricCounterLabel is the default label used for metric counters.
// It is used when initializing expvar.Int counters for tracking request metrics.
const DefaultMetricCounterLabel = "counter"

// httpRetrierRegex is a regular expression that matches HTTP status codes
// indicating that a request should be retried. Specifically, it matches:
// - 408 (Request Timeout)
// - 429 (Too Many Requests)
// - 5xx (Server Errors)
var httpRetrierRegex = regexp.MustCompile(`(?m)^(408|429|5\d\d)$`)

// singleton and once are used to implement a singleton pattern for the Client.
// This ensures that only one instance of the Client is created.
var (
	once      sync.Once
	singleton *Client
)

// Ensure that Client implements the IHTTP interface.
// This is a compile-time check to ensure that Client satisfies the interface.
// If Client does not implement all methods of IHTTP, this line will cause a compile error.
var _ IHTTP = (*Client)(nil)

// Client represents a pre-configured HTTP client with additional features.
// It wraps the standard http.Client and adds functionalities like retry logic,
// metrics tracking, and structured logging.
type Client struct {
	client *http.Client // The underlying HTTP client used to make requests.

	// Metrics counters for tracking the number of failed, retried, and successful requests.
	counterFailed  *expvar.Int `json:"-" validate:"required,gte=0"` // Counter for failed requests.
	counterRetried *expvar.Int `json:"-" validate:"required,gte=0"` // Counter for retried requests.
	counterSuccess *expvar.Int `json:"-" validate:"required,gte=0"` // Counter for successful requests.

	Logger sypl.ISypl `json:"-" validate:"required"` // Logger used for logging information.

	Headers map[string]string `json:"-"       validate:"omitempty,gt=0"`           // Default headers for every request.
	Name    string            `json:"name"    validate:"required,lowercase,gte=1"` // Name of the client instance.
	Timeout time.Duration     `json:"timeout" validate:"omitempty,gte=100ms"`      // Timeout for HTTP requests.

	RetrierBackoffDuration time.Duration `json:"retrierBackoffDuration" validate:"omitempty,gte=100ms"` // Initial backoff duration for retries.
	RetrierBackoffTimes    int           `json:"retrierBackoffTimes"    validate:"omitempty,gte=1"`     // Number of retry attempts.
}

// HandleHTTPResponseError handles errors in HTTP responses.
// It enriches errors with additional context, such as the URL, status code,
// and response body, and ensures that the response body is properly closed.
//
// Parameters:
// - resp: The HTTP response that may contain an error.
// - url: The URL that was requested.
// - msg: A message describing the error.
// - externalErr: An external error that triggered this handler.
//
// Returns:
// - An error enriched with context, or an error that occurred during processing.
func HandleHTTPResponseError(resp *http.Response, url, msg string, externalErr error) error {
	// Prepare options for creating a custom error.
	customerrorOptions := []customerror.Option{
		customerror.WithField("url", url), // Include the URL in the error context.
	}

	// Include the external error if provided.
	if externalErr != nil {
		customerrorOptions = append(customerrorOptions, customerror.WithError(externalErr))
	}

	// If the response is not nil and has a status code, include more details.
	if resp != nil && resp.StatusCode > 0 {
		customerrorOptions = append(customerrorOptions, customerror.WithStatusCode(resp.StatusCode))

		// If the response body is available, read and include it in the error.
		if resp.Body != nil {
			// Read the entire response body.
			body, err := shared.ReadAll(resp.Body)
			if err != nil {
				resp.Body.Close() // Ensure the body is closed even if read fails.

				return err // Return the error encountered while reading.
			}

			// Include the response body in the error context.
			customerrorOptions = append(customerrorOptions, customerror.WithField("respBody", string(body)))

			resp.Body.Close() // Close the body after reading.

			// Reset the response body so it can be read again if necessary.
			resp.Body = io.NopCloser(bytes.NewBuffer(body))
		}
	}

	// Create and return a new custom error with the provided context.
	return customerror.NewFailedToError(msg, customerrorOptions...)
}

// GetLogger returns the client's logger instance.
// This allows access to the logger for custom logging if needed.
func (c *Client) GetLogger() sypl.ISypl {
	return c.Logger
}

// GetName returns the name of the client instance.
// Useful for identifying the client in logs and metrics.
func (c *Client) GetName() string {
	return c.Name
}

// GetClient returns the underlying http.Client instance.
// This allows for direct access to the http.Client if low-level operations are needed.
func (c *Client) GetClient() *http.Client {
	return c.client
}

// SetName sets the client's name and returns the client instance.
// This allows for method chaining when configuring the client.
func (c *Client) SetName(name string) *Client {
	c.Name = name

	return c
}

// request is the core method used to make HTTP requests with enhanced features.
// It includes retry logic, logging, error handling, and response processing.
//
// Parameters:
// - ctx: Context for controlling cancellation and timeouts.
// - method: The HTTP method to use (e.g., GET, POST).
// - url: The URL to request.
// - o: Variadic functional options to customize the request.
//
// Returns:
// - The HTTP response and any error encountered.
func (c *Client) request(ctx context.Context, method string, url string, o ...Func) (*http.Response, error) {
	// Validate that the method and URL are provided.
	if method == "" || url == "" {
		return nil, customerror.NewRequiredError("method and url are required")
	}

	// Initialize options with default values.
	options := &Options{
		Headers:     make(map[string]string), // Per-request headers.
		QueryParams: make(map[string]string), // Query parameters.
	}

	// Apply functional options to customize the request.
	for _, opt := range o {
		if err := opt(options); err != nil {
			return nil, err // Return error if option application fails.
		}
	}

	// Create a new HTTP request with the provided method, URL, and body.
	req, err := http.NewRequestWithContext(ctx, method, url, options.reqBodyAsIOReader)
	if err != nil {
		return nil, customerror.NewFailedToError("create request", customerror.WithError(err))
	}

	// Set query parameters on the request URL.
	c.setQueryParams(req, options.QueryParams)

	// Set headers on the request.
	c.setHeaders(req, options.Headers)

	// Prepare fields for logging the request.
	reqFields := c.prepareRequestFields(method, url, req, options)

	// Copy request fields to response fields for consistent logging.
	respFields := make(fields.Fields, len(reqFields))
	for k, v := range reqFields {
		respFields[k] = v
	}

	// Log the creation of the request.
	c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Created.String(), sypl.WithFields(reqFields))

	// Initialize a retrier with exponential backoff strategy.
	r := retrier.New(
		retrier.ExponentialBackoff(c.RetrierBackoffTimes, c.RetrierBackoffDuration), // Backoff strategy.
		HTTPStatusCodeClassifier{Regex: httpRetrierRegex, Client: c},                // Classifier to determine retryable errors.
	)

	var resp *http.Response // Variable to hold the response.

	// Execute the request with retry logic.
	if err := r.Run(func() error {
		req.Close = true // Ensure the connection is closed after the request.

		// Perform the HTTP request.
		respInternal, err := c.client.Do(req)
		if err != nil {
			// Handle errors that occur during the request.
			return HandleHTTPResponseError(respInternal, url, "request", err)
		}

		// Check if the response status code indicates a retryable error.
		if respInternal != nil && respInternal.StatusCode > 0 {
			statusCodeStr := strconv.Itoa(respInternal.StatusCode) // Convert status code to string.

			// If the status code matches retryable codes, trigger a retry.
			if httpRetrierRegex.MatchString(statusCodeStr) {
				respInternal.Body.Close() // Close the body before returning the error.

				return HandleHTTPResponseError(respInternal, url, "request", err)
			}

			// If the status code is not in the 2xx range, handle as an error.
			if respInternal.StatusCode < http.StatusOK || respInternal.StatusCode >= http.StatusBadRequest {
				respInternal.Body.Close() // Close the body before returning the error.

				return HandleHTTPResponseError(respInternal, url, "request", err)
			}
		}

		// Log the successful request.
		c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Succeeded.String(), sypl.WithFields(reqFields))

		resp = respInternal // Assign the successful response.

		return nil // Return nil to indicate success.
	}); err != nil {
		return nil, err // Return any error encountered during retries.
	}

	// If a response body is expected and available, process it.
	if resp != nil && options.RespBody != nil && resp.Body != nil {
		if err := c.handleResponseBody(resp, options.RespBody, &respFields); err != nil {
			resp.Body.Close() // Ensure the body is closed if processing fails.

			return nil, err // Return the error encountered.
		}
	}

	// Increment the success counter for metrics.
	c.counterSuccess.Add(1)

	// Log the successful completion of the request.
	c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Succeeded.String(), sypl.WithFields(respFields), sypl.WithTags("request"))

	return resp, nil // Return the response and nil error.
}

// setQueryParams sets the query parameters for the HTTP request.
//
// Parameters:
// - req: The HTTP request to modify.
// - queryParams: A map of query parameter keys and values.
func (c *Client) setQueryParams(req *http.Request, queryParams map[string]string) {
	// Only set query parameters if any are provided.
	if len(queryParams) > 0 {
		q := req.URL.Query() // Get the existing query parameters.

		loggingOpts := []sypl.OptionFunc{sypl.WithTags("query params")} // Prepare logging options.

		// Add each query parameter to the request and logging options.
		for k, v := range queryParams {
			q.Add(k, v)                                             // Add query parameter to the request URL.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v)) // Add to logging fields.
		}

		// Log the query parameters being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "query params", loggingOpts...)

		req.URL.RawQuery = q.Encode() // Encode and set the query parameters.
	}
}

// setHeaders sets the headers for the HTTP request, including both default and per-request headers.
//
// Parameters:
// - req: The HTTP request to modify.
// - headers: A map of per-request header keys and values.
func (c *Client) setHeaders(req *http.Request, headers map[string]string) {
	// Set default headers if any are defined.
	if c.Headers != nil {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("default headers")} // Prepare logging options.

		// Add each default header to the request and logging options.
		for k, v := range c.Headers {
			req.Header.Set(k, v)                                    // Set header on the request.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v)) // Add to logging fields.
		}

		// Log the default headers being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "default headers", loggingOpts...)
	}

	// Set per-request headers if any are provided.
	if headers != nil {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("per-request headers")} // Prepare logging options.

		// Add each per-request header to the request and logging options.
		for k, v := range headers {
			req.Header.Set(k, v)                                    // Set header on the request.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v)) // Add to logging fields.
		}

		// Log the per-request headers being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "per-request headers", loggingOpts...)
	}
}

// prepareRequestFields prepares the fields used for logging the request.
//
// Parameters:
// - method: The HTTP method being used.
// - url: The URL being requested.
// - req: The HTTP request.
// - options: The options provided for the request.
//
// Returns:
// - A fields.Fields map containing the prepared logging fields.
func (c *Client) prepareRequestFields(method, url string, req *http.Request, options *Options) fields.Fields {
	reqFields := fields.Fields{
		"method": method, // Log the HTTP method.
		"url":    url,    // Log the requested URL.
	}

	// Include headers in the logging fields if provided.
	if options.Headers != nil {
		reqFields["reqHeaders"] = req.Header // Include the request headers.
	}

	// Include the request body in the logging fields if provided.
	if options.ReqBody != nil {
		reqFields["reqBody"] = options.ReqBody // Include the request body.
	}

	// Include query parameters in the logging fields if present.
	if req.URL.RawQuery != "" {
		reqFields["reqQueryParams"] = req.URL.RawQuery // Include the raw query string.
	}

	// Correlate request with any existing application performance monitoring (APM) context.
	return logging.ToAPM(req.Context(), reqFields)
}

// handleResponseBody reads and processes the response body.
//
// Parameters:
// - resp: The HTTP response containing the body.
// - respBody: The destination where the response body should be written or decoded.
// - respFields: A pointer to the logging fields to be updated.
//
// Returns:
// - An error if any occurs during processing.
func (c *Client) handleResponseBody(resp *http.Response, respBody interface{}, respFields *fields.Fields) error {
	defer resp.Body.Close() // Ensure the response body is always closed after processing.

	var bodyBytes []byte // Variable to hold the response body bytes.

	// Determine how to handle the response body based on the type of respBody.
	switch v := respBody.(type) {
	case *os.File:
		// If respBody is a file, write the response body directly to the file.
		if _, err := io.Copy(v, resp.Body); err != nil {
			return err // Return error if writing fails.
		}
	default:
		// For other types, read the entire response body.
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err // Return error if reading fails.
		}

		// Decode the response body into the provided interface.
		if err := shared.Decode(bytes.NewReader(bodyBytes), respBody); err != nil {
			return err // Return error if decoding fails.
		}

		// Include the response body in the logging fields.
		(*respFields)["respBody"] = fmt.Sprintf("%+v", respBody)
	}

	// Reset the response body so it can be read again if necessary.
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Include the status code in the logging fields if available.
	if resp.StatusCode > 0 {
		(*respFields)["respStatusCode"] = resp.StatusCode
	}

	return nil // Return nil to indicate successful processing.
}

// Get returns the singleton instance of the Client.
// It panics if the client has not been initialized.
//
// Returns:
// - The singleton Client instance.
func Get() *Client {
	if singleton == nil {
		panic("http client not initialized. Call `http.New()` or `http.NewDefault()` first")
	}

	return singleton
}

// IsRespSuccess checks if the response status code indicates a successful request.
//
// Parameters:
// - resp: The HTTP response to check.
//
// Returns:
// - true if the status code is between 200 and 299, indicating success.
// - false otherwise.
func IsRespSuccess(resp *http.Response) bool {
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

// New creates a new instance of the Client with the provided configuration.
//
// Parameters:
// - name: The name of the client instance.
// - headers: Default headers to include in every request.
// - timeout: The timeout duration for HTTP requests.
// - retrierBackoffDuration: The initial backoff duration for retries.
// - retrierBackoffTimes: The number of retry attempts.
//
// Returns:
// - A pointer to the newly created Client.
// - An error if validation fails or parameters are invalid.
func New(name string, headers map[string]string, timeout time.Duration, retrierBackoffDuration time.Duration, retrierBackoffTimes int) (*Client, error) {
	// Ensure that Client implements the IHTTP interface.
	var _ IHTTP = (*Client)(nil)

	// Create a new logger instance for the client.
	logger := logging.Get().New(name).SetTags(shared.PackageName, name)

	// Initialize the Client instance with provided parameters.
	client := &Client{
		client: &http.Client{
			Timeout: timeout, // Set the timeout for HTTP requests.
		},
		counterFailed:          metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Failed, DefaultMetricCounterLabel)),
		counterRetried:         metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Retried, DefaultMetricCounterLabel)),
		counterSuccess:         metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Succeeded, DefaultMetricCounterLabel)),
		Logger:                 logger,
		Headers:                headers,
		Name:                   name,
		RetrierBackoffDuration: retrierBackoffDuration,
		RetrierBackoffTimes:    retrierBackoffTimes,
	}

	// Set default retrier backoff duration if not provided.
	if retrierBackoffDuration == 0 {
		client.RetrierBackoffDuration = time.Second // Default to 1 second.
	}

	// Set default retrier backoff times if not provided.
	if retrierBackoffTimes == 0 {
		client.RetrierBackoffTimes = 3 // Default to 3 retries.
	}

	// Validate the client configuration.
	if err := validation.Validate(client); err != nil {
		return nil, err // Return error if validation fails.
	}

	// Log the creation of the client.
	client.GetLogger().PrintlnWithOptions(
		level.Trace,
		fmt.Sprintf("%+v %s %s", client.GetName(), shared.PackageName, status.Created),
		sypl.WithTags(shared.PackageName, status.Initialized.String(), client.GetName()),
	)

	return client, nil // Return the initialized client.
}

// NewSingleton creates a new Client instance and sets it as the singleton.
// It ensures that only one instance is created, even in concurrent environments.
//
// Parameters:
// - name: The name of the client instance.
// - headers: Default headers to include in every request.
// - timeout: The timeout duration for HTTP requests.
// - retrierBackoffDuration: The initial backoff duration for retries.
// - retrierBackoffTimes: The number of retry attempts.
//
// Returns:
// - A pointer to the singleton Client instance.
// - An error if initialization fails.
func NewSingleton(name string, headers map[string]string, timeout time.Duration, retrierBackoffDuration time.Duration, retrierBackoffTimes int) (*Client, error) {
	var errOut error // Variable to hold any error encountered.

	// Use sync.Once to ensure singleton initialization.
	once.Do(func() {
		c, err := New(name, headers, timeout, retrierBackoffDuration, retrierBackoffTimes)
		if err != nil {
			errOut = err // Capture the error to return later.

			return
		}

		singleton = c // Set the singleton instance.
	})

	if errOut != nil {
		return nil, errOut // Return any error encountered during initialization.
	}

	return singleton, nil // Return the singleton client.
}

// NewDefaultSingleton creates a new default Client instance and sets it as the singleton.
//
// Parameters:
// - name: The name of the client instance.
//
// Returns:
// - A pointer to the singleton Client instance.
// - An error if initialization fails.
func NewDefaultSingleton(name string) (*Client, error) {
	var errOut error // Variable to hold any error encountered.

	// Use sync.Once to ensure singleton initialization.
	once.Do(func() {
		c, err := NewDefault(name)
		if err != nil {
			errOut = err // Capture the error to return later.

			return
		}

		singleton = c // Set the singleton instance.
	})

	if errOut != nil {
		return nil, errOut // Return any error encountered during initialization.
	}

	return singleton, nil // Return the singleton client.
}

// NewDefault creates a new Client instance with default settings.
//
// It sets the following headers:
// - Accept: "*/*"
// - Content-Type: "application/json"
// - User-Agent: name
//
// It uses default values for timeout (30 seconds) and retrier configuration.
//
// Parameters:
// - name: The name of the client instance.
//
// Returns:
// - A pointer to the newly created Client.
// - An error if initialization fails.
func NewDefault(name string) (*Client, error) {
	return New(name, map[string]string{
		"Accept":       "*/*",              // Accept all content types.
		"Content-Type": "application/json", // Default content type.
		"User-Agent":   name,               // User-Agent set to the client name.
	}, 30*time.Second, time.Second, 3) // Default timeout and retrier settings.
}

// Initialize sets up the Client using provided options.
// It allows for custom configuration through functional options.
//
// Parameters:
// - opts: Variadic functional options to customize the client.
//
// Returns:
// - A pointer to the initialized Client.
// - An error if initialization fails.
func Initialize(opts ...ClientFunc) (*Client, error) {
	options := &ClientOptions{} // Initialize options struct.

	// Apply client options provided.
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, err // Return error if option application fails.
		}
	}

	// Set a default name if not provided.
	if options.Name == "" {
		options.Name = shared.PackageName + "-" + shared.GenerateUUID() // Generate a unique name.
	}

	return NewDefault(options.Name) // Create a default client with the specified or generated name.
}
