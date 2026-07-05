package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plainReader is a bare io.Reader that http.NewRequest does NOT recognize, so
// it gets no GetBody — exactly the case that could not be replayed on retry.
type plainReader struct {
	data []byte
	pos  int
}

func (r *plainReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// A generic io.Reader request body must survive a retry (re-sent identically).
func TestReqBody_GenericReaderRetries(t *testing.T) {
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

	c := mustClient(t, uniqueName("readerretry"))
	c.RetrierBackoffDuration = 100 * time.Millisecond
	c.RetrierBackoffTimes = 3

	resp, err := c.Post(ctx5(t), srv.URL, WithReqBody(&plainReader{data: []byte("stream-body")}))
	require.NoError(t, err, "a generic io.Reader body must be replayable on retry")
	defer resp.Body.Close()

	require.GreaterOrEqual(t, atomic.LoadInt32(&attempts), int32(2))
	assert.Equal(t, "stream-body", bodies[0])
	assert.Equal(t, "stream-body", bodies[1], "retry must re-send the same buffered body")
}
