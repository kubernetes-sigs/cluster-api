package controller

import (
	"github.com/spf13/pflag"
)

type Configuration struct {
	Kubeconfig string
	Cloud string
}

func NewConfiguration() *Configuration {
	return &Configuration{}
}

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Kubeconfig, "kubeconfig", c.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")
	fs.StringVar(&c.Cloud, "cloud", c.Cloud, "Cloud provider (google/azure).")
}
