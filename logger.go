package httpclient

import (
	"fmt"

	"github.com/thalesfsp/httpclient/internal/shared"
	"github.com/thalesfsp/sypl"
	"github.com/thalesfsp/sypl/level"
)

// Logger conforms Sypl to Req's logger requirements.
type Logger struct {
	*sypl.Sypl
}

// Implements custom io.Writer.
func (h *Logger) Write(p []byte) (int, error) {
	h.PrintlnWithOptions(level.Debug, string(p), sypl.WithTags(shared.PackageName))

	return len(p), nil
}

// Errorf prints according with the format @ the Error level.
func (h *Logger) Errorf(format string, v ...interface{}) {
	h.PrintlnWithOptions(level.Error, fmt.Sprintf(format, v...), sypl.WithTags(shared.PackageName))
}

// Warnf prints according with the specified format @ the Warn level.
func (h *Logger) Warnf(format string, v ...interface{}) {
	h.PrintlnWithOptions(level.Warn, fmt.Sprintf(format, v...), sypl.WithTags(shared.PackageName))
}

// Debugf prints according with the specified format @ the Debug level.
func (h *Logger) Debugf(format string, v ...interface{}) {
	h.PrintlnWithOptions(level.Debug, fmt.Sprintf(format, v...), sypl.WithTags(shared.PackageName))
}
