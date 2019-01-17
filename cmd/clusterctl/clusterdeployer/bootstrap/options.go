/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bootstrap

import "github.com/spf13/pflag"

type Options struct {
	Type       string
	Cleanup    bool
	ExtraFlags []string
	KubeConfig string
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Type, "bootstrap-type", "", "none", "The cluster bootstrapper to use.")
	fs.BoolVarP(&o.Cleanup, "bootstrap-cluster-cleanup", "", true, "Whether to cleanup the bootstrap cluster after bootstrap.")
	fs.StringVarP(&o.KubeConfig, "bootstrap-cluster-kubeconfig", "e", "", "Sets the bootstrap cluster to be an existing Kubernetes cluster.")
	fs.StringSliceVarP(&o.ExtraFlags, "bootstrap-flags", "", []string{}, "Command line flags to be passed to the chosen bootstrapper")
}
