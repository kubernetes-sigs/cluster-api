package controller

import (
	"github.com/spf13/pflag"
)

type Configuration struct {
	Kubeconfig string
	Clusterconfig string
}

func NewConfiguration() *Configuration {
	return &Configuration{}
}

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Kubeconfig, "kubeconfig", c.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")
	fs.StringVar(&c.Clusterconfig, "clusterconfig", c.Clusterconfig, "Path to cluster yaml file.")
}
