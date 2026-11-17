# Critical defect plan — httpclient (base 90944eaba719a16dd802090615792e51d5801292)

This records the original audit at the base SHA above. The follow-up fix is now
authorized and implemented; verification and release status are recorded below.

## C1 — `NewDefault()` without a name leaks 3 process-global expvar counters per call (unbounded, never reclaimed)

### Severity rationale (CRITICAL, availability)
- Deterministic, unbounded, process-lifetime heap growth on the library's default constructor path.
  Nothing reclaims it: `expvar` has no unpublish, and `internal/metrics.registry` also pins every entry.
- Measured (Go 1.25.14, no `-race`, after `runtime.GC()`): 20,000 × `NewDefault()` → +60,000 expvar
  vars, +18,469,336 heap bytes (~923 B/call). Control: 20,000 × `NewDefault(WithClientName("stable-control"))`
  → +3 vars, -17,288 bytes.
- Construction also slows as the leak grows: `expvar.Publish` inserts into a sorted key slice (O(n)),
  under the global `metrics.mu`. The 20,000-call loop took 42.8 s.
- Example impact: a service building a client per request at 50 req/s retains about 4 GB a day,
  which kills a 512 MiB container within hours. `/debug/vars` output also grows without bound.
- Precondition: the caller constructs clients repeatedly without `WithClientName`. Create-once and
  singleton use is NOT affected. The package's own comment (`internal/metrics/metrics.go:9-13`)
  says per-client vars "leak memory unboundedly" and claims reuse-by-name fixed it. That fix does
  not cover the default path, because the default name is a fresh UUID on every call.

### Source
- `httpclient.go:683`: `name := shared.PackageName + "-" + shared.GenerateUUID()` (unique per call).
- `httpclient.go:625-627`: three `metrics.NewInt(...)` per `New`, each keyed by `name`.
- `internal/metrics/metrics.go:16`: `registry = make(map[string]*expvar.Int)` (never pruned).
- `internal/metrics/metrics.go:51-53`: `expvar.NewInt(finalName)` (permanent global publish) plus a registry insert.

### Triggering call
```go
for {
    c, _ := httpclient.NewDefault() // no WithClientName
    resp, _ := c.Get(ctx, url)
    if resp != nil { resp.Body.Close() }
}
```

### Evidence
- Probe: `/Volumes/Dev/Dev/critical-review-20261116/httpclient/probes/consumer/consumer_test.go`
  (`TestProbe_NewDefaultExpvarGrowth`, run with `RUN_LEAK_PROBE=1`).
- Logs: `logs/baseline/leak-probe-norace.log` and `logs/baseline/consumer-probe.log` (the `-race` run, ~938 B/call).

### Root cause
Metric identity is bound to the client instance name. The default name is random, so every unnamed
client mints three new global metric identities. The existing regression tests
(`TestBug_SameName_NoMetricPanic`, `TestNewInt_NoLeakOnReuse`) only use STABLE names. They pass for
the wrong reason with respect to the default path.

### Bounded fix plan (smallest change; keeps the public API)
1. Separate the metric key from the per-instance name. When `NewDefault` uses its generated default
   name, key metrics by a stable default label (for example `httpclient.default.<status>.counter`).
   A caller-supplied `WithClientName` keeps its name-keyed metrics. `User-Agent`/`Name` may stay unique.
2. Implement this as an unexported metric-name field or parameter used by `New`. Do not change the
   exported signatures of `New`, `NewDefault`, `MustNew*` or `NewSingleton*`.
3. Document in the `NewDefault` godoc that unnamed clients share aggregate default counters.
4. Out of scope: caller-chosen dynamic names in `New(name, ...)`. That is the documented
   reuse-by-name semantics and the caller's own cardinality choice.

### Regression and negative-control plan
- New test: 1,000 × `NewDefault()`. Assert the `expvar.Do` count grows by ≤ 3 in total, and that
  heap growth after GC stays under a loose bound (for example < 200 KB).
- Negative control: revert step 1 and watch the test fail with a +3,000 var delta. Restore and watch it pass.
- Bypass test: 1,000 × `NewDefault(WithClientHeader("X","y"))` (an option without a name) must also
  stay ≤ 3. `NewDefaultSingleton()` racing first-inits must not add more than 3.
- Keep `TestBug_SameName_NoMetricPanic` and the counter-semantics tests green. Add one test that two
  unnamed default clients share counters, which is the intended documented aggregate.

### Follow-up implementation
- A private factory accepts a metric prefix independently of the visible name. Unnamed
  default clients use `httpclient.<status>.counter`; named clients retain the original
  `httpclient.<name>.<status>.counter`. There is no reserved client name to collide with.
- `Name` and `User-Agent` remain unique for unnamed clients. Public signatures are unchanged.
- `default_metrics_test.go` covers 1,000 plain and 1,000 option-bearing constructions,
  legacy named-client metrics, a real local HTTP request, and 64 forced concurrent
  singleton first-initializers (including discarded losing clients).
- The unchanged-constructor negative control fails with 3,000 keys instead of 3 for
  each 1,000-client case, and 192 instead of 3 for the singleton race. The fix makes
  these same tests pass under `-race`. Raw logs are in the session scratch directory,
  `logs/fix/regression-original.log` and `logs/fix/regression-fixed.log`.
- Deterministic retained-key assertions replace the proposed noisy heap threshold.
  The original post-GC heap probe remains audit evidence.
- Required CI now executes `make ci`, including these unit regressions, with the same
  pinned linter and race/coverage flags as the local target. No lint rules are disabled.
- Release is pending the parent's independent review of the committed local result.
