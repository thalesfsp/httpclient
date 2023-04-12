package httpclient

import (
	"context"
	"net/http"

	"github.com/thalesfsp/concurrentloop"
)

// ParallelGet concurrently call GET on the given URLs.
//
// NOTE: Both `Content-shared.PackageName` and `Accept` are already set to `application/json`.
// Change it accordingly to the needs.
//
// WARN: IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
//
//nolint:bodyclose
func (c *Client) ParallelGet(
	ctx context.Context,
	opts []Func,
	urls ...string,
) ([]*http.Response, concurrentloop.Errors) {
	return concurrentloop.Map(ctx, urls, func(ctx context.Context, url string) (*http.Response, error) {
		return c.Get(ctx, url, opts...)
	})
}
