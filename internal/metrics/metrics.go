package metrics

import (
	"expvar"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// seq is a process-wide monotonic counter guaranteeing every metric name is
// unique, so expvar.NewInt never panics with "Reuse of exported var name" when
// two clients share the same name (a timestamp alone collides within a second).
var seq atomic.Uint64

// NewInt creates and initializes a new expvar.Int. Name should be in the format of
// "{packageName}.{subject}.{type}", e.g. "{companyname}.api.rest.failed.counter".
//
// NOTE- All metrics are prefixed with the company name (e.g. companyname).
func NewInt(name string) *expvar.Int {
	prefix := os.Getenv("HTTPCLIENT_METRICS_PREFIX")

	// The atomic sequence guarantees in-process uniqueness (race-free); the
	// timestamp is retained purely for human readability.
	finalName := name + "-" + time.Now().Format(time.RFC3339) + "-" + strconv.FormatUint(seq.Add(1), 10)

	if prefix != "" {
		finalName = prefix + "." + finalName
	}

	counter := expvar.NewInt(finalName)

	counter.Set(0)

	return counter
}
