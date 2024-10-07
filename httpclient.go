//nolint:lll,wsl,mnd
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

// DefaultMetricCounterLabel is the default metric counter label.
const DefaultMetricCounterLabel = "counter"

var (
	httpRetrierRegex = regexp.MustCompile(`(?m)^(408|429|5\d\d)$`)
	once             sync.Once
	singleton        *Client
)

// Client is the application pre-configured HTTP client.
type Client struct {
	client *http.Client

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

// HandleHTTPResponseError handles the HTTP response error.
func HandleHTTPResponseError(resp *http.Response, url, msg string, externalErr error) error {
	customerrorOptions := []customerror.Option{
		customerror.WithField("url", url),
	}

	if externalErr != nil {
		customerrorOptions = append(customerrorOptions, customerror.WithError(externalErr))
	}

	if resp != nil && resp.StatusCode > 0 {
		customerrorOptions = append(customerrorOptions, customerror.WithStatusCode(resp.StatusCode))

		if resp.Body != nil {
			body, err := shared.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			customerrorOptions = append(customerrorOptions, customerror.WithField("respBody", string(body)))
		}
	}

	return customerror.NewFailedToError(msg, customerrorOptions...)
}

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

// request is the base request method.
func (c *Client) request(ctx context.Context, method string, url string, o ...Func) (*http.Response, error) {
	if method == "" || url == "" {
		return nil, customerror.NewRequiredError("method and url are")
	}

	options := &Options{
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	for _, opt := range o {
		if err := opt(options); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, options.reqBodyAsIOReader)
	if err != nil {
		return nil, customerror.NewFailedToError("create request", customerror.WithError(err))
	}

	c.setQueryParams(req, options.QueryParams)
	c.setHeaders(req, options.Headers)

	reqFields := c.prepareRequestFields(method, url, req, options)
	respFields := make(fields.Fields, len(reqFields))
	for k, v := range reqFields {
		respFields[k] = v
	}

	c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Created.String(), sypl.WithFields(reqFields))

	r := retrier.New(
		retrier.ExponentialBackoff(c.RetrierBackoffTimes, c.RetrierBackoffDuration),
		HTTPStatusCodeClassifier{Regex: httpRetrierRegex, Client: c},
	)

	var resp *http.Response
	err = r.Run(func() error {
		req.Close = true

		respInternal, err := c.client.Do(req)
		if err != nil {
			return HandleHTTPResponseError(respInternal, url, "request", err)
		}

		if respInternal != nil && respInternal.StatusCode > 0 {
			if httpRetrierRegex.MatchString(strconv.Itoa(respInternal.StatusCode)) {
				respInternal.Body.Close() // Close the body before returning the error

				return HandleHTTPResponseError(respInternal, url, "request", err)
			}

			if respInternal.StatusCode < http.StatusOK || respInternal.StatusCode >= http.StatusBadRequest {
				respInternal.Body.Close() // Close the body before returning the error

				return HandleHTTPResponseError(respInternal, url, "request", err)
			}
		}

		c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Succeeded.String(), sypl.WithFields(reqFields))

		resp = respInternal

		return nil
	})
	if err != nil {
		return nil, err
	}

	if resp != nil && options.RespBody != nil && resp.Body != nil {
		defer resp.Body.Close()
		if err := c.handleResponseBody(resp, options.RespBody, &respFields); err != nil {
			return nil, err
		}
	}

	c.counterSuccess.Add(1)
	c.GetLogger().PrintlnWithOptions(level.Trace, "request "+status.Succeeded.String(), sypl.WithFields(respFields), sypl.WithTags("request"))

	return resp, nil
}

func (c *Client) setQueryParams(req *http.Request, queryParams map[string]string) {
	if len(queryParams) > 0 {
		q := req.URL.Query()
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("query params")}

		for k, v := range queryParams {
			q.Add(k, v)
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}

		c.GetLogger().PrintlnWithOptions(level.Trace, "query params", loggingOpts...)
		req.URL.RawQuery = q.Encode()
	}
}

func (c *Client) setHeaders(req *http.Request, headers map[string]string) {
	if c.Headers != nil {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("default headers")}
		for k, v := range c.Headers {
			req.Header.Set(k, v)
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}
		c.GetLogger().PrintlnWithOptions(level.Trace, "default headers", loggingOpts...)
	}

	if headers != nil {
		loggingOpts := []sypl.OptionFunc{sypl.WithTags("per-request headers")}
		for k, v := range headers {
			req.Header.Set(k, v)
			loggingOpts = append(loggingOpts, sypl.WithField(k, v))
		}
		c.GetLogger().PrintlnWithOptions(level.Trace, "per-request headers", loggingOpts...)
	}
}

func (c *Client) prepareRequestFields(method, url string, req *http.Request, options *Options) fields.Fields {
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

	return logging.ToAPM(req.Context(), reqFields)
}

func (c *Client) handleResponseBody(resp *http.Response, respBody interface{}, respFields *fields.Fields) error {
	switch v := respBody.(type) {
	case *os.File:
		if _, err := io.Copy(v, resp.Body); err != nil {
			return err
		}
	default:
		if err := shared.Decode(resp.Body, respBody); err != nil {
			return err
		}
		(*respFields)["respBody"] = fmt.Sprintf("%+v", respBody)
	}

	if resp.StatusCode > 0 {
		(*respFields)["respStatusCode"] = resp.StatusCode
	}

	return nil
}

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

// New creates a new HTTP client.
func New(name string, headers map[string]string, timeout time.Duration, retrierBackoffDuration time.Duration, retrierBackoffTimes int) (*Client, error) {
	logger := logging.Get().New(name).SetTags(shared.PackageName, name)

	client := &Client{
		client: &http.Client{
			Timeout: timeout,
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

	if retrierBackoffDuration == 0 {
		client.RetrierBackoffDuration = time.Second
	}

	if retrierBackoffTimes == 0 {
		client.RetrierBackoffTimes = 3
	}

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
func NewSingleton(name string, headers map[string]string, timeout time.Duration, retrierBackoffDuration time.Duration, retrierBackoffTimes int) (*Client, error) {
	var errOut error

	once.Do(func() {
		c, err := New(name, headers, timeout, retrierBackoffDuration, retrierBackoffTimes)
		if err != nil {
			errOut = err

			return
		}

		singleton = c
	})
	if errOut != nil {
		return nil, errOut
	}

	return singleton, nil
}

// NewDefaultSingleton creates a new default HTTP client, and sets it as the singleton.
func NewDefaultSingleton(name string) (*Client, error) {
	var errOut error

	once.Do(func() {
		c, err := NewDefault(name)
		if err != nil {
			errOut = err

			return
		}

		singleton = c
	})
	if errOut != nil {
		return nil, errOut
	}

	return singleton, nil
}

// NewDefault is like new, but uses the default values.
func NewDefault(name string) (*Client, error) {
	return New(name, map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/json",
		"User-Agent":   name,
	}, 30*time.Second, time.Second, 3)
}

// Initialize setup the HTTP client. Use options to customize it.
func Initialize(opts ...ClientFunc) (*Client, error) {
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
