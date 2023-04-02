package httpclient

import (
	"context"
	"net/http"
)

// Delete does a `DELETE` request.
//
// NOTE: If `opt.RespBody` is provided, it will read and decode the body, ALSO
// CLOSING IT. Otherwise, the body will be left open, and returned. In this case
// IT'S THE CALLER'S RESPONSIBILITY TO CLOSE THE BODY.
func (c *Client) Delete(
	ctx context.Context,
	url string,
	o ...Func,
) (*http.Response, error) {
	return c.request(
		ctx,
		http.MethodDelete,
		url,
		o...,
	)
}
