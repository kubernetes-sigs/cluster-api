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
	"net"
	"time"

	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// Conn is a Kubernetes API server proxied type of net/conn.
type Conn struct {
	connection    httpstream.Connection
	stream        httpstream.Stream
	readDeadline  time.Time
	writeDeadline time.Time
}

// Read from the connection.
func (c Conn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

// Close the underlying proxied connection.
func (c Conn) Close() error {
	return kerrors.NewAggregate([]error{c.stream.Close(), c.connection.Close()})
}

// Write to the connection.
func (c Conn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

// LocalAddr returns a fake address representing the proxied connection.
func (c Conn) LocalAddr() net.Addr {
	return NewAddrFromConn(c)
}

// RemoteAddr returns a fake address representing the proxied connection.
func (c Conn) RemoteAddr() net.Addr {
	return NewAddrFromConn(c)
}

// SetDeadline sets the read and write deadlines to the specified interval.
func (c Conn) SetDeadline(t time.Time) error {
	// TODO: Handle deadlines
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

// SetWriteDeadline sets the read and write deadlines to the specified interval.
func (c Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

// SetReadDeadline sets the read and write deadlines to the specified interval.
func (c Conn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

// NewConn creates a new net/conn interface based on an underlying Kubernetes
// API server proxy connection.
func NewConn(connection httpstream.Connection, stream httpstream.Stream) Conn {
	return Conn{
		connection: connection,
		stream:     stream,
	}
}
