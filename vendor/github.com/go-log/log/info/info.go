// Package info allows users to create a Logger interface from any
// object that supports Info and Infof.
package info

// Info is an interface for Info and Infof.
type Info interface {
	Info(v ...interface{})
	Infof(format string, v ...interface{})
}

type logger struct{
	info Info
}

func (logger *logger) Log(v ...interface{}) {
	logger.info.Info(v...)
}

func (logger *logger) Logf(format string, v ...interface{}) {
	logger.info.Infof(format, v...)
}

// New creates a new logger wrapping info.
func New(info Info) *logger {
	return &logger{
		info: info,
	}
}
