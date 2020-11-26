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
	"errors"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/etcdserverpb"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdfake "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
)

var (
	subject *etcdClientGenerator
)

func TestGetClientFactoryDefault(t *testing.T) {
	g := NewWithT(t)
	subject = &etcdClientGenerator{
		restConfig: &rest.Config{},
		tlsConfig:  &tls.Config{},
	}
	subject.getClientFactory()
	g.Expect(subject.clientFactory).To(Not(BeNil()))
}

func TestForNodes(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name  string
		nodes []string
		cf    clientFactory

		expectedErr    string
		expectedClient etcd.Client
	}{
		{
			name:  "Returns client successfully",
			nodes: []string{"node-1"},
			cf: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return &etcd.Client{Endpoint: endpoints[0]}, nil
			},
			expectedClient: etcd.Client{Endpoint: "etcd-node-1"},
		},
		{
			name:  "Returns error",
			nodes: []string{"node-1"},
			cf: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return nil, errors.New("something went wrong")
			},
			expectedErr: "something went wrong",
		},
	}

	for _, tt := range tests {
		subject = &etcdClientGenerator{
			restConfig:    &rest.Config{},
			tlsConfig:     &tls.Config{},
			clientFactory: tt.cf,
		}

		client, err := subject.forNodes(ctx, toNodes(tt.nodes))

		if tt.expectedErr != "" {
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).Should(Equal(tt.expectedErr))
		} else {
			g.Expect(*client).Should(Equal(tt.expectedClient))
		}

	}
}

func TestForLeader(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name  string
		nodes []string
		cf    clientFactory

		expectedErr    string
		expectedClient etcd.Client
	}{
		{
			name:  "Returns client for leader successfully",
			nodes: []string{"node-1", "node-leader"},
			cf: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
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
						AlarmResponse: &clientv3.AlarmResponse{
						},
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
					AlarmResponse: &clientv3.AlarmResponse{
					},
				}},
		},

		{
			name:  "Returns client for leader even when one or more nodes are down",
			nodes: []string{"node-down-1", "node-down-2", "node-leader"},
			cf: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
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
						AlarmResponse: &clientv3.AlarmResponse{
						},
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
					AlarmResponse: &clientv3.AlarmResponse{
					},
				}},
		},
		{
			name:  "Returns error when all nodes are down",
			nodes: []string{"node-down-1", "node-down-2", "node-down-3"},
			cf: func(ctx context.Context, endpoints []string) (*etcd.Client, error) {
				return nil, errors.New("node down")
			},
			expectedErr: "could not establish a connection to the etcd leader: node down",
		},
	}

	for _, tt := range tests {
		subject = &etcdClientGenerator{
			restConfig:    &rest.Config{},
			tlsConfig:     &tls.Config{},
			clientFactory: tt.cf,
		}

		client, err := subject.forLeader(ctx, toNodes(tt.nodes))

		if tt.expectedErr != "" {
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).Should(Equal(tt.expectedErr))
		} else {
			g.Expect(*client).Should(Equal(tt.expectedClient))
		}
	}

}

func toNodes(nodeNames []string) []corev1.Node {
	nodes := make([]corev1.Node, len(nodeNames))
	for i, n := range nodeNames {
		nodes[i] = corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: n},
		}
	}
	return nodes
}
