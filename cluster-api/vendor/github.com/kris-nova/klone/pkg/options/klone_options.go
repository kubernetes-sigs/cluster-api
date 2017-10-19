package options

type RuntimeContext struct {
	TestAuthMode bool
}

var R = &RuntimeContext{
	TestAuthMode: false,
}
