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
	"sync"
	"time"

	kerrors "k8s.io/apimachinery/pkg/util/errors"
	//nolint:staticcheck // Keep using this package for now, see comment in dial.go
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// Conn is a Kubernetes API server proxied type of net/conn.
type Conn struct {
	connection httpstream.Connection
	stream     httpstream.Stream

	readDeadline  time.Time
	writeDeadline time.Time

	deadlineTimerLock sync.Mutex
	deadlineTimer     *time.Timer
}

// Read from the connection.
func (c *Conn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

// Close the underlying proxied connection.
func (c *Conn) Close() error {
	c.deadlineTimerLock.Lock()
	if c.deadlineTimer != nil {
		c.deadlineTimer.Stop()
		c.deadlineTimer = nil
	}
	c.deadlineTimerLock.Unlock()

	return kerrors.NewAggregate([]error{c.stream.Close(), c.connection.Close()})
}

// Write to the connection.
func (c *Conn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

// LocalAddr returns a fake address representing the proxied connection.
func (c *Conn) LocalAddr() net.Addr {
	return NewAddrFromConn(c)
}

// RemoteAddr returns a fake address representing the proxied connection.
func (c *Conn) RemoteAddr() net.Addr {
	return NewAddrFromConn(c)
}

// SetDeadline sets the read and write deadlines to the specified interval.
func (c *Conn) SetDeadline(t time.Time) error {
	c.deadlineTimerLock.Lock()
	defer c.deadlineTimerLock.Unlock()
	c.readDeadline = t
	c.writeDeadline = t
	c.armDeadlineTimerLocked()
	return nil
}

// SetWriteDeadline sets the write deadline to the specified time.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.deadlineTimerLock.Lock()
	defer c.deadlineTimerLock.Unlock()
	c.writeDeadline = t
	c.armDeadlineTimerLocked()
	return nil
}

// SetReadDeadline sets the read deadline to the specified time.
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.deadlineTimerLock.Lock()
	defer c.deadlineTimerLock.Unlock()
	c.readDeadline = t
	c.armDeadlineTimerLocked()
	return nil
}

// armDeadlineTimerLocked (re)arms the timer that enforces the earliest of the
// configured read/write deadlines by closing the underlying connection when
// it elapses. httpstream.Connection has no concept of an absolute deadline
// (only SetIdleTimeout, a rolling duration), so closing on expiry is the only
// way to unblock a stream.Read/Write call that ignores context cancellation,
// which is required for callers like tls.HandshakeContext and the gRPC
// transport that rely on net.Conn deadlines to interrupt blocked I/O.
// c.mu must be held by the caller.
func (c *Conn) armDeadlineTimerLocked() {
	if c.deadlineTimer != nil {
		c.deadlineTimer.Stop()
		c.deadlineTimer = nil
	}

	deadline := earliestDeadline(c.readDeadline, c.writeDeadline)
	if deadline.IsZero() {
		return
	}

	d := time.Until(deadline)
	if d < 0 {
		d = 0
	}
	c.deadlineTimer = time.AfterFunc(d, func() {
		_ = c.Close()
	})
}

// earliestDeadline returns the earlier of a and b, treating the zero value
// (no deadline) as later than any set deadline.
func earliestDeadline(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case a.Before(b):
		return a
	default:
		return b
	}
}

// NewConn creates a new net/conn interface based on an underlying Kubernetes
// API server proxy connection.
func NewConn(connection httpstream.Connection, stream httpstream.Stream) *Conn {
	return &Conn{
		connection: connection,
		stream:     stream,
	}
}
