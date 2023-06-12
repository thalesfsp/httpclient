package metrics

import (
	"expvar"
	"os"
	"time"
)

// NewInt creates and initializes a new expvar.Int. Name should be in the format of
// "{packageName}.{subject}.{type}", e.g. "{companyname}.api.rest.failed.counter".
//
// NOTE- All metrics are prefixed with the company name (e.g. companyname).
func NewInt(name string) *expvar.Int {
	prefix := os.Getenv("HTTPCLIENT_METRICS_PREFIX")

	finalName := name + "--" + time.Now().Format(time.RFC3339)

	if prefix != "" {
		finalName = prefix + "." + finalName
	}

	counter := expvar.NewInt(finalName)

	counter.Set(0)

	return counter
}
