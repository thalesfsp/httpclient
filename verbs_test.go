package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// echoServer returns a server that records the method/body it received and
// replies with the configured status and body.
func echoServer(t *testing.T, status int, respBody string) (*httptest.Server, *recorded) {
	t.Helper()

	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.method = r.Method
		rec.body = string(b)
		w.WriteHeader(status)
		if respBody != "" {
			_, _ = w.Write([]byte(respBody))
		}
	}))
	t.Cleanup(srv.Close)

	return srv, rec
}

type recorded struct {
	method string
	body   string
}

func ctx5(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// verb is a tiny indirection so the happy-path table can exercise every method.
type verb struct {
	name   string
	method string
	call   func(c *Client, ctx context.Context, url string, o ...Func) (*http.Response, error)
}

func allVerbs() []verb {
	return []verb{
		{"GET", http.MethodGet, (*Client).Get},
		{"POST", http.MethodPost, (*Client).Post},
		{"PUT", http.MethodPut, (*Client).Put},
		{"PATCH", http.MethodPatch, (*Client).Patch},
		{"DELETE", http.MethodDelete, (*Client).Delete},
	}
}

// Happy path: every verb reaches the server with the right method and returns 200.
func TestVerbs_HappyPath(t *testing.T) {
	for _, v := range allVerbs() {
		t.Run(v.name, func(t *testing.T) {
			srv, rec := echoServer(t, http.StatusOK, "ok")
			c := mustClient(t, uniqueName("verb"))

			resp, err := v.call(c, ctx5(t), srv.URL, WithReqBody("payload"))
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, v.method, rec.method)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, "ok", string(body))
		})
	}
}

// Bad path: every verb surfaces a non-retryable 4xx as an error.
func TestVerbs_BadRequestIsError(t *testing.T) {
	for _, v := range allVerbs() {
		t.Run(v.name, func(t *testing.T) {
			srv, _ := echoServer(t, http.StatusBadRequest, "nope")
			c := mustClient(t, uniqueName("verberr"))

			//nolint:bodyclose
			resp, err := v.call(c, ctx5(t), srv.URL)
			require.Error(t, err)
			assert.Nil(t, resp)
		})
	}
}

// Edge: a verb decodes into RespBody and the response is consumed correctly.
func TestVerbs_RespBodyDecode(t *testing.T) {
	srv, _ := echoServer(t, http.StatusOK, `{"name":"n","version":"v"}`)
	c := mustClient(t, uniqueName("verbdecode"))

	var out struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	resp, err := c.Put(ctx5(t), srv.URL, WithReqBody("x"), WithRespBody(&out))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "n", out.Name)
	assert.Equal(t, "v", out.Version)
}

// Patch must be reachable through the IHTTP interface (it was previously only on
// the concrete type).
func TestIHTTP_IncludesPatch(t *testing.T) {
	srv, rec := echoServer(t, http.StatusOK, "ok")

	var h IHTTP = mustClient(t, uniqueName("iface"))

	resp, err := h.Patch(ctx5(t), srv.URL, WithReqBody("p"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.MethodPatch, rec.method)
}
