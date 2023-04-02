package metrics

import (
	"expvar"
	"fmt"
	"os"

	"github.com/thalesfsp/httpclient/internal/logging"
	"github.com/thalesfsp/httpclient/internal/shared"
)

// NewInt creates and initializes a new expvar.Int. Name should be in the format of
// "{packageName}.{subject}.{type}", e.g. "{companyname}.api.rest.failed.counter".
//
// NOTE: All metrics are prefixed with the company name (e.g. companyname).
func NewInt(name string) *expvar.Int {
	prefix := os.Getenv("HTTPCLIENT_METRICS_PREFIX")

	if prefix == "" {
		logging.Get().Warnln("HTTPCLIENT_METRICS_PREFIX is not set. Using default (httpclient).")

		prefix = shared.PackageName
	}

	counter := expvar.NewInt(
		fmt.Sprintf(
			"%s.%s",
			prefix,
			name,
		),
	)

	counter.Set(0)

	return counter
}
