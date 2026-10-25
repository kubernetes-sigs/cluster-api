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

package pkg

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	"go.etcd.io/etcd/client/pkg/v3/logutil"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg/etcd"
)

var (
	subject             *EtcdClientGenerator
	etcdClientLogger, _ = logutil.CreateDefaultZapLogger(zapcore.InfoLevel)
)

func TestNewEtcdClientGenerator(t *testing.T) {
	g := NewWithT(t)
	subject = NewEtcdClientGenerator(&rest.Config{}, &tls.Config{MinVersion: tls.VersionTLS12}, 0, 0, etcdClientLogger)
	g.Expect(subject.createClient).To(Not(BeNil()))
}

func TestFirstAvailableNode(t *testing.T) {
	tests := []struct {
		name  string
		nodes []string
		cc    clientCreator

		expectedErr    string
		expectedClient etcd.Client
	}{
		{
			name:  "Returns client successfully",
			nodes: []string{"node-1"},
			cc: func(_ context.Context, endpoint string) (*etcd.Client, error) {
				return &etcd.Client{Endpoint: endpoint}, nil
			},
			expectedClient: etcd.Client{Endpoint: "etcd-node-1"},
		},
		{
			name:        "Fails when called with an empty node list",
			nodes:       nil,
			cc:          nil,
			expectedErr: "invalid argument: forFirstAvailableNode can't be called with an empty list of nodes",
		},
		{
			name:  "Returns error from client",
			nodes: []string{"node-1", "node-2"},
			cc: func(context.Context, string) (*etcd.Client, error) {
				return nil, pkgerrors.New("something went wrong")
			},
			expectedErr: "could not establish a connection to etcd members hosted on node-1,node-2: something went wrong",
		},
		{
			name:  "Returns client when some of the nodes are down but at least one node is up",
			nodes: []string{"node-down-1", "node-down-2", "node-up"},
			cc: func(_ context.Context, endpoint string) (*etcd.Client, error) {
				if strings.Contains(endpoint, "node-down") {
					return nil, pkgerrors.New("node down")
				}

				return &etcd.Client{Endpoint: endpoint}, nil
			},
			expectedClient: etcd.Client{Endpoint: "etcd-node-up"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			subject = NewEtcdClientGenerator(&rest.Config{}, &tls.Config{MinVersion: tls.VersionTLS12}, 0, 0, etcdClientLogger)
			subject.createClient = tt.cc

			client, err := subject.forFirstAvailableNode(ctx, tt.nodes)

			if tt.expectedErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).Should(BeComparableTo(tt.expectedErr))
			} else {
				g.Expect(*client).Should(BeComparableTo(tt.expectedClient))
			}
		})
	}
}
