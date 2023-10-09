package httpclient

import (
	"context"
	"net/http"
)

// RequestResult is the result of a request.
type RequestResult struct {
	Error    error
	Response *http.Response
}

// ParallelGetResult is the result of the `ParallelGet` operation.
type ParallelGetResult = []RequestResult

// IHTTP is the interface for the HTTP client.
type IHTTP interface {
	// GetClient returns the underlying HTTP client.
	GetClient() *http.Client

	// Get returns a new GetRequest.
	//
	// NOTE: If `opt.RespBody` is provided, it will read and decode the body,
	// also closing it. Otherwise, the body will be left open, and returned.
	// In this case IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
	Get(ctx context.Context, url string, o ...Func) (*http.Response, error)

	// Post does a `POST` request.
	//
	// NOTE: If `opt.RespBody` is provided, it will read and decode the body,
	// ALSO CLOSING IT. Otherwise, the body will be left open, and returned. In
	// this case IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
	Post(ctx context.Context, url string, o ...Func) (*http.Response, error)

	// Put does a `PUT` request.
	//
	// NOTE: If `opt.RespBody` is provided, it will read and decode the body,
	// ALSO CLOSING IT. Otherwise, the body will be left open, and returned. In
	// this case IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
	Put(ctx context.Context, url string, o ...Func) (*http.Response, error)

	// Delete does a `DELETE` request.
	//
	// NOTE: If `opt.RespBody` is provided, it will read and decode the body,
	// ALSO CLOSING IT. Otherwise, the body will be left open, and returned. In
	// this case IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
	Delete(ctx context.Context, url string, o ...Func) (*http.Response, error)
}
