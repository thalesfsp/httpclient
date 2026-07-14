package httpclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thalesfsp/sypl/v2"
	"github.com/thalesfsp/sypl/v2/level"
)

func newTestLogger() *Logger {
	return &Logger{sypl.NewDefault("httpclient-logger-test", level.None)}
}

func TestLogger_Write(t *testing.T) {
	l := newTestLogger()
	p := []byte("hello log")
	n, err := l.Write(p)
	require.NoError(t, err)
	assert.Equal(t, len(p), n, "Write must report all bytes consumed")
}

func TestLogger_FormattedLevels(t *testing.T) {
	l := newTestLogger()
	require.NotPanics(t, func() {
		l.Errorf("err %d", 1)
		l.Warnf("warn %s", "x")
		l.Debugf("debug %v", true)
	})
}
