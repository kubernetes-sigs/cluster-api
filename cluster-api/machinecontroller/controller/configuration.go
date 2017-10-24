package controller

import (
	"github.com/spf13/pflag"
)

type Configuration struct {
}

func NewConfiguration() *Configuration {
	// Set defaults
	return &Configuration{}
}

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	// Populate values
}
