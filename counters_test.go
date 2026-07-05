package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A successful request increments only the success counter.
func TestCounters_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("cntok"))
	resp, err := c.Get(ctx5(t), srv.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.EqualValues(t, 1, c.counterSuccess.Value())
	assert.EqualValues(t, 0, c.counterFailed.Value())
}

// A non-retryable failure counts as failed exactly once.
func TestCounters_NonRetryableFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404 — not retryable
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("cnt404"))

	//nolint:bodyclose
	_, err := c.Get(ctx5(t), srv.URL)
	require.Error(t, err)

	assert.EqualValues(t, 1, c.counterFailed.Value(), "non-retryable failure counts once")
	assert.EqualValues(t, 0, c.counterSuccess.Value())
}

// The regression: a request that EXHAUSTS its retries must still count as failed
// exactly once (previously counterFailed stayed 0 while counterRetried grew).
func TestCounters_ExhaustedRetryCountsAsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // 503 — retryable, always
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("cnt503"))
	c.RetrierBackoffDuration = 50 * time.Millisecond
	c.RetrierBackoffTimes = 2

	//nolint:bodyclose
	_, err := c.Get(ctx5(t), srv.URL)
	require.Error(t, err)

	assert.EqualValues(t, 1, c.counterFailed.Value(), "exhausted retries must count as one failure")
	assert.Greater(t, c.counterRetried.Value(), int64(0), "retries should have been counted")
	assert.EqualValues(t, 0, c.counterSuccess.Value())
}

// A response-body decode failure (HTTP succeeded, processing failed) must count
// as failed — previously it counted as neither success nor failure.
func TestCounters_DecodeErrorCountsAsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this-is-not-json"))
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("cntdecode"))

	var out struct {
		Name string `json:"name"`
	}

	//nolint:bodyclose
	_, err := c.Get(ctx5(t), srv.URL, WithRespBody(&out))
	require.Error(t, err)

	assert.EqualValues(t, 1, c.counterFailed.Value(), "decode failure counts as failed")
	assert.EqualValues(t, 0, c.counterSuccess.Value(), "decode failure is not a success")
}

// A preflight failure (empty URL) also counts as failed exactly once.
func TestCounters_PreflightErrorCountsAsFailed(t *testing.T) {
	c := mustClient(t, uniqueName("cntpre"))

	//nolint:bodyclose
	_, err := c.Get(ctx5(t), "")
	require.Error(t, err)

	assert.EqualValues(t, 1, c.counterFailed.Value())
	assert.EqualValues(t, 0, c.counterSuccess.Value())
}
