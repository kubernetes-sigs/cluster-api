/*
Copyright 2023 The Kubernetes Authors.

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

package remote

import (
	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	restConfigTimeout         = defaultClientTimeout
	restConfigQPS     float32 = 20
	restConfigBurst           = 30
)

// AddRestConfigFlags adds the kube-api rest configuration flags to the flag set.
func AddRestConfigFlags(fs *pflag.FlagSet) {
	fs.Float32Var(&restConfigQPS, "kube-api-qps", 20,
		"Maximum queries per second from the controller client to the Kubernetes API server.")

	fs.IntVar(&restConfigBurst, "kube-api-burst", 30,
		"Maximum number of queries that should be allowed in one burst from the controller client to the Kubernetes API server.")

	fs.DurationVar(&restConfigTimeout, "kube-api-timeout", defaultClientTimeout,
		"The maximum length of time to wait before giving up on a server request. A value of zero means no timeout.")
}

// DefaultRestConfig returns the runtime rest configuration with values set from the flags.
func DefaultRestConfig() *rest.Config {
	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst
	return restConfig
}
