package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thalesfsp/sypl/level"
)

// With trace logging enabled, the request path builds and logs fields (the
// branch skipped at the default ERROR level). Behavior — decoding + a
// re-readable body — must be identical.
//
// NOTE: sypl outputs are shared across the process, so SetMaxLevel is global;
// the test restores ERROR afterwards to avoid polluting other tests.
func TestRequest_TraceLevelBuildsFieldsAndStillWorks(t *testing.T) {
	payload := `{"name":"tn","version":"tv"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("trace"))
	c.Logger.SetMaxLevel(level.Trace) // force the trace-enabled hot path
	defer c.Logger.SetMaxLevel(level.Error)

	require.True(t, c.traceEnabled())

	var out struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	resp, err := c.Get(ctx5(t), srv.URL,
		WithHeader("X-H", "v"),
		WithReqBody("body"),
		WithRespBody(&out),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "tn", out.Name)
	assert.Equal(t, "tv", out.Version)

	// Body still re-readable after decode.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(body))
}

// At ERROR level the trace path is skipped.
func TestTraceEnabled_ErrorLevelIsFalse(t *testing.T) {
	c := mustClient(t, uniqueName("notrace"))
	c.Logger.SetMaxLevel(level.Error) // deterministic: shared outputs may be dirty
	assert.False(t, c.traceEnabled())
}
