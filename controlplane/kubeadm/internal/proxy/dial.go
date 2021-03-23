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
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const defaultTimeout = 10 * time.Second

// Dialer creates connections using Kubernetes API Server port-forwarding.
type Dialer struct {
	proxy          Proxy
	clientset      *kubernetes.Clientset
	proxyTransport http.RoundTripper
	upgrader       spdy.Upgrader
	timeout        time.Duration
}

// NewDialer creates a new dialer for a given API server scope.
func NewDialer(p Proxy, options ...func(*Dialer) error) (*Dialer, error) {
	if p.Port == 0 {
		return nil, errors.New("port required")
	}

	dialer := &Dialer{
		proxy: p,
	}

	for _, option := range options {
		err := option(dialer)
		if err != nil {
			return nil, err
		}
	}

	if dialer.timeout == 0 {
		dialer.timeout = defaultTimeout
	}
	p.KubeConfig.Timeout = dialer.timeout
	clientset, err := kubernetes.NewForConfig(p.KubeConfig)
	if err != nil {
		return nil, err
	}
	proxyTransport, upgrader, err := spdy.RoundTripperFor(p.KubeConfig)
	if err != nil {
		return nil, err
	}
	dialer.proxyTransport = proxyTransport
	dialer.upgrader = upgrader
	dialer.clientset = clientset
	return dialer, nil
}

// DialContextWithAddr is a GO grpc compliant dialer construct.
func (d *Dialer) DialContextWithAddr(ctx context.Context, addr string) (net.Conn, error) {
	return d.DialContext(ctx, scheme, addr)
}

// DialContext creates proxied port-forwarded connections.
// ctx is currently unused, but fulfils the type signature used by GRPC.
func (d *Dialer) DialContext(_ context.Context, network string, addr string) (net.Conn, error) {
	req := d.clientset.CoreV1().RESTClient().
		Post().
		Resource(d.proxy.Kind).
		Namespace(d.proxy.Namespace).
		Name(addr).
		SubResource("portforward")

	dialer := spdy.NewDialer(d.upgrader, &http.Client{Transport: d.proxyTransport}, "POST", req.URL())

	// Create a new connection from the dialer.
	//
	// Warning: Any early return should close this connection, otherwise we're going to leak them.
	connection, _, err := dialer.Dial(portforward.PortForwardProtocolV1Name)
	if err != nil {
		return nil, errors.Wrap(err, "error upgrading connection")
	}

	// Create the headers.
	headers := http.Header{}

	// Set the header port number to match the proxy one.
	headers.Set(corev1.PortHeader, fmt.Sprintf("%d", d.proxy.Port))

	// We only create a single stream over the connection
	headers.Set(corev1.PortForwardRequestIDHeader, "0")

	// Create the error stream.
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	errorStream, err := connection.CreateStream(headers)
	if err != nil {
		return nil, kerrors.NewAggregate([]error{
			err,
			connection.Close(),
		})
	}
	// Close the error stream right away, we're not writing to it.
	if err := errorStream.Close(); err != nil {
		return nil, kerrors.NewAggregate([]error{
			err,
			connection.Close(),
		})
	}

	// Create the data stream.
	//
	// NOTE: Given that we're reusing the headers,
	// we need to overwrite the stream type before creating it.
	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	dataStream, err := connection.CreateStream(headers)
	if err != nil {
		return nil, kerrors.NewAggregate([]error{
			errors.Wrap(err, "error creating forwarding stream"),
			connection.Close(),
		})
	}

	// Create the net.Conn and return.
	return NewConn(connection, dataStream), nil
}

// DialTimeout sets the timeout.
func DialTimeout(duration time.Duration) func(*Dialer) error {
	return func(d *Dialer) error {
		return d.setTimeout(duration)
	}
}

func (d *Dialer) setTimeout(duration time.Duration) error {
	d.timeout = duration
	return nil
}
