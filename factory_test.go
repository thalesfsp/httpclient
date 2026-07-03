package httpclient

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		clientName string
		backoffDur time.Duration
		wantErr    bool
	}{
		{"empty name fails required", "", time.Second, true},
		{"uppercase name fails lowercase", "BadName", time.Second, true},
		{"sub-100ms backoff fails gte", uniqueName("bo"), 50 * time.Millisecond, true},
		{"valid config", uniqueName("ok"), time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(tt.clientName, nil, nil, nil, time.Second, tt.backoffDur, 3)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, c)
		})
	}
}

func TestNew_DefaultsAppliedWhenZero(t *testing.T) {
	c, err := New(uniqueName("def"), nil, nil, nil, time.Second, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, time.Second, c.RetrierBackoffDuration, "zero backoff duration defaults to 1s")
	assert.Equal(t, 3, c.RetrierBackoffTimes, "zero backoff times defaults to 3")
}

func TestMustNew_HappyAndPanic(t *testing.T) {
	require.NotPanics(t, func() {
		c := MustNew(uniqueName("must"), nil, nil, nil, time.Second, time.Second, 3)
		assert.NotNil(t, c)
	})

	require.Panics(t, func() {
		// Empty name fails validation → New returns error → MustNew panics.
		_ = MustNew("", nil, nil, nil, time.Second, time.Second, 3)
	})
}

func TestMustNewDefault_HappyAndPanic(t *testing.T) {
	require.NotPanics(t, func() {
		c := MustNewDefault(WithClientName(uniqueName("mustdef")))
		assert.NotNil(t, c)
	})

	require.Panics(t, func() {
		// Uppercase name fails the lowercase validation.
		_ = MustNewDefault(WithClientName("UPPER"))
	})
}

func TestNewDefault_UserAgentTracksName(t *testing.T) {
	name := uniqueName("ua")
	c, err := NewDefault(WithClientName(name))
	require.NoError(t, err)
	assert.Equal(t, name, c.Headers["User-Agent"], "User-Agent should match the client name")
	assert.Equal(t, "*/*", c.Headers["Accept"])
	assert.Equal(t, "application/json", c.Headers["Content-Type"])
}

func TestNewDefault_OptionErrorPropagates(t *testing.T) {
	boom := func(_ *ClientOptions) error { return assert.AnError }
	c, err := NewDefault(boom)
	require.Error(t, err)
	assert.Nil(t, c)
}

func TestGetters(t *testing.T) {
	name := uniqueName("getters")
	c := mustClient(t, name)

	assert.Equal(t, name, c.GetName())
	assert.NotNil(t, c.GetLogger())
	assert.NotNil(t, c.GetClient())
	assert.IsType(t, &http.Client{}, c.GetClient())

	// SetName is chainable and mutates the name.
	returned := c.SetName("renamed")
	assert.Same(t, c, returned)
	assert.Equal(t, "renamed", c.GetName())
}

func TestGet_PanicsWhenUninitialized(t *testing.T) {
	once = sync.Once{}
	singleton = nil

	require.Panics(t, func() { Get() })
}

func TestNewSingleton_ReturnsSameInstance(t *testing.T) {
	once = sync.Once{}
	singleton = nil

	c1, err := NewSingleton(uniqueName("single"), nil, nil, nil, time.Second, time.Second, 3)
	require.NoError(t, err)
	require.NotNil(t, c1)

	// Second call with a different name still returns the first instance
	// (once.Do only runs once).
	c2, err := NewSingleton(uniqueName("single2"), nil, nil, nil, time.Second, time.Second, 3)
	require.NoError(t, err)
	assert.Same(t, c1, c2)
	assert.Same(t, c1, Get())
}

func TestNewDefaultSingleton_ReturnsSameInstance(t *testing.T) {
	once = sync.Once{}
	singleton = nil

	c1, err := NewDefaultSingleton(WithClientName(uniqueName("dsingle")))
	require.NoError(t, err)
	require.NotNil(t, c1)

	c2, err := NewDefaultSingleton(WithClientName(uniqueName("dsingle2")))
	require.NoError(t, err)
	assert.Same(t, c1, c2)
}

func TestNewSingleton_ErrorPropagates(t *testing.T) {
	once = sync.Once{}
	singleton = nil

	// Uppercase name → validation error inside New → NewSingleton returns it.
	c, err := NewSingleton("UPPER", nil, nil, nil, time.Second, time.Second, 3)
	require.Error(t, err)
	assert.Nil(t, c)
}

func TestNewDefaultSingleton_ErrorPropagates(t *testing.T) {
	once = sync.Once{}
	singleton = nil

	boom := func(_ *ClientOptions) error { return assert.AnError }
	c, err := NewDefaultSingleton(boom)
	require.Error(t, err)
	assert.Nil(t, c)
}
