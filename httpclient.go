package httpclient

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"regexp"
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

const DefaultMetricCounterLabel = "counter"

// Singleton.
var (
	httpRetrierRegex = regexp.MustCompile(`(?m)(429)|(5\d\d)`)

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

	Headers map[string]string `json:"-" validate:"omitempty,gt=0"`
	Name    string            `json:"name" validate:"required,lowercase,gte=1"`
	Timeout time.Duration     `json:"timeout" validate:"omitempty,gte=100ms"`

	RetrierBackoffDuration time.Duration `json:"retrierBackoffDuration" validate:"omitempty,gte=100ms"`
	RetrierBackoffTimes    int           `json:"retrierBackoffTimes" validate:"omitempty,gte=1"`
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
	// Create request.
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

		for k, v := range options.QueryParams {
			q.Add(k, v)
		}

		req.URL.RawQuery = q.Encode()
	}

	//////
	// Setup headers.
	//////

	// From default headers.
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	// Per-request headers.
	if options.Headers != nil {
		for k, v := range options.Headers {
			req.Header.Set(k, v)
		}
	}

	//////
	// Setup log fields.
	//////

	reqFields := fields.Fields{
		"method": method,
		"url":    url,
	}

	if options.ReqBody != nil {
		reqFields["reqBody"] = options.ReqBody
	}

	if req.URL.RawQuery != "" {
		reqFields["queryParams"] = req.URL.RawQuery
	}

	// Request <-> Transaction <-> Log correlation.
	reqFields = logging.ToAPM(ctx, reqFields)

	c.Logger.PrintlnWithOptions(
		level.Debug,
		status.Created.String()+" request",
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
		HTTPStatusCodeClassifier{
			Regex: httpRetrierRegex,
		},
	)

	var resp *http.Response

	if err := r.Run(func() error {
		req.Close = true

		resp, err = c.client.Do(req)
		if err != nil {
			c.counterFailed.Add(1)

			// Resp isn't all the time available.
			if resp != nil && resp.Status != "" {
				respFields["status"] = resp.Status
			}

			cE := customerror.NewFailedToError(
				fmt.Sprintf("send request %s", url),
				customerror.WithError(err),
			)

			c.GetLogger().PrintlnWithOptions(
				level.Error,
				cE.Error(),
				sypl.WithFields(respFields),
				sypl.WithTags("request"),
			)

			return cE
		}

		//////
		// Handles HTTP status codes, and retries.
		//////

		// 429 - Retry after at least 1 second; avoid bursts of requests
		// 4xx - Do not retry
		// 5xx - Retry 3 times with 5, 10, 15 second pause between retries.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			c.counterRetried.Add(1)

			var cE error

			if resp.Body != nil {
				var body []byte

				body, err := shared.ReadAll(resp.Body)
				if err != nil {
					return err
				}

				cE = customerror.NewFailedToError(
					fmt.Sprintf("request (%s). It may be %s, depending on the error and status code) %s",
						http.StatusText(resp.StatusCode),
						status.Retried,
						url,
					),
					customerror.WithStatusCode(resp.StatusCode),
					customerror.WithError(errors.New(string(body))),
				)
			} else {
				cE = customerror.NewFailedToError(
					fmt.Sprintf("request (%s). It may be %s, depending on the error and status code) %s",
						http.StatusText(resp.StatusCode),
						status.Retried,
						url,
					),
					customerror.WithStatusCode(resp.StatusCode),
				)
			}

			c.GetLogger().PrintlnWithOptions(
				level.Error,
				cE.Error(),
				sypl.WithFields(respFields),
				sypl.WithTags("request"),
			)

			return cE
		}

		return nil
	}); err != nil {
		return nil, err
	}

	// If 2xx neither 4xx, return an error with the status code.
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, customerror.NewHTTPError(resp.StatusCode)
	}

	c.counterSuccess.Add(1)

	//////
	// Handles response body.
	//////

	if options.RespBody != nil {
		if err := shared.Decode(resp.Body, options.RespBody); err != nil {
			return resp, err
		}

		respFields["respBody"] = fmt.Sprintf("%+v", options.RespBody)
	}

	respFields["status"] = resp.StatusCode

	c.GetLogger().PrintlnWithOptions(
		level.Info,
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
//nolint:lll
func New(
	name string,
	headers map[string]string,
	timeout time.Duration,
	retrierBackoffDuration time.Duration,
	retrierBackoffTimes int,
) *Client {
	// Enforces IHTTP interface implementation.
	var (
		_      IHTTP = (*Client)(nil)
		client *Client
	)

	once.Do(func() {
		logger := logging.Get().New(name).SetTags(shared.PackageName, name)

		client = &Client{
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
			panic(err)
		}

		client.GetLogger().PrintlnWithOptions(
			level.Debug,
			fmt.Sprintf("%+v %s %s", client.GetName(), shared.PackageName, status.Created),
			sypl.WithTags(shared.PackageName, status.Initialized.String(), client.GetName()),
		)

		singleton = client
	})

	return singleton
}

// NewDefault is like new, but uses the default values.
func NewDefault(name string) *Client {
	return New(name, map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/json",
		"User-Agent":   name,
	}, 0, 0, 0)
}
