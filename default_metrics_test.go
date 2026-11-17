package httpclient

import (
	"expvar"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func countClientMetrics(prefix string) int {
	n := 0
	expvar.Do(func(kv expvar.KeyValue) {
		if strings.HasPrefix(kv.Key, prefix+".httpclient.") {
			n++
		}
	})

	return n
}

// The previous same-name regression missed the default UUID name: every
// construction permanently published three new metrics, even after GC.
func TestNewDefault_MetricsBounded(t *testing.T) {
	tests := []struct {
		name string
		opts []ClientFunc
	}{
		{name: "unnamed"},
		{name: "non-name options", opts: []ClientFunc{
			WithClientHeader("X-Test", "yes"), WithClientQueryParam("q", "v"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix := uniqueName("defaultmetrics")
			t.Setenv("HTTPCLIENT_METRICS_PREFIX", prefix)
			for range 1000 {
				c, err := NewDefault(tt.opts...)
				require.NoError(t, err)
				assert.Len(t, c.Name, 47, "keep the unique default client name")
				assert.Equal(t, c.Name, c.Headers["User-Agent"])
			}
			assert.Equal(t, 3, countClientMetrics(prefix), "construction must not grow the global metric registry")
		})
	}
}

func TestNewDefault_MetricsAggregateWithoutNamedCollision(t *testing.T) {
	prefix := uniqueName("aggregate")
	t.Setenv("HTTPCLIENT_METRICS_PREFIX", prefix)
	a, err := NewDefault()
	require.NoError(t, err)
	b, err := NewDefault(WithClientHeader("X-Test", "yes"))
	require.NoError(t, err)
	named, err := NewDefault(WithClientName("default"))
	require.NoError(t, err)
	legacy, err := New("default", nil, nil, nil, time.Second, time.Second, 3)
	require.NoError(t, err)

	assert.NotEqual(t, a.Name, b.Name)
	assert.Same(t, a.counterSuccess, b.counterSuccess)
	assert.Same(t, a.counterFailed, b.counterFailed)
	assert.Same(t, a.counterRetried, b.counterRetried)
	assert.NotSame(t, a.counterSuccess, named.counterSuccess)
	assert.Same(t, named.counterSuccess, legacy.counterSuccess)
	assert.Same(t, named.counterSuccess, expvar.Get(prefix+".httpclient.default.succeeded.counter"))
	assert.Equal(t, 6, countClientMetrics(prefix))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, a.Name, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	resp, err := a.Get(ctx5(t), srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, 1, b.counterSuccess.Value(), "unnamed clients aggregate actual requests")
	assert.Zero(t, named.counterSuccess.Value(), "named clients remain independent")
}

func TestNewDefaultSingleton_ConcurrentMetricsBounded(t *testing.T) {
	previous := singleton.Load()
	singleton.Store(nil)
	t.Cleanup(func() { singleton.Store(previous) })
	prefix := uniqueName("singletonmetrics")
	t.Setenv("HTTPCLIENT_METRICS_PREFIX", prefix)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	const n = 64
	var ready, finished sync.WaitGroup
	ready.Add(n)
	finished.Add(n)
	release := make(chan struct{})
	clients := make([]*Client, n)
	errs := make([]error, n)
	for i := range n {
		go func() {
			defer finished.Done()
			clients[i], errs[i] = NewDefaultSingleton(func(_ *ClientOptions) error {
				// All constructors must pass the fast path before any can
				// publish a winner; otherwise the losing-client leak is missed.
				ready.Done()
				<-release

				return nil
			})
			if errs[i] == nil {
				resp, err := clients[i].Get(ctx5(t), srv.URL)
				errs[i] = err
				if resp != nil {
					_ = resp.Body.Close()
				}
			}
		}()
	}
	ready.Wait()
	close(release)
	finished.Wait()
	for i := range n {
		require.NoError(t, errs[i])
		assert.Same(t, clients[0], clients[i])
	}
	assert.Equal(t, 3, countClientMetrics(prefix), "discarded first-initializers must not publish unique metrics")
	assert.EqualValues(t, n, clients[0].counterSuccess.Value(), "creation and requests safely share counters")
}
