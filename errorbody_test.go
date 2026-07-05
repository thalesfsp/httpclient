package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HandleHTTPResponseError must cap how much of a huge error response body it
// reads into the error context (and the body it resets for re-reading), so a
// giant error payload multiplied by retries cannot blow up memory. It must also
// NOT truncate bodies at or below the cap.
//
// Negative control: delete the io.LimitReader in HandleHTTPResponseError and the
// "huge body capped" case fails — the full 2 MiB is read back instead of exactly
// maxErrorBodyBytes.
func TestHandleHTTPResponseError_BoundsBodySize(t *testing.T) {
	tests := []struct {
		name    string
		bodyLen int
		wantLen int
	}{
		{name: "small body preserved", bodyLen: 128, wantLen: 128},
		{name: "exactly at cap", bodyLen: maxErrorBodyBytes, wantLen: maxErrorBodyBytes},
		{name: "huge body capped", bodyLen: 2 * maxErrorBodyBytes, wantLen: maxErrorBodyBytes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("a", tt.bodyLen))),
			}

			err := HandleHTTPResponseError(resp, "http://example.test", "boom", nil)
			require.Error(t, err)

			// The body was reset to exactly the bounded read — proving the cap
			// (and that sub-cap bodies are preserved whole).
			got, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)
			assert.Len(t, got, tt.wantLen)
		})
	}
}
