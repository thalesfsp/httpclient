package shared

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:noctx // test helper exercising the mock server; context is not relevant here.
func TestCreateHTTPTestServer_HeaderAndQueryEnforcement(t *testing.T) {
	srv := CreateHTTPTestServer(
		http.StatusOK,
		map[string]string{"X-Req": "yes"},
		map[string]string{"q": "1"},
		"hello",
		nil,
	)
	defer srv.Close()

	client := srv.Client()

	t.Run("matching headers and query succeed", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"?q=1", nil)
		require.NoError(t, err)
		req.Header.Set("X-Req", "yes")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "hello", string(body))
	})

	t.Run("missing header is rejected", func(t *testing.T) {
		resp, err := client.Get(srv.URL + "?q=1")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "header not found")
	})

	t.Run("missing query param is rejected", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)
		req.Header.Set("X-Req", "yes")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), "query param not found")
	})
}

//nolint:noctx // test helper exercising the mock server; context is not relevant here.
func TestCreateHTTPTestServer_InjectedError(t *testing.T) {
	srv := CreateHTTPTestServer(
		http.StatusServiceUnavailable,
		nil,
		nil,
		"",
		assert.AnError,
	)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Contains(t, string(body), assert.AnError.Error())
}
