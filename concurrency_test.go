package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrent_SharedClient_NoRace fires many concurrent requests through a
// single shared client. Run with -race, it guards the request hot path
// (setHeaders / setQueryParams / setCustomRetryableRegexes and the expvar
// counters) against data races.
func TestConcurrent_SharedClient_NoRace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("concurrent"))

	const goroutines = 50

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			resp, err := c.Get(
				ctx5(t),
				srv.URL,
				WithHeader(fmt.Sprintf("X-N-%d", i), "v"),
				WithQueryParam(fmt.Sprintf("q%d", i), "v"),
			)
			if err != nil {
				errCh <- err
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	assert.EqualValues(t, goroutines, c.counterSuccess.Value(), "every request should count as success")
}
