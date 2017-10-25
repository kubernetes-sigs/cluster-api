package signals

const (
	GlobalTimeoutSecondsDefault = 600
	GlobalExitOnTimeoutDefault  = false
)

var sigHandler *Handler

func GlobalSignalHandler() *Handler {
	if sigHandler == nil {
		sigHandler = NewSignalHandler(GlobalTimeoutSecondsDefault, GlobalExitOnTimeoutDefault)
		sigHandler.Register()
	}
	return sigHandler
}
