package httpclient

import (
	"regexp"
	"strconv"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/thalesfsp/customerror"
)

// CustomClassifier classifies errors based on - in order:
// - Any regex set by the user.
// - HTTP status code list.
// - Standard retryable HTTP status codes.
// - Retryable custom errors.
// If no error is passed, it will succeed.
// If nothing matches, it will fail.
//
// NOTE: It's safe to assume that `err` will be of customerror.CustomError and
// will contain a status code.
type CustomClassifier struct {
	CustomRegexes    []*regexp.Regexp
	HTTPRetrierRegex *regexp.Regexp
	StatusCodes      []int
	Client           *Client
}

// Classify implements the Classifier interface.
//
//nolint:nestif,gocognit
func (hSCC CustomClassifier) Classify(err error) retrier.Action {
	// Should do nothing if there's no error.
	if err == nil {
		return retrier.Succeed
	}

	// Any user defined regex.
	//
	// Should retry if regex match the error's message.
	if len(hSCC.CustomRegexes) > 0 {
		for _, r := range hSCC.CustomRegexes {
			errMsg := err.Error()

			if r.MatchString(errMsg) {
				// Should only increment the counter if the client is set.
				if hSCC.Client != nil {
					hSCC.Client.counterRetried.Add(1)
				}

				// Update the error retried status.
				if cE, ok := customerror.To(err); ok {
					cE.SetRetried(true)
				}

				return retrier.Retry
			}
		}
	}

	if cE, ok := customerror.To(err); ok {
		// User defined HTTP status codes prioritized over standard ones.
		//
		// Should retry if status code is in the list.
		for _, statusCode := range hSCC.StatusCodes {
			// Should retry if status code is in the list.
			if cE.StatusCode == statusCode {
				// Should only increment the counter if the client is set.
				if hSCC.Client != nil {
					hSCC.Client.counterRetried.Add(1)
				}

				// Update the error retried status.
				cE.SetRetried(true)

				return retrier.Retry
			}
		}

		// Standard retryable HTTP status codes.
		//
		// Should retry if regex match the error's status code.
		if hSCC.HTTPRetrierRegex != nil {
			if hSCC.HTTPRetrierRegex.MatchString(strconv.Itoa(cE.StatusCode)) {
				// Should only increment the counter if the client is set.
				if hSCC.Client != nil {
					hSCC.Client.counterRetried.Add(1)
				}

				// Update the error retried status.
				cE.SetRetried(true)

				return retrier.Retry
			}
		}

		// Retryable errors.
		if cE.Retryable {
			// Should only increment the counter if the client is set.
			if hSCC.Client != nil {
				hSCC.Client.counterRetried.Add(1)
			}

			// Update the error retried status.
			cE.SetRetried(true)

			return retrier.Retry
		}
	}

	// Should only increment the counter if the client is set.
	if hSCC.Client != nil {
		hSCC.Client.counterFailed.Add(1)
	}

	// Should fail for everything else.
	return retrier.Fail
}
