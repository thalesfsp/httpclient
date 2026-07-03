package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uniqueName returns a process-unique client name so tests don't collide on
// expvar metric registration.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// BUG 1 regression: WithQueryParam must never return a nil Func, even when the
// key or value is empty. A nil Func panics when applied in request().
func TestBug_WithQueryParam_NeverNilFunc(t *testing.T) {
	// Unit level: the option constructor must return a usable (non-nil) Func.
	assert.NotNil(t, WithQueryParam("", "v"), "empty key")
	assert.NotNil(t, WithQueryParam("k", ""), "empty value")
	assert.NotNil(t, WithQueryParam("", ""), "both empty")
	assert.NotNil(t, WithQueryParam("k", "v"), "both set")

	// Applying the empty option must be a safe no-op (no panic, no entry).
	o := &Options{QueryParams: map[string]string{}}
	require.NotPanics(t, func() {
		_ = WithQueryParam("", "v")(o)
	})
	assert.Empty(t, o.QueryParams, "empty key/value must not add a query param")

	// Integration level: a request using an empty WithQueryParam must not panic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("bug1"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NotPanics(t, func() {
		//nolint:bodyclose
		resp, err := c.Get(ctx, srv.URL, WithQueryParam("", "v"))
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	})
}

// BUG 2 regression: when WithRespBody decodes the body, the returned resp.Body
// must still be readable with the ORIGINAL bytes (the code claims to reset it).
func TestBug_RespBody_RemainsReadable(t *testing.T) {
	payload := `{"name":"test","version":"1.0.0"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("bug2"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var out struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	resp, err := c.Get(ctx, srv.URL, WithRespBody(&out))
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	// Decode worked.
	assert.Equal(t, "test", out.Name)
	assert.Equal(t, "1.0.0", out.Version)

	// The reset body must contain the original payload, not be empty.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(body), "resp.Body must be re-readable after WithRespBody")
}

// BUG 3 regression: a POST with a body that hits a retryable status must retry
// and re-send the SAME body, not die with a ContentLength mismatch.
func TestBug_Retry_ResendsBody(t *testing.T) {
	var attempts int32
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // retryable
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("bug3"))
	c.RetrierBackoffDuration = 100 * time.Millisecond
	c.RetrierBackoffTimes = 3

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.Post(ctx, srv.URL, WithReqBody("hello-body"))
	require.NoError(t, err, "retry with a body must succeed")
	require.NotNil(t, resp)
	defer resp.Body.Close()

	require.GreaterOrEqual(t, atomic.LoadInt32(&attempts), int32(2), "server must see the retry")
	assert.Equal(t, "hello-body", bodies[0], "first attempt body")
	assert.Equal(t, "hello-body", bodies[1], "retried attempt must re-send the same body")
}

// BUG 4 regression: creating two clients with the same name (even within the
// same second) must not panic on expvar metric registration.
func TestBug_SameName_NoMetricPanic(t *testing.T) {
	name := uniqueName("bug4dup")

	require.NotPanics(t, func() {
		c1, err := New(name, nil, nil, nil, time.Second, time.Second, 3)
		require.NoError(t, err)
		require.NotNil(t, c1)

		c2, err := New(name, nil, nil, nil, time.Second, time.Second, 3)
		require.NoError(t, err)
		require.NotNil(t, c2)
	}, "two clients with the same name must not panic")
}

// mustClient builds a default client or fails the test.
func mustClient(t *testing.T, name string) *Client {
	t.Helper()
	c, err := NewDefault(WithClientName(name))
	require.NoError(t, err)
	return c
}
