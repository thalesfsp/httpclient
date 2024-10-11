package httpclient

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/thalesfsp/customerror"
)

const requestPurpose = "request"

// purpose, nolint:gosec
const (
	ErrRequestBadGateway           = "ERR_HTTP_BAD_GATEWAY"                   // FailedTo.
	ErrRequestBadRequest           = "ERR_HTTP_BAD_REQUEST"                   // FailedTo.
	ErrRequestConflict             = "ERR_HTTP_CONFLICT"                      // FailedTo.
	ErrRequestEntityTooLarge       = "ERR_HTTP_ENTITY_TOO_LARGE"              // FailedTo.
	ErrRequestExpectationFailed    = "ERR_HTTP_EXPECTATION_FAILED"            // FailedTo.
	ErrRequestFailedDependency     = "ERR_HTTP_FAILED_DEPENDENCY"             // FailedTo.
	ErrRequestForbidden            = "ERR_HTTP_FORBIDDEN"                     // FailedTo.
	ErrRequestGatewayTimeout       = "ERR_HTTP_GATEWAY_TIMEOUT"               // FailedTo.
	ErrRequestGone                 = "ERR_HTTP_GONE"                          // FailedTo.
	ErrRequestInternalServerError  = "ERR_HTTP_INTERNAL_SERVER_ERROR"         // FailedTo.
	ErrRequestLengthRequired       = "ERR_HTTP_LENGTH_REQUIRED"               // FailedTo.
	ErrRequestLocked               = "ERR_HTTP_LOCKED"                        // FailedTo.
	ErrRequestMethodNotAllowed     = "ERR_HTTP_METHOD_NOT_ALLOWED"            // FailedTo.
	ErrRequestMisdirectedRequest   = "ERR_HTTP_MISDIRECTED_REQUEST"           // FailedTo.
	ErrRequestNotAcceptable        = "ERR_HTTP_NOT_ACCEPTABLE"                // FailedTo.
	ErrRequestNotAuthorized        = "ERR_HTTP_UNAUTHORIZED"                  // FailedTo.
	ErrRequestNotFound             = "ERR_HTTP_NOT_FOUND"                     // FailedTo.
	ErrRequestNotImplemented       = "ERR_HTTP_NOT_IMPLEMENTED"               // FailedTo.
	ErrRequestPaymentRequired      = "ERR_HTTP_PAYMENT_REQUIRED"              // FailedTo.
	ErrRequestPreconditionFailed   = "ERR_HTTP_PRECONDITION_FAILED"           // FailedTo.
	ErrRequestPreconditionRequired = "ERR_HTTP_PRECONDITION_REQUIRED"         // FailedTo.
	ErrRequestProxyAuthRequired    = "ERR_HTTP_PROXY_AUTH_REQUIRED"           // FailedTo.
	ErrRequestRangeNotSatisfiable  = "ERR_HTTP_RANGE_NOT_SATISFIABLE"         // FailedTo.
	ErrRequestServiceUnavailable   = "ERR_HTTP_SERVICE_UNAVAILABLE"           // FailedTo.
	ErrRequestTeapot               = "ERR_HTTP_TEAPOT"                        // FailedTo.
	ErrRequestTimeout              = "ERR_HTTP_REQUEST_TIMEOUT"               // FailedTo.
	ErrRequestTooManyRequests      = "ERR_HTTP_TOO_MANY_REQUESTS"             // FailedTo.
	ErrRequestUnavailableForLegal  = "ERR_HTTP_UNAVAILABLE_FOR_LEGAL_REASONS" // FailedTo.
	ErrRequestUnprocessableEntity  = "ERR_HTTP_UNPROCESSABLE_ENTITY"          // FailedTo.
	ErrRequestUnsupportedMediaType = "ERR_HTTP_UNSUPPORTED_MEDIA_TYPE"        // FailedTo.
	ErrRequestUpgradeRequired      = "ERR_HTTP_UPGRADE_REQUIRED"              // FailedTo.
	ErrRequestURITooLong           = "ERR_HTTP_URI_TOO_LONG"                  // FailedTo.
	ErrRequestVersionNotSupported  = "ERR_HTTP_VERSION_NOT_SUPPORTED"         // FailedTo.
)

// ErrorCatalog is the catalog of errors.
var ErrorCatalog = customerror.
	MustNewCatalog(Type).
	MustSet(ErrRequestBadGateway, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusBadGateway))).
	MustSet(ErrRequestBadRequest, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusBadRequest))).
	MustSet(ErrRequestConflict, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusConflict))).
	MustSet(ErrRequestEntityTooLarge, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusRequestEntityTooLarge))).
	MustSet(ErrRequestExpectationFailed, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusExpectationFailed))).
	MustSet(ErrRequestFailedDependency, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusFailedDependency))).
	MustSet(ErrRequestForbidden, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusForbidden))).
	MustSet(ErrRequestGatewayTimeout, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusGatewayTimeout))).
	MustSet(ErrRequestGone, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusGone))).
	MustSet(ErrRequestInternalServerError, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusInternalServerError))).
	MustSet(ErrRequestLengthRequired, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusLengthRequired))).
	MustSet(ErrRequestLocked, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusLocked))).
	MustSet(ErrRequestMethodNotAllowed, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusMethodNotAllowed))).
	MustSet(ErrRequestMisdirectedRequest, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusMisdirectedRequest))).
	MustSet(ErrRequestNotAcceptable, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusNotAcceptable))).
	MustSet(ErrRequestNotAuthorized, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusUnauthorized))).
	MustSet(ErrRequestNotFound, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusNotFound))).
	MustSet(ErrRequestNotImplemented, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusNotImplemented))).
	MustSet(ErrRequestPaymentRequired, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusPaymentRequired))).
	MustSet(ErrRequestPreconditionFailed, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusPreconditionFailed))).
	MustSet(ErrRequestPreconditionRequired, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusPreconditionRequired))).
	MustSet(ErrRequestProxyAuthRequired, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusProxyAuthRequired))).
	MustSet(ErrRequestRangeNotSatisfiable, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusRequestedRangeNotSatisfiable))).
	MustSet(ErrRequestServiceUnavailable, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusServiceUnavailable))).
	MustSet(ErrRequestTeapot, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusTeapot))).
	MustSet(ErrRequestTimeout, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusRequestTimeout))).
	MustSet(ErrRequestTooManyRequests, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusTooManyRequests))).
	MustSet(ErrRequestUnavailableForLegal, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusUnavailableForLegalReasons))).
	MustSet(ErrRequestUnprocessableEntity, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusUnprocessableEntity))).
	MustSet(ErrRequestUnsupportedMediaType, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusUnsupportedMediaType))).
	MustSet(ErrRequestUpgradeRequired, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusUpgradeRequired))).
	MustSet(ErrRequestURITooLong, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusRequestURITooLong))).
	MustSet(ErrRequestVersionNotSupported, fmt.Sprintf("%s, %s", requestPurpose, http.StatusText(http.StatusHTTPVersionNotSupported)))

// ErrorByStatusCode returns an error based on the status code.
func ErrorByStatusCode(statusCode int, opts ...customerror.Option) error {
	// Prepare options for creating a custom error.
	customerrorOptions := []customerror.Option{
		customerror.WithStatusCode(statusCode),
	}

	customerrorOptions = append(customerrorOptions, opts...)

	switch statusCode {
	// 4xx errors.
	case http.StatusBadRequest:
		return ErrorCatalog.MustGet(ErrRequestBadRequest).NewInvalidError(customerrorOptions...)
	case http.StatusConflict:
		return ErrorCatalog.MustGet(ErrRequestConflict).NewInvalidError(customerrorOptions...)
	case http.StatusRequestEntityTooLarge:
		return ErrorCatalog.MustGet(ErrRequestEntityTooLarge).NewInvalidError(customerrorOptions...)
	case http.StatusExpectationFailed:
		return ErrorCatalog.MustGet(ErrRequestExpectationFailed).NewInvalidError(customerrorOptions...)
	case http.StatusFailedDependency:
		return ErrorCatalog.MustGet(ErrRequestFailedDependency).NewInvalidError(customerrorOptions...)
	case http.StatusForbidden:
		return ErrorCatalog.MustGet(ErrRequestForbidden).NewInvalidError(customerrorOptions...)
	case http.StatusGone:
		return ErrorCatalog.MustGet(ErrRequestGone).NewInvalidError(customerrorOptions...)
	case http.StatusLengthRequired:
		return ErrorCatalog.MustGet(ErrRequestLengthRequired).NewInvalidError(customerrorOptions...)
	case http.StatusLocked:
		return ErrorCatalog.MustGet(ErrRequestLocked).NewInvalidError(customerrorOptions...)
	case http.StatusMethodNotAllowed:
		return ErrorCatalog.MustGet(ErrRequestMethodNotAllowed).NewInvalidError(customerrorOptions...)
	case http.StatusMisdirectedRequest:
		return ErrorCatalog.MustGet(ErrRequestMisdirectedRequest).NewInvalidError(customerrorOptions...)
	case http.StatusNotAcceptable:
		return ErrorCatalog.MustGet(ErrRequestNotAcceptable).NewInvalidError(customerrorOptions...)
	case http.StatusUnauthorized:
		return ErrorCatalog.MustGet(ErrRequestNotAuthorized).NewInvalidError(customerrorOptions...)
	case http.StatusNotFound:
		return ErrorCatalog.MustGet(ErrRequestNotFound).NewInvalidError(customerrorOptions...)
	case http.StatusNotImplemented:
		return ErrorCatalog.MustGet(ErrRequestNotImplemented).NewInvalidError(customerrorOptions...)
	case http.StatusPaymentRequired:
		return ErrorCatalog.MustGet(ErrRequestPaymentRequired).NewInvalidError(customerrorOptions...)
	case http.StatusPreconditionFailed:
		return ErrorCatalog.MustGet(ErrRequestPreconditionFailed).NewInvalidError(customerrorOptions...)
	case http.StatusPreconditionRequired:
		return ErrorCatalog.MustGet(ErrRequestPreconditionRequired).NewInvalidError(customerrorOptions...)
	case http.StatusProxyAuthRequired:
		return ErrorCatalog.MustGet(ErrRequestProxyAuthRequired).NewInvalidError(customerrorOptions...)
	case http.StatusRequestedRangeNotSatisfiable:
		return ErrorCatalog.MustGet(ErrRequestRangeNotSatisfiable).NewInvalidError(customerrorOptions...)
	case http.StatusTeapot:
		return ErrorCatalog.MustGet(ErrRequestTeapot).NewInvalidError(customerrorOptions...)
	case http.StatusRequestTimeout:
		return ErrorCatalog.MustGet(ErrRequestTimeout).NewInvalidError(customerrorOptions...)
	case http.StatusTooManyRequests:
		return ErrorCatalog.MustGet(ErrRequestTooManyRequests).NewInvalidError(customerrorOptions...)
	case http.StatusUnavailableForLegalReasons:
		return ErrorCatalog.MustGet(ErrRequestUnavailableForLegal).NewInvalidError(customerrorOptions...)
	case http.StatusUnprocessableEntity:
		return ErrorCatalog.MustGet(ErrRequestUnprocessableEntity).NewInvalidError(customerrorOptions...)
	case http.StatusUnsupportedMediaType:
		return ErrorCatalog.MustGet(ErrRequestUnsupportedMediaType).NewInvalidError(customerrorOptions...)
	case http.StatusUpgradeRequired:
		return ErrorCatalog.MustGet(ErrRequestUpgradeRequired).NewInvalidError(customerrorOptions...)
	case http.StatusRequestURITooLong:
		return ErrorCatalog.MustGet(ErrRequestURITooLong).NewInvalidError(customerrorOptions...)

	// 5xx errors.
	case http.StatusBadGateway:
		return ErrorCatalog.MustGet(ErrRequestBadGateway).NewFailedToError(customerrorOptions...)
	case http.StatusGatewayTimeout:
		return ErrorCatalog.MustGet(ErrRequestGatewayTimeout).NewFailedToError(customerrorOptions...)
	case http.StatusInternalServerError:
		return ErrorCatalog.MustGet(ErrRequestInternalServerError).NewFailedToError(customerrorOptions...)
	case http.StatusServiceUnavailable:
		return ErrorCatalog.MustGet(ErrRequestServiceUnavailable).NewFailedToError(customerrorOptions...)
	case http.StatusHTTPVersionNotSupported:
		return ErrorCatalog.MustGet(ErrRequestVersionNotSupported).NewFailedToError(customerrorOptions...)

	// If no error is found, return a generic error.
	default:
		return ErrorCatalog.MustGet(ErrRequestInternalServerError).NewFailedToError(customerrorOptions...)
	}
}

// IsErrorStatusCode checks if the error is of a specific status code.
func IsErrorStatusCode(err error, statusCode int) bool {
	// Verify the status code
	var finalError *customerror.CustomError

	if errors.As(err, &finalError) {
		if finalError.StatusCode == statusCode {
			return true
		}
	}

	return false
}
