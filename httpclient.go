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
	"sync/atomic"
	"time"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/thalesfsp/customerror"
	"github.com/thalesfsp/httpclient/v2/internal/logging"
	"github.com/thalesfsp/httpclient/v2/internal/metrics"
	"github.com/thalesfsp/httpclient/v2/internal/shared"
	"github.com/thalesfsp/status"
	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/fields"
	"github.com/thalesfsp/sypl/level"
	"github.com/thalesfsp/validation"
)

// DefaultMetricCounterLabel is the default label used for metric counters.
// It is used when initializing expvar.Int counters for tracking request metrics.
const DefaultMetricCounterLabel = "counter"

// Type of the entity.
const Type = "httpclient"

// maxErrorBodyBytes caps how much of an error response body is read into the
// error context, bounding memory on huge error payloads (multiplied by retries).
const maxErrorBodyBytes = 1 << 20 // 1 MiB.

// httpRetrierRegex is a regular expression that matches HTTP status codes
// indicating that a request should be retried. Specifically, it matches:
// - 408 (Request Timeout)
// - 429 (Too Many Requests)
// - 5xx (Server Errors)
var httpRetrierRegex = regexp.MustCompile(`(?m)^(408|429|5\d\d)$`)

// singleton holds the process-wide Client. It is an atomic.Pointer so Get() can
// read it locklessly without racing the store done by the NewSingleton
// constructors, which serialize their check-and-set under singletonMu.
//
// A mutex (not sync.Once) guards initialization so that a FAILED init does not
// poison the singleton: unlike once.Do, a later call can retry after an error
// instead of silently returning (nil, nil).
var (
	singletonMu sync.Mutex
	singleton   atomic.Pointer[Client]
)

// Ensure that Client implements the IHTTP interface.
// This is a compile-time check to ensure that Client satisfies the interface.
// If Client does not implement all methods of IHTTP, this line will cause a
// compile error.
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

	// WARN: CustomRetrierRegexes, Headers, and QueryParams are READ on every
	// request. They are safe for concurrent requests, but must NOT be mutated
	// after the client starts serving requests — configure them at construction
	// time (via New/NewDefault options). Mutating them concurrently with a
	// request is a data race.
	CustomRetrierRegexes []*regexp.Regexp  `json:"-"`                                // Custom regexes for retryable errors.
	Headers              map[string]string `json:"-" validate:"omitempty,dive,gt=0"` // Default headers for every request.
	QueryParams          map[string]string `json:"-" validate:"omitempty,dive,gt=0"` // Default query parameters for every request.

	Name string `json:"name" validate:"required,lowercase,gte=1"` // Name of the client instance.

	RetrierBackoffDuration time.Duration `json:"retrierBackoffDuration" validate:"omitempty,gte=100ms"` // Initial backoff duration for retries.
	RetrierBackoffTimes    int           `json:"retrierBackoffTimes"    validate:"omitempty,gte=1"`     // Number of retry attempts.
	Timeout                time.Duration `json:"timeout"                validate:"omitempty,gte=100ms"` // Timeout for HTTP requests.
}

// HandleHTTPResponseError handles errors in HTTP responses.
// It enriches errors with additional context, such as the URL, status code,
// and response body, and ensures that the response body is properly closed.
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
			// Ensure the response body will be closed.
			defer resp.Body.Close()

			// Read the response body for the error context, bounded to
			// maxErrorBodyBytes so a huge error payload (× retries) cannot blow
			// up memory.
			body, err := shared.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))

			// Drain a bounded remainder so a normal-sized error body still lets
			// the keep-alive connection be reused, without draining a
			// pathological body (which would let a server tie us up).
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))

			if err == nil {
				// Include the response body in the error context.
				customerrorOptions = append(customerrorOptions, customerror.WithField("respBody", string(body)))

				// Reset the response body so it can be read again if necessary.
				resp.Body = io.NopCloser(bytes.NewBuffer(body))
			}
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
// This allows for direct access to the http.Client if low-level operations are
// needed.
func (c *Client) GetClient() *http.Client {
	return c.client
}

// SetName sets the client's name and returns the client instance.
// This allows for method chaining when configuring the client.
func (c *Client) SetName(name string) *Client {
	c.Name = name

	return c
}

// traceEnabled reports whether any logger output is at TRACE level, so the hot
// path can skip building log fields (maps, APM correlation, Sprintf) that would
// be discarded at the default ERROR level.
func (c *Client) traceEnabled() bool {
	for _, lvl := range c.Logger.GetMaxLevel() {
		if lvl >= level.Trace {
			return true
		}
	}

	return false
}

// request is the core method used to make HTTP requests with enhanced features.
// It includes retry logic, logging, error handling, and response processing.
func (c *Client) request(ctx context.Context, method string, url string, o ...Func) (resp *http.Response, err error) {
	// Count every failed request exactly once, no matter which stage failed
	// (preflight, transport / retry-exhaustion, or response-body processing). A
	// successful request increments counterSuccess instead (below).
	defer func() {
		if err != nil {
			c.counterFailed.Add(1)
		}
	}()

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

	// Build log fields only when trace logging is actually enabled. The field
	// maps, APM correlation, map copy, and later Sprintf are wasted work at the
	// default ERROR level.
	logTrace := c.traceEnabled()

	var reqFields, respFields fields.Fields
	if logTrace {
		reqFields = c.prepareRequestFields(method, url, req, options)

		// Copy request fields to response fields for consistent logging.
		respFields = make(fields.Fields, len(reqFields))
		for k, v := range reqFields {
			respFields[k] = v
		}

		// Log the creation of the request.
		c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Created.String(), sypl.WithFields(reqFields))
	}

	// Initialize a retrier with exponential backoff strategy.
	r := retrier.New(
		retrier.ExponentialBackoff(c.RetrierBackoffTimes, c.RetrierBackoffDuration), // Backoff strategy.
		CustomClassifier{
			// Custom regexes for retryable errors.
			CustomRegexes: c.setCustomRetryableRegexes(options.CustomRetrierRegexes...),

			// Regex to match retryable HTTP status codes.
			HTTPRetrierRegex: httpRetrierRegex,

			// Classifier to determine retryable errors.
			Client: c,
		},
	)

	// Execute the request with retry logic (resp is the named return value).
	if err := r.Run(func() error {
		// NOTE: do NOT set req.Close = true. It forces "Connection: close" and
		// defeats the transport's keep-alive pool, opening a fresh TCP/TLS
		// connection on every request. Callers reusing a client benefit from
		// connection reuse; they remain responsible for draining/closing bodies.

		// Rewind the request body before each attempt. The body reader is
		// drained (and closed) by the transport on every attempt, so without
		// this a retry would send an empty body and fail with a ContentLength
		// mismatch. GetBody is set by http.NewRequest for the rewindable body
		// types this package produces (string, []byte, url.Values, JSON).
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return HandleHTTPResponseError(nil, url, "rewind request body", err)
			}

			req.Body = body
		}

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
				return HandleHTTPResponseError(respInternal, url, "request", err)
			}

			// If the status code is not in the 2xx range, handle as an error.
			if respInternal.StatusCode < http.StatusOK || respInternal.StatusCode >= http.StatusBadRequest {
				return HandleHTTPResponseError(respInternal, url, "request", err)
			}
		}

		resp = respInternal // Assign the successful response.

		return nil // Return nil to indicate success.
	}); err != nil {
		return nil, err // Counted by the deferred failure counter.
	}

	// If a response body is expected and available, process it.
	if resp != nil && options.RespBody != nil && resp.Body != nil {
		if err := c.handleResponseBody(resp, options.RespBody, respFields); err != nil {
			return nil, err // Return the error encountered.
		}
	}

	// Increment the success counter for metrics.
	c.counterSuccess.Add(1)

	// Log the successful completion of the request (trace only).
	if logTrace {
		c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Succeeded.String(), sypl.WithFields(respFields), sypl.WithTags("request"))
	}

	return resp, nil // Return the response and nil error.
}

// setQueryParams sets the query params for the HTTP request, including both
// default and per-request headers.
func (c *Client) setQueryParams(req *http.Request, queryParams map[string]string) {
	// NOTE: do NOT lazily initialize c.QueryParams here. Writing to the shared
	// client field during a request is a data race under concurrent use;
	// ranging over a nil map below is already a safe no-op.

	// Initialize the query parameters for the request.
	q := req.URL.Query()

	// Process default query parameters if any are defined.
	if len(c.QueryParams) > 0 {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("default query params")}

		// Add each query parameter to the request, and logging options.
		for k, v := range c.QueryParams {
			// Add query parameter to the request.
			q.Add(k, v)

			// Add to logging fields.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		// Log the default query parameters being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "default query params", loggingOpts...)
	}

	// Process per-request query parameters if any are defined.
	if len(queryParams) > 0 {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("per-request query params")}

		// Add each query parameter to the request, and logging options.
		for k, v := range queryParams {
			// Add query parameter to the request.
			q.Add(k, v)

			// Add to logging fields.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		// Log the per-request query parameters being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "per-request query params", loggingOpts...)
	}

	// Encode if there are any query parameters.
	if len(q) > 0 {
		req.URL.RawQuery = q.Encode()
	}
}

// setHeaders sets the headers for the HTTP request, including both default and
// per-request headers.
func (c *Client) setHeaders(req *http.Request, headers map[string]string) {
	// NOTE: do NOT lazily initialize c.Headers here. Writing to the shared
	// client field during a request is a data race under concurrent use;
	// ranging over a nil map below is already a safe no-op.

	// Process default headers if any are defined.
	if len(c.Headers) > 0 {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("default headers")}

		// Add each header to the request and logging options.
		for k, v := range c.Headers {
			// Add header to the request.
			req.Header.Set(k, v)

			// Add to logging fields.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		// Log the default headers being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "default headers", loggingOpts...)
	}

	// Process per-request headers if any are defined.
	if len(headers) > 0 {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("per-request headers")}

		// Add each header to the request and logging options.
		for k, v := range headers {
			// Add header to the request.
			req.Header.Set(k, v)

			// Add to logging fields.
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		// Log the per-request headers being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "per-request headers", loggingOpts...)
	}
}

// setCustomRetryableRegexes sets the custom retryable regexes for the client.
// It combines the default and per-request retryable regexes and returns the
// combined list.
func (c *Client) setCustomRetryableRegexes(customRetryableRegexes ...*regexp.Regexp) []*regexp.Regexp {
	// Initialize accumulator for custom retryable regexes.
	accumulator := make([]*regexp.Regexp, 0)

	// NOTE: do NOT lazily initialize c.CustomRetrierRegexes here. Writing to the
	// shared client field during a request is a data race under concurrent use;
	// ranging over a nil slice below is already a safe no-op.

	// Process default headers if any are defined.
	if len(c.CustomRetrierRegexes) > 0 {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("default custom retryable regexes")}

		// Add each header to the request and logging options.
		for i, v := range c.CustomRetrierRegexes {
			// Append to accumulator.
			accumulator = append(accumulator, v)

			// Add to logging fields.
			loggingOpts = append(loggingOpts, sypl.WithField(fmt.Sprintf("default custom retryable regex - %d", i), v))
		}

		// Log the default headers being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "default custom retryable regexes", loggingOpts...)
	}

	// Process per-request headers if any are defined.
	if len(customRetryableRegexes) > 0 {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("per-request custom retryable regexes")}

		// Add each header to the request and logging options.
		for i, v := range customRetryableRegexes {
			// Append to accumulator.
			accumulator = append(accumulator, v)

			// Add to logging fields.
			loggingOpts = append(loggingOpts, sypl.WithField(fmt.Sprintf("per-request custom retryable regex - %d", i), v))
		}

		// Log the per-request headers being set.
		c.GetLogger().PrintlnWithOptions(level.Trace, "per-request custom retryable regexes", loggingOpts...)
	}

	return accumulator
}

// prepareRequestFields prepares the fields used for logging the request.
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

// handleResponseBody reads and processes the response body. respFields may be
// nil when trace logging is disabled, in which case no log fields are recorded
// (and the expensive Sprintf is skipped).
func (c *Client) handleResponseBody(resp *http.Response, respBody interface{}, respFields fields.Fields) error {
	// Ensure the response body will be closed.
	defer resp.Body.Close()

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
		//
		// NOTE: assign to the outer `bodyBytes` (do NOT use `:=` here, which
		// would shadow it and leave the reset body empty).
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err // Return error if reading fails.
		}

		bodyBytes = b

		// Decode the response body into the provided interface.
		if err := shared.Decode(bytes.NewReader(bodyBytes), respBody); err != nil {
			return err // Return error if decoding fails.
		}

		// Include the response body in the logging fields (trace only).
		if respFields != nil {
			respFields["respBody"] = fmt.Sprintf("%+v", respBody)
		}
	}

	// Reset the response body so it can be read again if necessary.
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Include the status code in the logging fields if available.
	if respFields != nil && resp.StatusCode > 0 {
		respFields["respStatusCode"] = resp.StatusCode
	}

	return nil // Return nil to indicate successful processing.
}

// Get returns the singleton instance of the Client.
//
// WARN: It panics if the client has not been initialized.
func Get() *Client {
	c := singleton.Load()
	if c == nil {
		panic("http client not initialized. Call `http.New()` or `http.NewDefault()` first")
	}

	return c
}

// IsRespSuccess checks if the response status code indicates a successful
// request. A nil response is treated as unsuccessful (never panics).
func IsRespSuccess(resp *http.Response) bool {
	if resp == nil {
		return false
	}

	return resp.StatusCode >= http.StatusOK &&
		resp.StatusCode < http.StatusMultipleChoices
}

//////
// Factory.
//////

//////
// Base factory.

// New creates a new instance of the Client with the provided configuration.
func New(
	name string,
	headers map[string]string,
	queryParams map[string]string,
	customRetrierRegexes []*regexp.Regexp,
	timeout time.Duration,
	retrierBackoffDuration time.Duration,
	retrierBackoffTimes int,
) (*Client, error) {
	// Ensure that Client implements the IHTTP interface.
	var _ IHTTP = (*Client)(nil)

	// Create a new logger instance for the client.
	logger := logging.Get().New(name).SetTags(shared.PackageName, name)

	if customRetrierRegexes == nil {
		customRetrierRegexes = make([]*regexp.Regexp, 0)
	}

	if headers == nil {
		headers = make(map[string]string)
	}

	if queryParams == nil {
		queryParams = make(map[string]string)
	}

	// Initialize the Client instance with provided parameters.
	client := &Client{
		client: &http.Client{
			Timeout: timeout, // Set the timeout for HTTP requests.
		},

		counterFailed:  metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Failed, DefaultMetricCounterLabel)),
		counterRetried: metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Retried, DefaultMetricCounterLabel)),
		counterSuccess: metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Succeeded, DefaultMetricCounterLabel)),

		Logger: logger,

		CustomRetrierRegexes: customRetrierRegexes,
		Headers:              headers,
		QueryParams:          queryParams,

		Name: name,

		RetrierBackoffDuration: retrierBackoffDuration,
		RetrierBackoffTimes:    retrierBackoffTimes,
		Timeout:                timeout,
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

// NewDefault creates a new Client instance with default settings.
//
// It sets the:
// - Name to the package name and a random UUID.
// - Headers to:
//   - Accept: "*/*"
//   - Content-Type: "application/json"
//   - User-Agent: name
//
// - Timeout to 30 seconds
// - Retrier to:
//   - Exponential backoff
//   - 3 times
//   - 1 sec
func NewDefault(opts ...ClientFunc) (*Client, error) {
	// Defaul name is the package name and a random UUID.
	name := shared.PackageName + "-" + shared.GenerateUUID()

	// Initialize options struct.
	defaultOptions := &ClientOptions{
		Name: name,

		QueryParams: map[string]string{},
		Headers: map[string]string{
			// Accept all content types.
			"Accept": "*/*",

			// Default content type.
			"Content-Type": "application/json",

			// User-Agent set to the client name.
			"User-Agent": name,
		},
	}

	// Apply client options provided.
	for _, opt := range opts {
		if err := opt(defaultOptions); err != nil {
			return nil, err
		}
	}

	// Update the User-Agent header with the client name.
	if defaultOptions.Name != name {
		defaultOptions.Headers["User-Agent"] = defaultOptions.Name
	}

	return New(
		defaultOptions.Name,
		defaultOptions.Headers,
		defaultOptions.QueryParams,
		defaultOptions.CustomRetrierRegexes,
		30*time.Second,
		time.Second,
		3,
	)
}

//////
// Singleton factories.

// storeSingleton publishes c as the singleton if none is set yet and returns the
// winner. It double-checks under the lock so concurrent first-initializers agree
// on a single instance (the loser's client is discarded). The lock is held only
// for this fast check-and-store — never during client construction — so a
// re-entrant option func cannot deadlock.
func storeSingleton(c *Client) *Client {
	singletonMu.Lock()
	defer singletonMu.Unlock()

	if existing := singleton.Load(); existing != nil {
		return existing
	}

	singleton.Store(c)

	return c
}

// NewSingleton creates a new Client instance and sets it as the singleton.
// It ensures that only one instance is created, even in concurrent environments.
func NewSingleton(
	name string,
	headers map[string]string,
	queryParams map[string]string,
	customRetrierRegexes []*regexp.Regexp,
	timeout time.Duration,
	retrierBackoffDuration time.Duration,
	retrierBackoffTimes int,
) (*Client, error) {
	// Fast path: already initialized.
	if c := singleton.Load(); c != nil {
		return c, nil
	}

	// Build OUTSIDE the lock. A failed init is not stored, so a later call can
	// retry instead of being poisoned.
	c, err := New(
		name,
		headers,
		queryParams,
		customRetrierRegexes,
		timeout,
		retrierBackoffDuration,
		retrierBackoffTimes,
	)
	if err != nil {
		return nil, err
	}

	return storeSingleton(c), nil
}

// NewDefaultSingleton creates a new default Client instance and sets it as the
// singleton.
func NewDefaultSingleton(opts ...ClientFunc) (*Client, error) {
	// Fast path: already initialized.
	if c := singleton.Load(); c != nil {
		return c, nil
	}

	// Build OUTSIDE the lock so caller-supplied option funcs never run while
	// singletonMu is held (avoids a re-entrant deadlock). A failed init is not
	// stored, so a later call can retry.
	c, err := NewDefault(opts...)
	if err != nil {
		return nil, err
	}

	return storeSingleton(c), nil
}

//////
// Must

// MustNew creates a new Client instance and panics if initialization fails.
func MustNew(
	name string,
	headers map[string]string,
	queryParams map[string]string,
	customRetrierRegexes []*regexp.Regexp,
	timeout time.Duration,
	retrierBackoffDuration time.Duration,
	retrierBackoffTimes int,
) *Client {
	c, err := New(
		name,
		headers,
		queryParams,
		customRetrierRegexes,
		timeout,
		retrierBackoffDuration,
		retrierBackoffTimes,
	)
	if err != nil {
		panic(err)
	}

	return c
}

// MustNewDefault creates a new default Client instance and panics if
// initialization fails.
func MustNewDefault(opts ...ClientFunc) *Client {
	c, err := NewDefault(opts...)
	if err != nil {
		panic(err)
	}

	return c
}
