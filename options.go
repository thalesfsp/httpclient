package httpclient

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"

	"github.com/thalesfsp/customerror"
	"github.com/thalesfsp/httpclient/internal/shared"
)

//////
// Vars, consts, and types.
//////

// Func allows to set options.
type Func func(o *Options) error

// Options contains the fields shared between request's options.
type Options struct {
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
func WithQueryParam(k, v string) Func {
	if k == "" || v == "" {
		return nil
	}

	return func(o *Options) error {
		o.QueryParams[k] = v

		return nil
	}
}

// WithReqBody set the request's body. Processing rule:
//
// - If it's a string, then use it as is.
// - If it's an io.Reader, then use it as is.
// - If it's anything else, then marshal it and use it as is.
func WithReqBody(body interface{}) Func {
	return func(o *Options) error {
		if body == nil {
			return nil
		}

		var bodyReader io.Reader
		switch b := body.(type) {
		case string:
			bodyReader = strings.NewReader(b)
		case io.Reader:
			bodyReader = b
		default:
			bodyBytes, err := shared.Marshal(body)
			if err != nil {
				return customerror.NewFailedToError(
					"marshal reqBody",
					customerror.WithError(err),
				)
			}

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
