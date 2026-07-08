package metrics

import (
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func expvarCount() int {
	n := 0
	expvar.Do(func(expvar.KeyValue) { n++ })
	return n
}

func TestNewInt_InitializedToZero(t *testing.T) {
	c := NewInt("init.subject.type")
	require.NotNil(t, c)
	assert.EqualValues(t, 0, c.Value())

	c.Add(5)
	assert.EqualValues(t, 5, c.Value())
}

// Reuse-by-name: the same name returns the SAME counter (process-global expvar
// semantics), which is what prevents the registry leak.
func TestNewInt_SameNameReturnsSameCounter(t *testing.T) {
	require.NotPanics(t, func() {
		a := NewInt("reuse.name.counter")
		b := NewInt("reuse.name.counter")
		require.NotNil(t, a)
		require.NotNil(t, b)

		assert.Same(t, a, b, "same name must return the same counter")

		a.Add(3)
		assert.EqualValues(t, 3, b.Value(), "shared counter reflects the increment")
	})
}

// The leak regression: creating the same-named counter many times must not grow
// the process-global expvar registry (previously it grew by 1 per call).
func TestNewInt_NoLeakOnReuse(t *testing.T) {
	before := expvarCount()
	for range 25 {
		_ = NewInt("noleak.name.counter")
	}
	after := expvarCount()
	assert.LessOrEqual(t, after-before, 1, "reuse must add at most one expvar entry, not one per call")
}

// If a var with the same name was already published to expvar outside this
// package, NewInt adopts it instead of re-publishing (which would panic).
func TestNewInt_AdoptsExistingExpvar(t *testing.T) {
	name := "adopt.me.counter"
	published := expvar.NewInt(name) // published directly, not via NewInt
	published.Set(7)

	got := NewInt(name)
	assert.Same(t, published, got, "must adopt the already-published var")
	assert.EqualValues(t, 7, got.Value())
}

func TestNewInt_PrefixApplied(t *testing.T) {
	t.Setenv("HTTPCLIENT_METRICS_PREFIX", "myprefix")

	c := NewInt("prefixed.subject.type")
	require.NotNil(t, c)

	found := false
	expvar.Do(func(kv expvar.KeyValue) {
		if kv.Key == "myprefix.prefixed.subject.type" {
			found = true
		}
	})
	assert.True(t, found, "metric name should be prefixed")
}
