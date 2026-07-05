package metrics

import (
	"expvar"
	"os"
	"sync"
)

// registry reuses a single *expvar.Int per metric name. expvar publishes into a
// process-global map that is NEVER cleaned, so creating a fresh var per client
// leaks memory unboundedly (and panics on a duplicate name). Reusing by name
// fixes both: same-named clients share the same counter, and repeated
// construction registers nothing new.
var (
	mu       sync.Mutex
	registry = make(map[string]*expvar.Int)
)

// NewInt returns the expvar.Int for the given metric name, creating and
// publishing it on first use and reusing it thereafter. Name should be in the
// format "{packageName}.{subject}.{type}", e.g. "companyname.api.rest.failed.counter".
//
// NOTE: metrics are optionally prefixed via the HTTPCLIENT_METRICS_PREFIX env
// var. Reuse-by-name means two clients sharing a name also share (aggregate)
// their counters — the standard expvar semantics for a process-global metric.
func NewInt(name string) *expvar.Int {
	finalName := name

	if prefix := os.Getenv("HTTPCLIENT_METRICS_PREFIX"); prefix != "" {
		finalName = prefix + "." + finalName
	}

	mu.Lock()
	defer mu.Unlock()

	// Already handed out by this package.
	if c, ok := registry[finalName]; ok {
		return c
	}

	// Published elsewhere (or in a prior registry state) — adopt it rather than
	// re-publishing, which would panic with "Reuse of exported var name".
	if v := expvar.Get(finalName); v != nil {
		if iv, ok := v.(*expvar.Int); ok {
			registry[finalName] = iv

			return iv
		}
	}

	counter := expvar.NewInt(finalName)
	counter.Set(0)
	registry[finalName] = counter

	return counter
}
