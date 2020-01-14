/*
Copyright 2020 The Kubernetes Authors.

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

package proxy

import (
	"crypto/tls"
	"time"

	"k8s.io/client-go/rest"
)

// Proxy defines the API server port-forwarded proxy
type Proxy struct {

	// Kind is the kind of Kubernetes resource
	Kind string

	// Namespace is the namespace in which the Kubernetes resource exists
	Namespace string

	// ResourceName is the name of the Kubernetes resource
	ResourceName string

	// KubeConfig is the config to connect to the API server
	KubeConfig *rest.Config

	// TLSConfig is for connecting to TLS servers listening on a proxied port
	TLSConfig *tls.Config

	// KeepAlive specifies how often a keep alive message is sent to hold
	// the connection open
	KeepAlive *time.Duration

	// Port is the port to be forwarded from the relevant resource
	Port int
}
