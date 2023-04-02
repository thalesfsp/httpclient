package httpclient

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/thalesfsp/customerror"
)

// HTTPStatusCodeClassifier classifies errors based on a HTTP status code, or
// a regex. It will automatically fail if error isn't of the type `CustomError`.
type HTTPStatusCodeClassifier struct {
	Regex       *regexp.Regexp
	StatusCodes []int
}

// Classify implements the Classifier interface.
func (hSCC HTTPStatusCodeClassifier) Classify(err error) retrier.Action {
	// Should do nothing if there's no error.
	if err == nil {
		return retrier.Succeed
	}

	var cE *customerror.CustomError

	if errors.As(err, &cE) {
		// Should retry if regex match the error's status code.
		if hSCC.Regex != nil {
			if hSCC.Regex.MatchString(strconv.Itoa(cE.StatusCode)) {
				return retrier.Retry
			}
		}

		// Should retry if status code is in the list.
		for _, statusCode := range hSCC.StatusCodes {
			if cE.StatusCode == statusCode {
				return retrier.Retry
			}
		}
	}

	// Should fail if it isn't of the type `CustomError`, and everything else.
	return retrier.Fail
}
