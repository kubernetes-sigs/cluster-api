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

package portforward

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/klog/v2"
)

// HTTPStreamReceived is the httpstream.NewStreamHandler for port
// forward streams. Each valid stream is sent to the streams channel.
func HTTPStreamReceived(streamsCh chan httpstream.Stream) func(httpstream.Stream, <-chan struct{}) error {
	return func(stream httpstream.Stream, _ <-chan struct{}) error {
		// make sure it has a valid stream type header
		streamType := stream.Headers().Get(corev1.StreamType)
		if streamType == "" {
			return fmt.Errorf("%q header is required", corev1.StreamType)
		}
		if streamType != corev1.StreamTypeError && streamType != corev1.StreamTypeData {
			return fmt.Errorf("invalid stream type %q", streamType)
		}

		streamsCh <- stream
		return nil
	}
}

// NewHTTPStreamHandler returns a new httpStreamHandler capable of processing multiple port forward
// operations over a single httpstream.Connection.
func NewHTTPStreamHandler(conn httpstream.Connection, streamsCh chan httpstream.Stream, podName, podNamespace string, forwarder PortForwarder) HTTPStreamHandler {
	return &httpStreamHandler{
		conn:                  conn,
		streamChan:            streamsCh,
		streamPairs:           make(map[string]*httpStreamPair),
		streamCreationTimeout: 30 * time.Second,
		podName:               podName,
		podNamespace:          podNamespace,
		forwarder:             forwarder,
	}
}

// HTTPStreamHandler is capable of processing multiple port forward
// requests over a single httpstream.Connection.
type HTTPStreamHandler interface {
	Run(ctx context.Context)
}

// httpStreamHandler is capable of processing multiple port forward
// requests over a single httpstream.Connection.
type httpStreamHandler struct {
	// TODO: consider setting log.
	log                   logr.Logger
	conn                  httpstream.Connection
	streamChan            chan httpstream.Stream
	streamPairsLock       sync.RWMutex
	streamPairs           map[string]*httpStreamPair
	streamCreationTimeout time.Duration
	podName               string
	podNamespace          string
	forwarder             PortForwarder
}

// PortForwarder knows how to forward content from a data stream to/from a target (usually a port in a pod).
type PortForwarder func(ctx context.Context, podName, podNamespace, port string, stream io.ReadWriteCloser) error

// Run is the main loop for the HTTPStreamHandler. It processes new
// streams, invoking portForward for each complete stream pair. The loop exits
// when the httpstream.Connection is closed.
//
// Notes:
//   - two streams for each operation over the port forward connection, the data stream and the error stream;
//     both streams can be identified by using the requestID.
//   - it is required to wait for both the stream before stating the actual part forward.
//   - streams pair are kept around until the operation completes.
func (h *httpStreamHandler) Run(ctx context.Context) {
	h.log.V(4).Info("Port-forward: connection waiting for streams", "Pod", klog.KRef(h.podNamespace, h.podName))
Loop:
	for {
		select {
		case <-h.conn.CloseChan():
			h.log.V(4).Info("Port-forward: connection closed", "Pod", klog.KRef(h.podNamespace, h.podName))
			break Loop
		case stream := <-h.streamChan:
			requestID := h.requestID(stream)
			streamType := stream.Headers().Get(corev1.StreamType)
			h.log.V(4).Info("Port-forward: connection request received new type of stream", "Pod", klog.KRef(h.podNamespace, h.podName), "request", requestID, "streamType", streamType)

			p, created := h.getStreamPair(requestID)
			if created {
				go h.monitorStreamPair(p, time.After(h.streamCreationTimeout))
			}
			if complete, err := p.add(stream); err != nil {
				h.log.Error(err, "Port-forward: error processing stream", "Pod", klog.KRef(h.podNamespace, h.podName), "request", requestID, "streamType", streamType)
				err := fmt.Errorf("error processing stream for request %s: %w", requestID, err)
				p.printError(err.Error())
			} else if complete {
				go h.portForward(ctx, p)
			}
		}
	}
}

// requestID returns the request id for stream.
func (h *httpStreamHandler) requestID(stream httpstream.Stream) string {
	requestID := stream.Headers().Get(corev1.PortForwardRequestIDHeader)
	if requestID == "" {
		h.log.V(4).Info("Port-forward: connection stream received without requestID header", "Pod", klog.KRef(h.podNamespace, h.podName))
		// If we get here, it's because the connection came from an older client
		// that isn't generating the request id header
		// (https://github.com/kubernetes/kubernetes/blob/843134885e7e0b360eb5441e85b1410a8b1a7a0c/pkg/client/unversioned/portforward/portforward.go#L258-L287)
		//
		// This is a best-effort attempt at supporting older clients.
		//
		// When there aren't concurrent new forwarded connections, each connection
		// will have a pair of streams (data, error), and the stream IDs will be
		// consecutive odd numbers, e.g. 1 and 3 for the first connection. Convert
		// the stream ID into a pseudo-request id by taking the stream type and
		// using id = stream.Identifier() when the stream type is error,
		// and id = stream.Identifier() - 2 when it's data.
		//
		// NOTE: this only works when there are not concurrent new streams from
		// multiple forwarded connections; it's a best-effort attempt at supporting
		// old clients that don't generate request ids.  If there are concurrent
		// new connections, it's possible that 1 connection gets streams whose IDs
		// are not consecutive (e.g. 5 and 9 instead of 5 and 7).
		streamType := stream.Headers().Get(corev1.StreamType)
		switch streamType {
		case corev1.StreamTypeError:
			requestID = strconv.Itoa(int(stream.Identifier()))
		case corev1.StreamTypeData:
			requestID = strconv.Itoa(int(stream.Identifier()) - 2)
		}
		h.log.V(4).Info("Port-forward: connection automatically assigning request ID from stream type and stream ID", "Pod", klog.KRef(h.podNamespace, h.podName), "request", requestID, "streamType", streamType, "stream", stream.Identifier())
	}
	return requestID
}

// getStreamPair returns a httpStreamPair for requestID. This creates a
// new pair if one does not yet exist for the requestID. The returned bool is
// true if the pair was created.
func (h *httpStreamHandler) getStreamPair(requestID string) (*httpStreamPair, bool) {
	h.streamPairsLock.Lock()
	defer h.streamPairsLock.Unlock()

	if p, ok := h.streamPairs[requestID]; ok {
		h.log.V(4).Info("Port-forward: connection request found existing stream pair", "Pod", klog.KRef(h.podNamespace, h.podName), "request", requestID)
		return p, false
	}

	h.log.V(4).Info("Port-forward: connection request creating new stream pair", "Pod", klog.KRef(h.podNamespace, h.podName), "request", requestID)

	p := newPortForwardPair(requestID)
	h.streamPairs[requestID] = p

	return p, true
}

// monitorStreamPair waits for the pair to receive both its error and data
// streams, or for the timeout to expire (whichever happens first), and then
// removes the pair.
func (h *httpStreamHandler) monitorStreamPair(p *httpStreamPair, timeout <-chan time.Time) {
	select {
	case <-timeout:
		err := fmt.Errorf("(conn=%v, request=%s) timed out waiting for streams", h.conn, p.requestID)
		h.log.Error(err, "Port-forward: error processing stream", "Pod", klog.KRef(h.podNamespace, h.podName), "request", p.requestID)
		p.printError(err.Error())
	case <-p.complete:
		h.log.V(4).Info("Port-forward: connection request successfully received error and data streams", "Pod", klog.KRef(h.podNamespace, h.podName), "request", p.requestID)
	}
	h.removeStreamPair(p.requestID)
}

// removeStreamPair removes the stream pair identified by requestID from streamPairs.
func (h *httpStreamHandler) removeStreamPair(requestID string) {
	h.streamPairsLock.Lock()
	defer h.streamPairsLock.Unlock()

	if h.conn != nil {
		pair := h.streamPairs[requestID]
		h.conn.RemoveStreams(pair.dataStream, pair.errorStream)
	}
	delete(h.streamPairs, requestID)
}

// portForward invokes the HTTPStreamHandler's forwarder.PortForward
// function for the given stream pair.
func (h *httpStreamHandler) portForward(ctx context.Context, p *httpStreamPair) {
	defer func() {
		_ = p.errorStream.Close()
		_ = p.dataStream.Close()
	}()

	port := p.dataStream.Headers().Get(corev1.PortHeader)

	h.log.Info("Port-forward: connection request invoking forwarder.PortForward", "Pod", klog.KRef(h.podNamespace, h.podName), "request", p.requestID, "port", port)
	err := h.forwarder(ctx, h.podName, h.podNamespace, port, p.dataStream)
	h.log.V(4).Info("Port-forward: connection request done invoking forwarder.PortForward", "Pod", klog.KRef(h.podNamespace, h.podName), "request", p.requestID, "port", port)

	if err != nil {
		err := fmt.Errorf("error forwarding port %s to pod %s/%s: %w", port, h.podNamespace, h.podName, err)
		h.log.Error(err, "Port-forward: error processing request", "Pod", klog.KRef(h.podNamespace, h.podName), "request", p.requestID)
		fmt.Fprint(p.errorStream, err.Error())
	}
}

// httpStreamPair represents the error and data streams for a port
// forwarding request.
type httpStreamPair struct {
	lock        sync.RWMutex
	requestID   string
	dataStream  httpstream.Stream
	errorStream httpstream.Stream
	complete    chan struct{}
}

// newPortForwardPair creates a new httpStreamPair.
func newPortForwardPair(requestID string) *httpStreamPair {
	return &httpStreamPair{
		requestID: requestID,
		complete:  make(chan struct{}),
	}
}

// add adds the stream to the httpStreamPair. If the pair already
// contains a stream for the new stream's type, an error is returned. add
// returns true if both the data and error streams for this pair have been
// received.
func (p *httpStreamPair) add(stream httpstream.Stream) (bool, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	switch stream.Headers().Get(corev1.StreamType) {
	case corev1.StreamTypeError:
		if p.errorStream != nil {
			return false, fmt.Errorf("error stream already assigned")
		}
		p.errorStream = stream
	case corev1.StreamTypeData:
		if p.dataStream != nil {
			return false, fmt.Errorf("data stream already assigned")
		}
		p.dataStream = stream
	}

	complete := p.errorStream != nil && p.dataStream != nil
	if complete {
		close(p.complete)
	}
	return complete, nil
}

// printError writes s to p.errorStream if p.errorStream has been set.
func (p *httpStreamPair) printError(s string) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if p.errorStream != nil {
		fmt.Fprint(p.errorStream, s)
	}
}

// HTTPStreamTunnel create tunnels for two streams.
func HTTPStreamTunnel(ctx context.Context, c1, c2 io.ReadWriter) error {
	buf1 := make([]byte, 32*1024) // TODO: check if we can make smaller buffers
	buf2 := make([]byte, 32*1024)

	errCh := make(chan error)
	go func() {
		_, err := io.CopyBuffer(c2, c1, buf1)
		errCh <- err
	}()
	go func() {
		_, err := io.CopyBuffer(c1, c2, buf2)
		errCh <- err
	}()
	select {
	case <-ctx.Done():
		// Do nothing
	case err1 := <-errCh:
		select {
		case <-ctx.Done():
			if err1 != nil {
				return err1
			}
			// Do nothing
		case err2 := <-errCh:
			if err1 != nil {
				// TODO: Consider aggregating errors
				return err1
			}
			return err2
		}
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
