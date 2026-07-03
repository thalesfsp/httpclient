package metrics

import (
	"expvar"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInt_InitializedToZero(t *testing.T) {
	c := NewInt("pkg.subject.type")
	require.NotNil(t, c)
	assert.EqualValues(t, 0, c.Value())

	c.Add(5)
	assert.EqualValues(t, 5, c.Value())
}

func TestNewInt_SameNameNoPanicDistinctVars(t *testing.T) {
	require.NotPanics(t, func() {
		a := NewInt("dup.name.counter")
		b := NewInt("dup.name.counter")
		require.NotNil(t, a)
		require.NotNil(t, b)

		// Distinct counters — incrementing one must not affect the other.
		a.Add(1)
		assert.EqualValues(t, 1, a.Value())
		assert.EqualValues(t, 0, b.Value())
	})
}

func TestNewInt_PrefixApplied(t *testing.T) {
	t.Setenv("HTTPCLIENT_METRICS_PREFIX", "myprefix")

	c := NewInt("prefixed.subject.type")
	require.NotNil(t, c)

	found := false
	expvar.Do(func(kv expvar.KeyValue) {
		if strings.HasPrefix(kv.Key, "myprefix.prefixed.subject.type") {
			found = true
		}
	})
	assert.True(t, found, "metric name should be prefixed")
}
