package httpclient

import (
	"errors"
	"regexp"
	"testing"

	"github.com/eapache/go-resiliency/retrier"
	"github.com/stretchr/testify/assert"
	"github.com/thalesfsp/customerror"
)

// TestClassify_WithClientCounters verifies the classifier updates the client's
// retried/failed metrics and marks the error as retried.
func TestClassify_WithClientCounters(t *testing.T) {
	t.Run("nil error succeeds", func(t *testing.T) {
		c := mustClient(t, uniqueName("clsnil"))
		cc := CustomClassifier{Client: c}
		assert.Equal(t, retrier.Succeed, cc.Classify(nil))
	})

	t.Run("custom regex retries and counts", func(t *testing.T) {
		c := mustClient(t, uniqueName("clsregex"))
		cc := CustomClassifier{
			Client:        c,
			CustomRegexes: []*regexp.Regexp{regexp.MustCompile(`(?i)locked`)},
		}
		cE := &customerror.CustomError{Message: "resource is Locked"}
		assert.Equal(t, retrier.Retry, cc.Classify(cE))
		assert.EqualValues(t, 1, c.counterRetried.Value())
		assert.True(t, cE.Retried)
	})

	t.Run("status code list retries and counts", func(t *testing.T) {
		c := mustClient(t, uniqueName("clscode"))
		cc := CustomClassifier{Client: c, StatusCodes: []int{499}}
		cE := &customerror.CustomError{StatusCode: 499}
		assert.Equal(t, retrier.Retry, cc.Classify(cE))
		assert.EqualValues(t, 1, c.counterRetried.Value())
	})

	t.Run("http retrier regex retries and counts", func(t *testing.T) {
		c := mustClient(t, uniqueName("clshttp"))
		cc := CustomClassifier{Client: c, HTTPRetrierRegex: httpRetrierRegex}
		cE := &customerror.CustomError{StatusCode: 503}
		assert.Equal(t, retrier.Retry, cc.Classify(cE))
		assert.EqualValues(t, 1, c.counterRetried.Value())
	})

	t.Run("retryable flag retries and counts", func(t *testing.T) {
		c := mustClient(t, uniqueName("clsretryable"))
		cc := CustomClassifier{Client: c, HTTPRetrierRegex: httpRetrierRegex}
		cE := &customerror.CustomError{StatusCode: 400, Retryable: true}
		assert.Equal(t, retrier.Retry, cc.Classify(cE))
		assert.EqualValues(t, 1, c.counterRetried.Value())
	})

	t.Run("no match fails without counting (request() owns counterFailed)", func(t *testing.T) {
		c := mustClient(t, uniqueName("clsfail"))
		cc := CustomClassifier{Client: c, HTTPRetrierRegex: httpRetrierRegex}
		cE := &customerror.CustomError{StatusCode: 404}
		assert.Equal(t, retrier.Fail, cc.Classify(cE))
		assert.EqualValues(t, 0, c.counterFailed.Value(), "classifier must not count failures")
	})

	t.Run("non-custom error fails without counting", func(t *testing.T) {
		c := mustClient(t, uniqueName("clsplain"))
		cc := CustomClassifier{Client: c, HTTPRetrierRegex: httpRetrierRegex}
		assert.Equal(t, retrier.Fail, cc.Classify(errors.New("plain")))
		assert.EqualValues(t, 0, c.counterFailed.Value(), "classifier must not count failures")
	})
}
