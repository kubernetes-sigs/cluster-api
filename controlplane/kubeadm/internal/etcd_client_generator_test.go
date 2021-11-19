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

package internal

import (
	"context"
	"crypto/tls"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdfake "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
)

var (
	subject *EtcdClientGenerator
)

func TestNewEtcdClientGenerator(t *testing.T) {
	g := NewWithT(t)
	subject = NewEtcdClientGenerator(&rest.Config{}, &tls.Config{MinVersion: tls.VersionTLS12})
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
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return &etcd.Client{Endpoint: endpoints[0]}, nil
			},
			expectedClient: etcd.Client{Endpoint: "etcd-node-1"},
		},
		{
			name:        "Fails when called with an empty node list",
			nodes:       nil,
			cc:          nil,
			expectedErr: "invalid argument: forLeader can't be called with an empty list of nodes",
		},
		{
			name:  "Returns error from client",
			nodes: []string{"node-1", "node-2"},
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return nil, errors.New("something went wrong")
			},
			expectedErr: "could not establish a connection to any etcd node: something went wrong",
		},
		{
			name:  "Returns client when some of the nodes are down but at least one node is up",
			nodes: []string{"node-down-1", "node-down-2", "node-up"},
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				if strings.Contains(endpoints[0], "node-down") {
					return nil, errors.New("node down")
				}

				return &etcd.Client{Endpoint: endpoints[0]}, nil
			},
			expectedClient: etcd.Client{Endpoint: "etcd-node-up"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			subject = NewEtcdClientGenerator(&rest.Config{}, &tls.Config{MinVersion: tls.VersionTLS12})
			subject.createClient = tt.cc

			client, err := subject.forFirstAvailableNode(ctx, tt.nodes)

			if tt.expectedErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).Should(Equal(tt.expectedErr))
			} else {
				g.Expect(*client).Should(Equal(tt.expectedClient))
			}
		})
	}
}

func TestForLeader(t *testing.T) {
	tests := []struct {
		name  string
		nodes []string
		cc    clientCreator

		expectedErr    string
		expectedClient etcd.Client
	}{
		{
			name:  "Returns client for leader successfully",
			nodes: []string{"node-1", "node-leader"},
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return &etcd.Client{
					Endpoint: endpoints[0],
					LeaderID: 1729,
					EtcdClient: &etcdfake.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*etcdserverpb.Member{
								{ID: 1234, Name: "node-1"},
								{ID: 1729, Name: "node-leader"},
							},
						},
						AlarmResponse: &clientv3.AlarmResponse{},
					}}, nil
			},
			expectedClient: etcd.Client{
				Endpoint: "etcd-node-leader",
				LeaderID: 1729, EtcdClient: &etcdfake.FakeEtcdClient{
					MemberListResponse: &clientv3.MemberListResponse{
						Members: []*etcdserverpb.Member{
							{ID: 1234, Name: "node-1"},
							{ID: 1729, Name: "node-leader"},
						},
					},
					AlarmResponse: &clientv3.AlarmResponse{},
				}},
		},
		{
			name:  "Returns client for leader even when one or more nodes are down",
			nodes: []string{"node-down-1", "node-down-2", "node-leader"},
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				if strings.Contains(endpoints[0], "node-down") {
					return nil, errors.New("node down")
				}
				return &etcd.Client{
					Endpoint: endpoints[0],
					LeaderID: 1729,
					EtcdClient: &etcdfake.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*etcdserverpb.Member{
								{ID: 1729, Name: "node-leader"},
							},
						},
						AlarmResponse: &clientv3.AlarmResponse{},
					}}, nil
			},
			expectedClient: etcd.Client{
				Endpoint: "etcd-node-leader",
				LeaderID: 1729, EtcdClient: &etcdfake.FakeEtcdClient{
					MemberListResponse: &clientv3.MemberListResponse{
						Members: []*etcdserverpb.Member{
							{ID: 1729, Name: "node-leader"},
						},
					},
					AlarmResponse: &clientv3.AlarmResponse{},
				}},
		},
		{
			name:        "Fails when called with an empty node list",
			nodes:       nil,
			cc:          nil,
			expectedErr: "invalid argument: forLeader can't be called with an empty list of nodes",
		},
		{
			name:  "Returns error when the leader does not have a corresponding node",
			nodes: []string{"node-1"},
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return &etcd.Client{
					Endpoint: endpoints[0],
					LeaderID: 1729,
					EtcdClient: &etcdfake.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*etcdserverpb.Member{
								{ID: 1234, Name: "node-1"},
								{ID: 1729, Name: "node-leader"},
							},
						},
						AlarmResponse: &clientv3.AlarmResponse{},
					}}, nil
			},
			expectedErr: "etcd leader is reported as 6c1 with name \"node-leader\", but we couldn't find a corresponding Node in the cluster",
		},
		{
			name:  "Returns error when all nodes are down",
			nodes: []string{"node-down-1", "node-down-2", "node-down-3"},
			cc: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return nil, errors.New("node down")
			},
			expectedErr: "could not establish a connection to the etcd leader: could not establish a connection to any etcd node: node down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			subject = NewEtcdClientGenerator(&rest.Config{}, &tls.Config{MinVersion: tls.VersionTLS12})
			subject.createClient = tt.cc

			client, err := subject.forLeader(ctx, tt.nodes)

			if tt.expectedErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).Should(Equal(tt.expectedErr))
			} else {
				g.Expect(*client).Should(Equal(tt.expectedClient))
			}
		})
	}
}
