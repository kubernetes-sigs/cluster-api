package controller

import (
	"github.com/spf13/pflag"
)

type Configuration struct {
	Kubeconfig string
	Cloud string
	KubeadmToken string
}

func NewConfiguration() *Configuration {
	return &Configuration{}
}

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Kubeconfig, "kubeconfig", c.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")
	fs.StringVar(&c.Cloud, "cloud", c.Cloud, "Cloud provider (google/azure).")
	fs.StringVar(&c.KubeadmToken, "token", c.KubeadmToken, "Kubeadm token to use to join new machines.")
}
