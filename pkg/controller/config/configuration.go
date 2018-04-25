/*
Copyright 2017 The Kubernetes Authors.

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

package config

import (
	"github.com/spf13/pflag"
)

type Configuration struct {
	Kubeconfig  string
	InCluster   bool
	WorkerCount int
}

var ControllerConfig = Configuration{
	InCluster:   true,
	WorkerCount: 5, // Default 5 worker.
}

func (c *Configuration) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Kubeconfig, "kubeconfig", c.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")
	fs.BoolVar(&c.InCluster, "incluster", c.InCluster, "Controller will be running inside the cluster.")
	fs.IntVar(&c.WorkerCount, "workers", c.WorkerCount, "The number of workers for controller.")
}
