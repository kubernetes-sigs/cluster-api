// Package log provides a log interface
package log

// Logger is a generic logging interface
type Logger interface {
	// Log inserts a log entry.  Arguments may be handled in the manner
	// of fmt.Print, but the underlying logger may also decide to handle
	// them differently.
	Log(v ...interface{})
	// Logf insets a log entry.  Arguments are handled in the manner of
	// fmt.Printf.
	Logf(format string, v ...interface{})
}

var (
	// The global default logger
	DefaultLogger Logger = &noOpLogger{}
)

// noOpLogger is used as a placeholder for the default logger
type noOpLogger struct{}

func (n *noOpLogger) Log(v ...interface{}) {}

func (n *noOpLogger) Logf(format string, v ...interface{}) {}

// Log logs using the default logger
func Log(v ...interface{}) {
	DefaultLogger.Log(v...)
}

// Logf logs formatted using the default logger
func Logf(format string, v ...interface{}) {
	DefaultLogger.Logf(format, v...)
}
