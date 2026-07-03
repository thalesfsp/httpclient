package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_RequiresMethodAndURL(t *testing.T) {
	c := mustClient(t, uniqueName("req"))

	//nolint:bodyclose
	_, err := c.request(ctx5(t), "", "http://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	//nolint:bodyclose
	_, err = c.Get(ctx5(t), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestRequest_OptionErrorPropagates(t *testing.T) {
	c := mustClient(t, uniqueName("reqopt"))
	boom := func(_ *Options) error { return errors.New("option boom") }

	//nolint:bodyclose
	_, err := c.Get(ctx5(t), "http://example.com", boom)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "option boom")
}

func TestRequest_InvalidURL(t *testing.T) {
	c := mustClient(t, uniqueName("requrl"))

	//nolint:bodyclose
	_, err := c.Get(ctx5(t), "http://\x7f-bad-url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create request")
}

func TestRequest_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("reqctx"))
	c.RetrierBackoffDuration = 100 * time.Millisecond
	c.RetrierBackoffTimes = 1

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	//nolint:bodyclose
	_, err := c.Get(ctx, srv.URL)
	require.Error(t, err)
}

func TestRequest_RespBodyToFile(t *testing.T) {
	payload := "file-destined-content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	c := mustClient(t, uniqueName("reqfile"))

	path := filepath.Join(t.TempDir(), "out.txt")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	resp, err := c.Get(ctx5(t), srv.URL, WithRespBody(f))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.NoError(t, f.Close())
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, payload, string(got), "response body should be streamed into the file")
}

func TestHandleHTTPResponseError(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		err := HandleHTTPResponseError(nil, "http://u", "msg", errors.New("boom"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("with body is captured and reset", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(newStringReader("server-body")),
		}
		err := HandleHTTPResponseError(resp, "http://u", "msg", nil)
		require.Error(t, err)
		// The body must be reset so it can be read again.
		body, rerr := io.ReadAll(resp.Body)
		require.NoError(t, rerr)
		assert.Equal(t, "server-body", string(body))
	})

	t.Run("nil external error", func(t *testing.T) {
		err := HandleHTTPResponseError(nil, "http://u", "msg", nil)
		require.Error(t, err)
	})
}

func TestIsRespSuccess_Table(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{http.StatusOK, true},
		{http.StatusNoContent, true},
		{http.StatusMultipleChoices, false}, // 300
		{http.StatusBadRequest, false},
		{http.StatusInternalServerError, false},
		{199, false},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, IsRespSuccess(&http.Response{StatusCode: tc.code}), "code %d", tc.code)
	}
}

func TestSetHeadersAndQueryParams_Defaults(t *testing.T) {
	var gotHeader, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Default")
		gotQuery = r.URL.Query().Get("dq")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := NewDefault(
		WithClientName(uniqueName("defs")),
		WithClientHeader("X-Default", "dh"),
		WithClientQueryParam("dq", "dv"),
	)
	require.NoError(t, err)

	resp, err := c.Get(ctx5(t), srv.URL, WithHeader("X-Req", "rh"), WithQueryParam("rq", "rv"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "dh", gotHeader)
	assert.Equal(t, "dv", gotQuery)
}

func TestPrepareRequestFields(t *testing.T) {
	c := mustClient(t, uniqueName("fields"))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://u?a=b", nil)
	require.NoError(t, err)

	opts := &Options{
		Headers: map[string]string{"X": "y"},
		ReqBody: "the-body",
	}
	f := c.prepareRequestFields(http.MethodGet, "http://u?a=b", req, opts)
	assert.Equal(t, http.MethodGet, f["method"])
	assert.Equal(t, "http://u?a=b", f["url"])
	assert.Equal(t, "the-body", f["reqBody"])
	assert.Equal(t, "a=b", f["reqQueryParams"])
}

// newStringReader avoids importing strings in multiple files.
func newStringReader(s string) io.Reader { return &sliceReader{data: []byte(s)} }

type sliceReader struct {
	data []byte
	pos  int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
