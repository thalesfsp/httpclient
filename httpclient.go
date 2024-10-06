package httpclient

import (
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

//////
// Vars, consts, and types.
//////

// DefaultMetricCounterLabel is the default metric counter label.
const DefaultMetricCounterLabel = "counter"

// Singleton.
var (
	httpRetrierRegex = regexp.MustCompile(`(?m)^(408|429|5\d\d)$`)

	once      sync.Once
	singleton *Client
)

// Client is the application pre-configured HTTP client.
type Client struct {
	client *http.Client

	// Request's metrics.
	counterFailed  *expvar.Int `json:"-" validate:"required,gte=0"`
	counterRetried *expvar.Int `json:"-" validate:"required,gte=0"`
	counterSuccess *expvar.Int `json:"-" validate:"required,gte=0"`

	Logger sypl.ISypl `json:"-" validate:"required"`

	Headers map[string]string `json:"-"       validate:"omitempty,gt=0"`
	Name    string            `json:"name"    validate:"required,lowercase,gte=1"`
	Timeout time.Duration     `json:"timeout" validate:"omitempty,gte=100ms"`

	RetrierBackoffDuration time.Duration `json:"retrierBackoffDuration" validate:"omitempty,gte=100ms"`
	RetrierBackoffTimes    int           `json:"retrierBackoffTimes"    validate:"omitempty,gte=1"`
}

//////
// Helpers.
//////

// HandleHTTPResponseError handles the HTTP response error.
func HandleHTTPResponseError(
	resp *http.Response,
	url, msg string,
	externalErr error,
) error {
	// Set the status code.
	customerrorOptions := []customerror.Option{
		customerror.WithField("url", url),
	}

	if externalErr != nil {
		customerrorOptions = append(
			customerrorOptions,
			customerror.WithError(externalErr),
		)
	}

	if resp != nil && resp.StatusCode > 0 {
		customerrorOptions = append(
			customerrorOptions,
			customerror.WithStatusCode(resp.StatusCode),
		)

		// Optionally set the body.
		if resp.Body != nil {
			var body []byte

			body, err := shared.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			customerrorOptions = append(
				customerrorOptions,
				customerror.WithField("respBody", string(body)),
			)
		}
	}

	// Return the error.
	return customerror.NewFailedToError(msg, customerrorOptions...)
}

//////
// Implements the IMeta interface.
//////

// GetLogger returns the logger.
func (c *Client) GetLogger() sypl.ISypl {
	return c.Logger
}

// GetName returns the HTTP client name.
func (c *Client) GetName() string {
	return c.Name
}

// GetClient returns the HTTP client.
func (c *Client) GetClient() *http.Client {
	return c.client
}

// SetName sets the HTTP client name.
func (c *Client) SetName(name string) *Client {
	c.Name = name

	return c
}

//////
// Methods.
//////

// request is the base request.
//
// For `body`, optionally pass a `struct`.
// For `headers`, optionally pass a `map[string]string`.
// For `query params`, optionally pass a `map[string]string`.
//
// It throws custom errors, and handle retries, metrics, and logging.
//
// NOTE: Per-request timeout is achieved by using `context.WithTimeout`.
//
// NOTE: If `respBody` is provided, it will read and decode the body, ALSO
// CLOSING IT. Otherwise, the body will be left open, and returned. In this case
// IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
//
//nolint:gocognit,goerr113,bodyclose,cyclop,maintidx
func (c *Client) request(
	ctx context.Context,
	method string,
	url string,
	o ...Func,
) (*http.Response, error) {
	// Basic validation.
	if method == "" || url == "" {
		return nil, customerror.NewRequiredError("method and url are")
	}

	// Initialize the options.
	options := &Options{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
		ReqBody:     nil,
		RespBody:    nil,

		reqBodyAsIOReader: nil,
	}

	// Applies options.
	for _, opt := range o {
		if err := opt(options); err != nil {
			return nil, err
		}
	}

	//////
	// Create a request.
	//////

	req, err := http.NewRequestWithContext(ctx, method, url, options.reqBodyAsIOReader)
	if err != nil {
		return nil, customerror.NewFailedToError("create request", customerror.WithError(err))
	}

	//////
	// Setup query params.
	//////

	if len(options.QueryParams) > 0 {
		q := req.URL.Query()

		loggingOpts := []sypl.OptionFunc{
			sypl.WithTags("query params"),
		}

		for k, v := range options.QueryParams {
			q.Add(k, v)

			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		c.GetLogger().PrintlnWithOptions(
			level.Trace,
			"query params",
			loggingOpts...,
		)

		req.URL.RawQuery = q.Encode()
	}

	//////
	// Setup headers.
	//////

	// From default headers.
	if c.Headers != nil {
		loggingOpts := []sypl.OptionFunc{
			sypl.WithTags("default headers"),
		}

		for k, v := range c.Headers {
			req.Header.Set(k, v)

			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		c.GetLogger().PrintlnWithOptions(
			level.Trace,
			"default headers",
			loggingOpts...,
		)
	}

	// Per-request headers.
	if options.Headers != nil {
		loggingOpts := []sypl.OptionFunc{
			sypl.WithTags("per-request headers"),
		}

		for k, v := range options.Headers {
			req.Header.Set(k, v)

			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		c.GetLogger().PrintlnWithOptions(
			level.Trace,
			"per-request headers",
			loggingOpts...,
		)
	}

	//////
	// Setup logging fields.
	//////

	reqFields := fields.Fields{
		"method": method,
		"url":    url,
	}

	if options.Headers != nil {
		reqFields["reqHeaders"] = req.Header
	}

	if options.ReqBody != nil {
		reqFields["reqBody"] = options.ReqBody
	}

	if req.URL.RawQuery != "" {
		reqFields["reqQueryParams"] = req.URL.RawQuery
	}

	// Request <-> Transaction <-> Log correlation.
	reqFields = logging.ToAPM(ctx, reqFields)

	c.GetLogger().PrintlnWithOptions(
		level.Trace,
		"request "+status.Created.String(),
		sypl.WithFields(reqFields),
	)

	// Copy reqFields to respFields.
	respFields := make(fields.Fields, len(reqFields))

	for k, v := range reqFields {
		respFields[k] = v
	}

	//////
	// Send request.
	//////

	r := retrier.New(
		retrier.ExponentialBackoff(c.RetrierBackoffTimes, c.RetrierBackoffDuration),
		// Automagically retries on 408, 429, and 5xx.
		HTTPStatusCodeClassifier{
			Regex:  httpRetrierRegex,
			Client: c,
		},
	)

	var resp *http.Response

	if err := r.Run(func() error {
		req.Close = true

		// WARN: A non-2xx status code doesn't cause an error!
		respInternal, err := c.client.Do(req)
		//////
		// Deals with errored requests.
		//////
		if err != nil {
			// Handle the error because it's an error, lol.
			return HandleHTTPResponseError(respInternal, url, "request", err)
		}

		if respInternal != nil && respInternal.StatusCode > 0 {
			//////
			// Deal with retryable requests.
			//////

			if httpRetrierRegex.MatchString(strconv.Itoa(respInternal.StatusCode)) {
				// There's no error, but it's a retryable case.
				return HandleHTTPResponseError(respInternal, url, "request", err)
			}

			//////
			// Deal with out-of-ban (non-2xx - 3xx) requests.
			//////

			// If 2xx neither 4xx, return an error with the status code.
			if respInternal.StatusCode < http.StatusOK || respInternal.StatusCode >= http.StatusBadRequest {
				// There's no error, it's non-retryable, and it's not 2xx. Just
				// return a error.
				return HandleHTTPResponseError(respInternal, url, "request", err)
			}
		}

		c.GetLogger().PrintlnWithOptions(
			level.Trace,
			"request "+status.Succeeded.String(),
			sypl.WithFields(reqFields),
		)

		resp = respInternal

		// If did reach here, it's a successful 2xx request.
		return nil
	}); err != nil {
		return nil, err
	}

	//////
	// Handles response body.
	//////

	//nolint:gocritic
	if resp != nil && options.RespBody != nil && resp.Body != nil {
		switch options.RespBody.(type) {
		case *os.File:
			// Write the response body to file
			if _, err := io.Copy(options.RespBody.(*os.File), resp.Body); err != nil {
				return resp, err
			}

		default:
			if err := shared.Decode(resp.Body, options.RespBody); err != nil {
				return resp, err
			}

			respFields["respBody"] = fmt.Sprintf("%+v", options.RespBody)
		}

		if resp.StatusCode > 0 {
			respFields["respStatusCode"] = resp.StatusCode
		}
	}

	// Metrics
	c.counterSuccess.Add(1)

	c.GetLogger().PrintlnWithOptions(
		level.Trace,
		"request "+status.Succeeded.String(),
		sypl.WithFields(respFields),
		sypl.WithTags("request"),
	)

	return resp, nil
}

//////
// Exported functionalities.
//////

// Get creates a new HTTP client, otherwise returns it.
func Get() *Client {
	if singleton == nil {
		panic("http client not initialized. Call `http.New()` or `http.NewDefault()` first")
	}

	return singleton
}

// IsRespSuccess returns true if the response is a success.
func IsRespSuccess(resp *http.Response) bool {
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

//////
// Factory.
//////

// New creates a new HTTP client. Default values:
// - Timeout: 30s
// - Retrier Times: 3x
// - Retrier Initial Time: 1s
//
// NOTE: `headers` sets default headers for all requests.
//
// NOTE: Retrier use exponential backoff (e.g.: 1s -> 2s -> 4s). Be mindful:
// `timeout` can't be less than the cumulative retrier time.
//
//nolint:lll,mnd
func New(
	name string,
	headers map[string]string,
	timeout time.Duration,
	retrierBackoffDuration time.Duration,
	retrierBackoffTimes int,
) (*Client, error) {
	// Enforces IHTTP interface implementation.
	var _ IHTTP = (*Client)(nil)

	logger := logging.Get().New(name).SetTags(shared.PackageName, name)

	client := &Client{
		client: &http.Client{
			Timeout: timeout,
		},

		//////
		// Request's metrics.
		//////

		counterFailed:  metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Failed, DefaultMetricCounterLabel)),
		counterRetried: metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Retried, DefaultMetricCounterLabel)),
		counterSuccess: metrics.NewInt(fmt.Sprintf("%s.%s.%s.%s", shared.PackageName, name, status.Succeeded, DefaultMetricCounterLabel)),

		Logger: logger,

		Headers:                headers,
		Name:                   name,
		RetrierBackoffDuration: 1 * time.Second,
		RetrierBackoffTimes:    3,
	}

	if retrierBackoffDuration > 0 {
		client.RetrierBackoffDuration = retrierBackoffDuration
	}

	if retrierBackoffTimes > 0 {
		client.RetrierBackoffTimes = retrierBackoffTimes
	}

	// Validate the HTTP client.
	if err := validation.Validate(client); err != nil {
		return nil, err
	}

	client.GetLogger().PrintlnWithOptions(
		level.Trace,
		fmt.Sprintf("%+v %s %s", client.GetName(), shared.PackageName, status.Created),
		sypl.WithTags(shared.PackageName, status.Initialized.String(), client.GetName()),
	)

	return client, nil
}

// NewSingleton creates a new HTTP client, and sets it as the singleton.
func NewSingleton(
	name string,
	headers map[string]string,
	timeout time.Duration,
	retrierBackoffDuration time.Duration,
	retrierBackoffTimes int,
) (*Client, error) {
	var errOut error

	once.Do(func() {
		c, err := New(
			name,
			headers,
			timeout,
			retrierBackoffDuration,
			retrierBackoffTimes,
		)
		if err != nil {
			errOut = err
		}

		singleton = c
	})

	if errOut != nil {
		return nil, errOut
	}

	return singleton, nil
}

// NewSingleton creates a new HTTP client, and sets it as the singleton.
func NewDefaultSingleton(name string) (*Client, error) {
	var errOut error

	once.Do(func() {
		c, err := NewDefault(name)
		if err != nil {
			errOut = err
		}

		singleton = c
	})

	if errOut != nil {
		return nil, errOut
	}

	return singleton, nil
}

// NewDefault is like new, but uses the default values.
//
// Set the following headers:
// Accept:       "*/*"
// Content-Type: "application/json"
// User-Agent:   "name"
func NewDefault(name string) (*Client, error) {
	return New(name, map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/json",
		"User-Agent":   name,
	}, 0, 0, 0)
}

// Initialize setup the HTTP client. Use options to customize it.
//
//nolint:wsl
func Initialize(opts ...ClientFunc) (*Client, error) {
	//////
	// Initialize and process options.
	//////

	options := &ClientOptions{}

	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, err
		}
	}

	if options.Name == "" {
		options.Name = shared.PackageName + "-" + shared.GenerateUUID()
	}

	return NewDefault(options.Name)
}
