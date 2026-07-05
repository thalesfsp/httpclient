package httpclient

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/thalesfsp/customerror"
	"github.com/thalesfsp/httpclient/v2/internal/logging"
	"github.com/thalesfsp/httpclient/v2/internal/shared"
)

//////
// Vars, consts, and types.
//////

// Func allows to set options.
type Func func(o *Options) error

// Options contains the fields shared between request's options.
type Options struct {
	// Custom classifier regexes.
	CustomRetrierRegexes []*regexp.Regexp

	// Headers of the request.
	Headers map[string]string `json:"headers"`

	// QueryParams of the request.
	QueryParams map[string]string `json:"queryParams"`

	// ReqBody is the request body.
	ReqBody any `json:"reqBody"`

	// RespBody is the response body.
	RespBody any `json:"respBody"`

	reqBodyAsIOReader io.Reader `json:"-"`
}

//////
// Exported built-in options.
//////

// WithCustomRetrierRegexes allows to set custom retrier regexes applied against
// the error message.
func WithCustomRetrierRegexes(regexes ...*regexp.Regexp) Func {
	return func(o *Options) error {
		// Initialize if needed.
		if o.CustomRetrierRegexes == nil {
			o.CustomRetrierRegexes = make([]*regexp.Regexp, 0)
		}

		// Append the regexes.
		o.CustomRetrierRegexes = append(o.CustomRetrierRegexes, regexes...)

		return nil
	}
}

// WithHeader add a key value pair to the request's headers.
func WithHeader(k, v string) Func {
	return func(o *Options) error {
		if k == "" || v == "" {
			return nil
		}

		o.Headers[k] = v

		return nil
	}
}

// WithBearerAuthToken set the bearer auth token for the request.
func WithBearerAuthToken(token string) Func {
	return func(o *Options) error {
		if token == "" {
			return nil
		}

		o.Headers["Authorization"] = "Bearer " + token

		return nil
	}
}

// WithBasicAuth set the basic auth for the request.
func WithBasicAuth(username, password string) Func {
	return func(o *Options) error {
		if username == "" || password == "" {
			return nil
		}

		o.Headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString(
			[]byte(username+":"+password),
		)

		return nil
	}
}

// WithQueryParam add a key value pair to the request's query params.
//
// A nil-safe no-op when either key or value is empty (mirrors WithHeader).
func WithQueryParam(k, v string) Func {
	return func(o *Options) error {
		if k == "" || v == "" {
			return nil
		}

		o.QueryParams[k] = v

		return nil
	}
}

// WithReqBody set the request's body. Processing rule:
//
//   - If it's a string, then use it as is.
//   - If it's an io.Reader, then it is buffered into memory so the request stays
//     replayable on retry (a raw reader has no GetBody and cannot be rewound).
//     This trades streaming for retryability, which is the point of this client.
//   - If it's anything else, then marshal it and use it as is.
func WithReqBody(body interface{}) Func {
	return func(o *Options) error {
		if body == nil {
			return nil
		}

		var bodyReader io.Reader

		switch b := body.(type) {
		case string:
			logging.Get().Debuglnf("request body: %s", b)

			bodyReader = strings.NewReader(b)
		case io.Reader:
			// Buffer the reader so the body can be replayed on retry.
			data, err := io.ReadAll(b)
			if err != nil {
				return customerror.NewFailedToError(
					"read reqBody reader",
					customerror.WithError(err),
				)
			}

			logging.Get().Debuglnf("request body: %s", string(data))

			bodyReader = bytes.NewReader(data)
		case url.Values:
			logging.Get().Debuglnf("request body: %s", b.Encode())

			bodyReader = strings.NewReader(b.Encode())

			// Initialize o.Headers if needed.
			if o.Headers == nil {
				o.Headers = make(map[string]string)
			}
		default:
			bodyBytes, err := shared.Marshal(body)
			if err != nil {
				return customerror.NewFailedToError(
					"marshal reqBody",
					customerror.WithError(err),
				)
			}

			logging.Get().Debuglnf("request body: %s", string(bodyBytes))

			bodyReader = bytes.NewReader(bodyBytes)
		}

		o.reqBodyAsIOReader = bodyReader

		return nil
	}
}

// WithRespBody set the response's body.
func WithRespBody(body interface{}) Func {
	return func(o *Options) error {
		if body == nil {
			return nil
		}

		o.RespBody = body

		return nil
	}
}

//////
// Client's specific options.
//////

// ClientOptions options specific to setting up the client.
type ClientOptions struct {
	// Custom classifier regexes.
	CustomRetrierRegexes []*regexp.Regexp

	// Name of the HTTP client.
	Name string

	// QueryParams of the client.
	QueryParams map[string]string

	// Headers of the client.
	Headers map[string]string
}

// ClientFunc defines the function signature for setting up the client.
type ClientFunc func(o *ClientOptions) error

// WithClientName set the name of the HTTP client.
func WithClientName(name string) ClientFunc {
	return func(o *ClientOptions) error {
		o.Name = name

		return nil
	}
}

// WithPrefix sets the prefix of the HTTP metrics.
//
// WARN: this sets a PROCESS-GLOBAL environment variable
// (HTTPCLIENT_METRICS_PREFIX) read at metric-creation time, so it affects every
// client created afterwards and is not safe to call concurrently with client
// construction. Set it once at startup, before creating clients.
func WithPrefix(prefix string) ClientFunc {
	return func(_ *ClientOptions) error {
		os.Setenv("HTTPCLIENT_METRICS_PREFIX", prefix)

		return nil
	}
}

// WithClientCustomRetrierRegexes allows to set custom retrier regexes applied against
// the error message.
func WithClientCustomRetrierRegexes(regexes ...*regexp.Regexp) ClientFunc {
	return func(o *ClientOptions) error {
		// Initialize if needed.
		if o.CustomRetrierRegexes == nil {
			o.CustomRetrierRegexes = make([]*regexp.Regexp, 0)
		}

		// Append the regexes.
		o.CustomRetrierRegexes = append(o.CustomRetrierRegexes, regexes...)

		return nil
	}
}

// WithClientQueryParam allows to set a default query param for the client.
func WithClientQueryParam(k, v string) ClientFunc {
	return func(o *ClientOptions) error {
		// Initialize o.QueryParams if needed.
		if o.QueryParams == nil {
			o.QueryParams = make(map[string]string)
		}

		if k != "" || v != "" {
			o.QueryParams[k] = v
		}

		return nil
	}
}

// WithClientHeader allows to set a default header for the client.
func WithClientHeader(k, v string) ClientFunc {
	return func(o *ClientOptions) error {
		// Initialize o.Headers if needed.
		if o.Headers == nil {
			o.Headers = make(map[string]string)
		}

		if k != "" || v != "" {
			o.Headers[k] = v
		}

		return nil
	}
}
