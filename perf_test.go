package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keep-alive: sequential, fully-drained requests on one client must REUSE the
// TCP connection (not open a fresh one each time). Distinct RemoteAddr count ==
// distinct connections. Fails while req.Close=true is set (n connections).
func TestKeepAlive_ConnectionReuse(t *testing.T) {
	var mu sync.Mutex
	conns := map[string]struct{}{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		conns[r.RemoteAddr] = struct{}{}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("keepalive"))

	const n = 5
	for range n {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := c.Get(ctx, srv.URL)
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body) // drain so the conn is reusable
		_ = resp.Body.Close()
		cancel()
	}

	mu.Lock()
	got := len(conns)
	mu.Unlock()

	t.Logf("%d sequential requests used %d distinct connection(s)", n, got)
	assert.Less(t, got, n, "connections must be reused (keep-alive), not one per request")
}

// Keep-alive across a retry: a retryable error response must be drained so the
// connection is returned to the pool and reused for the retry (proves the
// bounded drain in HandleHTTPResponseError works with keep-alive).
func TestKeepAlive_ReuseAcrossRetry(t *testing.T) {
	var mu sync.Mutex
	conns := map[string]struct{}{}
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		conns[r.RemoteAddr] = struct{}{}
		mu.Unlock()

		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("temporary error body"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("kareuse"))
	c.RetrierBackoffDuration = 100 * time.Millisecond
	c.RetrierBackoffTimes = 3

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.Get(ctx, srv.URL)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	mu.Lock()
	got := len(conns)
	mu.Unlock()

	t.Logf("retry sequence used %d distinct connection(s)", got)
	assert.Equal(t, 1, got, "the retry must reuse the drained connection")
}
