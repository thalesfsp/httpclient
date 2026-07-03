package httpclient

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thalesfsp/customerror"
)

// TestErrorByStatusCode_AllMappedCodes walks every status code the catalog maps
// and asserts the returned error carries that status code.
func TestErrorByStatusCode_AllMappedCodes(t *testing.T) {
	codes := []int{
		// 4xx
		http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge,
		http.StatusExpectationFailed, http.StatusFailedDependency, http.StatusForbidden,
		http.StatusGone, http.StatusLengthRequired, http.StatusLocked,
		http.StatusMethodNotAllowed, http.StatusMisdirectedRequest, http.StatusNotAcceptable,
		http.StatusUnauthorized, http.StatusNotFound, http.StatusNotImplemented,
		http.StatusPaymentRequired, http.StatusPreconditionFailed, http.StatusPreconditionRequired,
		http.StatusProxyAuthRequired, http.StatusRequestedRangeNotSatisfiable, http.StatusTeapot,
		http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusUnavailableForLegalReasons,
		http.StatusUnprocessableEntity, http.StatusUnsupportedMediaType, http.StatusUpgradeRequired,
		http.StatusRequestURITooLong,
		// 5xx
		http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusInternalServerError,
		http.StatusServiceUnavailable, http.StatusHTTPVersionNotSupported,
	}

	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			err := ErrorByStatusCode(code)
			require.Error(t, err)
			assert.True(t, IsErrorStatusCode(err, code), "error should carry status code %d", code)

			var cE *customerror.CustomError
			require.True(t, errors.As(err, &cE))
			assert.Equal(t, code, cE.StatusCode)
		})
	}
}

func TestErrorByStatusCode_UnmappedDefaultsToInternalError(t *testing.T) {
	// 510 (Not Extended) is a valid code (<=511) that the switch does not map,
	// so it falls through to the default branch. Note: customerror validates
	// StatusCode as gte=100,lte=511.
	err := ErrorByStatusCode(510)
	require.Error(t, err)
	// The default branch still stamps the requested status code.
	assert.True(t, IsErrorStatusCode(err, 510))
}

func TestErrorByStatusCode_WithExtraOptions(t *testing.T) {
	err := ErrorByStatusCode(http.StatusBadRequest, customerror.WithField("k", "v"))
	require.Error(t, err)
	assert.True(t, IsErrorStatusCode(err, http.StatusBadRequest))
}

func TestIsErrorStatusCode_NegativeCases(t *testing.T) {
	err := ErrorByStatusCode(http.StatusNotFound)

	// Wrong code.
	assert.False(t, IsErrorStatusCode(err, http.StatusOK))
	// Not a customerror at all.
	assert.False(t, IsErrorStatusCode(errors.New("plain"), http.StatusNotFound))
	// Nil error.
	assert.False(t, IsErrorStatusCode(nil, http.StatusNotFound))
}
