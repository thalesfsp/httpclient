package httpclient

import (
	"encoding/base64"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applyOpts(t *testing.T, opts ...Func) *Options {
	t.Helper()
	o := &Options{Headers: map[string]string{}, QueryParams: map[string]string{}}
	for _, opt := range opts {
		require.NotNil(t, opt, "option constructor must never return a nil Func")
		require.NoError(t, opt(o))
	}
	return o
}

func TestWithHeader(t *testing.T) {
	o := applyOpts(t, WithHeader("X-A", "1"))
	assert.Equal(t, "1", o.Headers["X-A"])

	// Empty key or value is a no-op.
	o = applyOpts(t, WithHeader("", "1"), WithHeader("X-B", ""))
	assert.Empty(t, o.Headers)
}

func TestWithQueryParam_NoOpAndSet(t *testing.T) {
	o := applyOpts(t, WithQueryParam("k", "v"))
	assert.Equal(t, "v", o.QueryParams["k"])

	o = applyOpts(t, WithQueryParam("", "v"), WithQueryParam("k", ""))
	assert.Empty(t, o.QueryParams)
}

func TestWithBearerAuthToken(t *testing.T) {
	o := applyOpts(t, WithBearerAuthToken("abc123"))
	assert.Equal(t, "Bearer abc123", o.Headers["Authorization"])

	// Empty token is a no-op.
	o = applyOpts(t, WithBearerAuthToken(""))
	assert.Empty(t, o.Headers["Authorization"])
}

func TestWithBasicAuth(t *testing.T) {
	o := applyOpts(t, WithBasicAuth("john", "doe"))
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("john:doe"))
	assert.Equal(t, want, o.Headers["Authorization"])

	// Missing username or password is a no-op.
	o = applyOpts(t, WithBasicAuth("", "doe"))
	assert.Empty(t, o.Headers["Authorization"])
	o = applyOpts(t, WithBasicAuth("john", ""))
	assert.Empty(t, o.Headers["Authorization"])
}

func TestWithRespBody(t *testing.T) {
	var target struct{ A string }
	o := applyOpts(t, WithRespBody(&target))
	assert.Same(t, &target, o.RespBody)

	// nil body is a no-op.
	o = applyOpts(t, WithRespBody(nil))
	assert.Nil(t, o.RespBody)
}

func TestWithReqBody_MarshalError(t *testing.T) {
	// A channel cannot be JSON-marshaled → the default branch errors.
	o := &Options{Headers: map[string]string{}}
	err := WithReqBody(make(chan int))(o)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal reqBody")
}

func TestWithReqBody_NilNoOp(t *testing.T) {
	o := &Options{}
	require.NoError(t, WithReqBody(nil)(o))
	assert.Nil(t, o.reqBodyAsIOReader)
}

func TestWithCustomRetrierRegexes(t *testing.T) {
	r1 := regexp.MustCompile(`foo`)
	r2 := regexp.MustCompile(`bar`)
	o := applyOpts(t, WithCustomRetrierRegexes(r1), WithCustomRetrierRegexes(r2))
	require.Len(t, o.CustomRetrierRegexes, 2)
}

func TestClientOptions(t *testing.T) {
	co := &ClientOptions{}

	require.NoError(t, WithClientName("n")(co))
	assert.Equal(t, "n", co.Name)

	require.NoError(t, WithClientHeader("H", "v")(co))
	assert.Equal(t, "v", co.Headers["H"])

	require.NoError(t, WithClientQueryParam("q", "v")(co))
	assert.Equal(t, "v", co.QueryParams["q"])

	r := regexp.MustCompile(`x`)
	require.NoError(t, WithClientCustomRetrierRegexes(r)(co))
	require.Len(t, co.CustomRetrierRegexes, 1)
}

func TestWithPrefix_SetsEnv(t *testing.T) {
	t.Setenv("HTTPCLIENT_METRICS_PREFIX", "")
	co := &ClientOptions{}
	require.NoError(t, WithPrefix("myprefix")(co))
	assert.Equal(t, "myprefix", os.Getenv("HTTPCLIENT_METRICS_PREFIX"))
}
