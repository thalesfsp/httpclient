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

	for i := range goroutines {
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

// TestConcurrent_NilSharedFields_NoRace is the regression test for the latent
// data race: with nil Headers/QueryParams/CustomRetrierRegexes, the request
// path must NOT lazily write those shared fields (which raced). Under -race
// this fails if the lazy-init blocks are reintroduced.
func TestConcurrent_NilSharedFields_NoRace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("nilfields"))

	// Force the shared fields to nil to exercise the (formerly racy) path.
	c.Headers = nil
	c.QueryParams = nil
	c.CustomRetrierRegexes = nil

	const goroutines = 50

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(ctx5(t), srv.URL)
			if err == nil && resp != nil {
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	// The shared fields must remain nil — the request path must not mutate them.
	assert.Nil(t, c.Headers)
	assert.Nil(t, c.QueryParams)
	assert.Nil(t, c.CustomRetrierRegexes)
}

// TestConcurrent_SingletonReadNoRace exercises Get() (a lockless read)
// concurrently with the singleton Store performed by NewDefaultSingleton. Under
// -race this fails if `singleton` is a plain *Client instead of an atomic.Pointer.
func TestConcurrent_SingletonReadNoRace(t *testing.T) {
	singleton.Store(nil)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = NewDefaultSingleton(WithClientName(uniqueName("racy")))
	}()
	go func() {
		defer wg.Done()
		defer func() { _ = recover() }() // may read nil before the Store — fine
		_ = Get()
	}()

	wg.Wait()

	// After both goroutines, the singleton is initialized and readable.
	assert.NotNil(t, Get())
}
