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

// Package proxy implements kubeadm proxy functionality.
package proxy

import (
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/portforward"
)

const scheme string = "proxy"

// Addr defines a proxy net/addr format.
type Addr struct {
	net.Addr
	port       string
	identifier uint32
}

// Network returns a fake network.
func (a Addr) Network() string {
	return portforward.PortForwardProtocolV1Name
}

// String returns encoded information about the connection.
func (a Addr) String() string {
	return fmt.Sprintf(
		"%s://%d.%s.local:%s",
		scheme,
		a.identifier,
		portforward.PortForwardProtocolV1Name,
		a.port,
	)
}

// NewAddrFromConn creates an Addr from the given connection.
func NewAddrFromConn(c *Conn) Addr {
	return Addr{
		port:       c.stream.Headers().Get(corev1.PortHeader),
		identifier: c.stream.Identifier(),
	}
}
