/*
Copyright The Kubernetes Authors.

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
	"io"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/httpstream" //nolint:staticcheck // Keep using this package for now as it's not straightforward to migrate this to k8s.io/streaming/pkg/httpstream.
)

// fakeStream is an httpstream.Stream backed by an io.Pipe, so Read blocks
// until data is written or the stream is closed.
type fakeStream struct {
	*io.PipeReader
	*io.PipeWriter
}

func newFakeStream() *fakeStream {
	r, w := io.Pipe()
	return &fakeStream{PipeReader: r, PipeWriter: w}
}

func (s *fakeStream) Close() error {
	_ = s.PipeWriter.Close()
	return s.PipeReader.Close()
}

func (s *fakeStream) Reset() error         { return s.Close() }
func (s *fakeStream) Headers() http.Header { return nil }
func (s *fakeStream) Identifier() uint32   { return 0 }

// fakeConnection is an httpstream.Connection that records whether Close was called.
type fakeConnection struct {
	closed chan struct{}
}

func newFakeConnection() *fakeConnection {
	return &fakeConnection{closed: make(chan struct{})}
}

func (f *fakeConnection) CreateStream(_ http.Header) (httpstream.Stream, error) { return nil, nil }
func (f *fakeConnection) CloseChan() <-chan bool                                { return nil }
func (f *fakeConnection) SetIdleTimeout(_ time.Duration)                        {}
func (f *fakeConnection) RemoveStreams(_ ...httpstream.Stream)                  {}

func (f *fakeConnection) Close() error {
	select {
	case <-f.closed:
		// already closed
	default:
		close(f.closed)
	}
	return nil
}

func TestConn_SetReadDeadline_UnblocksPendingRead(t *testing.T) {
	g := NewWithT(t)

	stream := newFakeStream()
	connection := newFakeConnection()
	conn := NewConn(connection, stream)

	g.Expect(conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))).To(Succeed())

	errCh := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		errCh <- err
	}()

	select {
	case err := <-errCh:
		g.Expect(err).To(HaveOccurred())
	case <-time.After(5 * time.Second):
		t.Fatal("Read did not unblock after read deadline expired")
	}

	// The underlying connection must have been closed to unblock the read.
	select {
	case <-connection.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("underlying connection was not closed on deadline expiry")
	}
}

func TestConn_SetDeadline_InThePast_ClosesImmediately(t *testing.T) {
	g := NewWithT(t)

	stream := newFakeStream()
	connection := newFakeConnection()
	conn := NewConn(connection, stream)

	g.Expect(conn.SetDeadline(time.Now().Add(-time.Second))).To(Succeed())

	select {
	case <-connection.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("connection was not closed for a deadline already in the past")
	}
}

func TestConn_SetDeadline_Zero_CancelsTimer(t *testing.T) {
	g := NewWithT(t)

	stream := newFakeStream()
	connection := newFakeConnection()
	conn := NewConn(connection, stream)

	g.Expect(conn.SetDeadline(time.Now().Add(50 * time.Millisecond))).To(Succeed())
	g.Expect(conn.SetDeadline(time.Time{})).To(Succeed())

	select {
	case <-connection.closed:
		t.Fatal("connection was closed even though the deadline was cleared")
	case <-time.After(5 * time.Second):
		// Expected: no close happened.
	}
}

func TestConn_Close_StopsPendingDeadlineTimer(t *testing.T) {
	g := NewWithT(t)

	stream := newFakeStream()
	connection := newFakeConnection()
	conn := NewConn(connection, stream)

	g.Expect(conn.SetDeadline(time.Now().Add(time.Hour))).To(Succeed())
	g.Expect(conn.Close()).To(Succeed())

	g.Expect(conn.deadlineTimer).To(BeNil())
}
