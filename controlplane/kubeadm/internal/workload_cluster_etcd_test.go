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
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	fake2 "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

func TestUpdateEtcdExternalInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		clusterConfigurationData string
		externalEtcd             bootstrapv1.ExternalEtcd
		wantClusterConfiguration string
	}{
		{
			name: "it should set external etcd configuration with external etcd",
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				etcd:
				  external: {}
				`),
			externalEtcd: bootstrapv1.ExternalEtcd{
				Endpoints: []string{"1.2.3.4"},
				CAFile:    "/tmp/ca_file.pem",
				CertFile:  "/tmp/cert_file.crt",
				KeyFile:   "/tmp/key_file.key",
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta3
				controllerManager: {}
				dns: {}
				etcd:
				  external:
				    caFile: /tmp/ca_file.pem
				    certFile: /tmp/cert_file.crt
				    endpoints:
				    - 1.2.3.4
				    keyFile: /tmp/key_file.key
				kind: ClusterConfiguration
				kubernetesVersion: v1.23.1
				networking: {}
				scheduler: {}
				`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateClusterConfiguration(ctx, semver.MustParse("1.23.1"), w.UpdateEtcdExternalInKubeadmConfigMap(tt.externalEtcd))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.wantClusterConfiguration), cmp.Diff(tt.wantClusterConfiguration, actualConfig.Data[clusterConfigurationKey]))
		})
	}
}

func TestUpdateEtcdLocalInKubeadmConfigMap(t *testing.T) {
	tests := []struct {
		name                     string
		version                  semver.Version
		clusterConfigurationData string
		localEtcd                bootstrapv1.LocalEtcd
		wantClusterConfiguration string
	}{
		{
			name:    "it should set local etcd configuration with local etcd (<1.31)",
			version: semver.MustParse("1.23.1"),
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta3
				kind: ClusterConfiguration
				etcd:
				  local: {}
				`),
			localEtcd: bootstrapv1.LocalEtcd{
				ImageRepository: "example.com/k8s",
				ImageTag:        "v1.6.0",
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "foo",
						Value: ptr.To("bar"),
					},
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta3
				controllerManager: {}
				dns: {}
				etcd:
				  local:
				    dataDir: ""
				    extraArgs:
				      foo: bar
				    imageRepository: example.com/k8s
				    imageTag: v1.6.0
				kind: ClusterConfiguration
				kubernetesVersion: v1.23.1
				networking: {}
				scheduler: {}
				`),
		},
		{
			name:    "it should set local etcd configuration with local etcd (>=1.31)",
			version: semver.MustParse("1.31.1"),
			clusterConfigurationData: utilyaml.Raw(`
				apiVersion: kubeadm.k8s.io/v1beta4
				kind: ClusterConfiguration
				etcd:
				  local: {}
				`),
			localEtcd: bootstrapv1.LocalEtcd{
				ImageRepository: "example.com/k8s",
				ImageTag:        "v1.6.0",
				ExtraArgs: []bootstrapv1.Arg{
					{
						Name:  "foo",
						Value: ptr.To("bar"),
					},
				},
			},
			wantClusterConfiguration: utilyaml.Raw(`
				apiServer: {}
				apiVersion: kubeadm.k8s.io/v1beta4
				controllerManager: {}
				dns: {}
				etcd:
				  local:
				    dataDir: ""
				    extraArgs:
				    - name: foo
				      value: bar
				    imageRepository: example.com/k8s
				    imageTag: v1.6.0
				kind: ClusterConfiguration
				kubernetesVersion: v1.31.1
				networking: {}
				proxy: {}
				scheduler: {}
				`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      kubeadmConfigKey,
					Namespace: metav1.NamespaceSystem,
				},
				Data: map[string]string{
					clusterConfigurationKey: tt.clusterConfigurationData,
				},
			}).Build()

			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateClusterConfiguration(ctx, tt.version, w.UpdateEtcdLocalInKubeadmConfigMap(tt.localEtcd))
			g.Expect(err).ToNot(HaveOccurred())

			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				client.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).Should(Equal(tt.wantClusterConfiguration), cmp.Diff(tt.wantClusterConfiguration, actualConfig.Data[clusterConfigurationKey]))
		})
	}
}

func TestRemoveEtcdMember(t *testing.T) {
	cp1Node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cp1",
			Namespace: "cp1",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
	}

	tests := []struct {
		name                string
		memberToDelete      string
		etcdClientGenerator etcdClientFor
		nodes               []client.Object
		expectErr           bool
	}{
		{
			name:                "returns an error if it fails to create the etcd client",
			memberToDelete:      "cp1",
			etcdClientGenerator: &fakeEtcdClientGenerator{forNodesErr: errors.New("no client")},
			expectErr:           true,
		},
		{
			name:           "returns an error if the client errors getting etcd members",
			memberToDelete: "cp1",
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						MemberListError: errors.New("cannot get etcd members"),
					},
				},
			},
			expectErr: true,
		},
		{
			name:           "no op if the member already does not exist",
			memberToDelete: "cp2",
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "cp1", ID: uint64(1)},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:           "returns an error if there is only one member",
			memberToDelete: "cp1",
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "cp1", ID: uint64(1)},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name:           "returns an error if the client errors removing the etcd member",
			memberToDelete: "cp1",
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "cp1", ID: uint64(1)},
								{Name: "cp2", ID: uint64(2)},
							},
						},
						MemberRemoveError: errors.New("cannot remove etcd members"),
					},
				},
			},
			expectErr: true,
		},
		{
			name:           "removes the member from etcd",
			memberToDelete: "cp1",
			etcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "cp1", ID: uint64(1)},
								{Name: "cp2", ID: uint64(2)},
							},
						},
					},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(cp1Node).Build()
			w := &Workload{
				Client:              fakeClient,
				etcdClientGenerator: tt.etcdClientGenerator,
			}
			err := w.RemoveEtcdMember(ctx, tt.memberToDelete)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestForwardEtcdLeadership(t *testing.T) {
	t.Run("handles errors correctly", func(t *testing.T) {
		tests := []struct {
			name                string
			machine             *clusterv1.Machine
			leaderCandidate     *clusterv1.Machine
			etcdClientGenerator etcdClientFor
			k8sClient           client.Client
			expectErr           bool
		}{
			{
				name:      "does nothing if the machine is nil",
				machine:   nil,
				expectErr: false,
			},
			{
				name: "does nothing if machine's NodeRef is nil",
				machine: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef = clusterv1.MachineNodeReference{}
				}),
				expectErr: false,
			},
			{
				name:            "returns an error if the leader candidate is nil",
				machine:         defaultMachine(),
				leaderCandidate: nil,
				expectErr:       true,
			},
			{
				name:    "returns an error if the leader candidate's noderef is nil",
				machine: defaultMachine(),
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef = clusterv1.MachineNodeReference{}
				}),
				expectErr: true,
			},
			{
				name:            "returns an error if it can't retrieve the list of control plane nodes",
				machine:         defaultMachine(),
				leaderCandidate: defaultMachine(),
				k8sClient:       &fakeClient{listErr: errors.New("failed to list nodes")},
				expectErr:       true,
			},
			{
				name:                "returns an error if it can't create an etcd client",
				machine:             defaultMachine(),
				leaderCandidate:     defaultMachine(),
				k8sClient:           &fakeClient{},
				etcdClientGenerator: &fakeEtcdClientGenerator{forLeaderErr: errors.New("no etcdClient")},
				expectErr:           true,
			},
			{
				name:            "returns error if it fails to get etcd members",
				machine:         defaultMachine(),
				leaderCandidate: defaultMachine(),
				k8sClient:       &fakeClient{},
				etcdClientGenerator: &fakeEtcdClientGenerator{
					forLeaderClient: &etcd.Client{
						EtcdClient: &fake2.FakeEtcdClient{
							MemberListError: errors.New("cannot get etcd members"),
						},
					},
				},
				expectErr: true,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				w := &Workload{
					Client:              tt.k8sClient,
					etcdClientGenerator: tt.etcdClientGenerator,
				}
				err := w.ForwardEtcdLeadership(ctx, tt.machine, tt.leaderCandidate)
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
			})
		}
	})

	t.Run("does nothing if the machine is not the leader", func(t *testing.T) {
		g := NewWithT(t)
		fakeEtcdClient := &fake2.FakeEtcdClient{
			MemberListResponse: &clientv3.MemberListResponse{
				Members: []*pb.Member{
					{Name: "machine-node", ID: uint64(101)},
				},
			},
			AlarmResponse: &clientv3.AlarmResponse{
				Alarms: []*pb.AlarmMember{},
			},
		}
		etcdClientGenerator := &fakeEtcdClientGenerator{
			forLeaderClient: &etcd.Client{
				EtcdClient: fakeEtcdClient,
				LeaderID:   555,
			},
		}

		w := &Workload{
			Client: &fakeClient{list: &corev1.NodeList{
				Items: []corev1.Node{nodeNamed("leader-node")},
			}},
			etcdClientGenerator: etcdClientGenerator,
		}
		err := w.ForwardEtcdLeadership(ctx, defaultMachine(), defaultMachine())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(fakeEtcdClient.MovedLeader).To(BeEquivalentTo(0))
	})

	t.Run("move etcd leader", func(t *testing.T) {
		tests := []struct {
			name               string
			leaderCandidate    *clusterv1.Machine
			etcdMoveErr        error
			expectedMoveLeader uint64
			expectErr          bool
		}{
			{
				name: "it moves the etcd leadership to the leader candidate",
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef.Name = "candidate-node"
				}),
				expectedMoveLeader: 12345,
			},
			{
				name: "returns error if failed to move to the leader candidate",
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef.Name = "candidate-node"
				}),
				etcdMoveErr: errors.New("move err"),
				expectErr:   true,
			},
			{
				name: "returns error if the leader candidate doesn't exist in etcd",
				leaderCandidate: defaultMachine(func(m *clusterv1.Machine) {
					m.Status.NodeRef.Name = "some other node"
				}),
				expectErr: true,
			},
		}

		currentLeader := defaultMachine(func(m *clusterv1.Machine) {
			m.Status.NodeRef.Name = "current-leader"
		})
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				fakeEtcdClient := &fake2.FakeEtcdClient{
					MemberListResponse: &clientv3.MemberListResponse{
						Members: []*pb.Member{
							{Name: currentLeader.Status.NodeRef.Name, ID: uint64(101)},
							{Name: "other-node", ID: uint64(1034)},
							{Name: "candidate-node", ID: uint64(12345)},
						},
					},
					AlarmResponse: &clientv3.AlarmResponse{
						Alarms: []*pb.AlarmMember{},
					},
					MoveLeaderError: tt.etcdMoveErr,
				}

				etcdClientGenerator := &fakeEtcdClientGenerator{
					forLeaderClient: &etcd.Client{
						EtcdClient: fakeEtcdClient,
						// this etcdClient belongs to the machine-node
						LeaderID: 101,
					},
				}

				w := &Workload{
					etcdClientGenerator: etcdClientGenerator,
					Client: &fakeClient{list: &corev1.NodeList{
						Items: []corev1.Node{nodeNamed("leader-node"), nodeNamed("other-node"), nodeNamed("candidate-node")},
					}},
				}
				err := w.ForwardEtcdLeadership(ctx, currentLeader, tt.leaderCandidate)
				if tt.expectErr {
					g.Expect(err).To(HaveOccurred())
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(fakeEtcdClient.MovedLeader).To(BeEquivalentTo(tt.expectedMoveLeader))
			})
		}
	})
}

type fakeEtcdClientGenerator struct {
	forNodesClient     *etcd.Client
	forNodesClientFunc func([]string) (*etcd.Client, error)
	forLeaderClient    *etcd.Client
	forNodesErr        error
	forLeaderErr       error
}

func (c *fakeEtcdClientGenerator) forFirstAvailableNode(_ context.Context, n []string) (*etcd.Client, error) {
	if c.forNodesClientFunc != nil {
		return c.forNodesClientFunc(n)
	}
	return c.forNodesClient, c.forNodesErr
}

func (c *fakeEtcdClientGenerator) forLeader(_ context.Context, _ []string) (*etcd.Client, error) {
	return c.forLeaderClient, c.forLeaderErr
}

func defaultMachine(transforms ...func(m *clusterv1.Machine)) *clusterv1.Machine {
	m := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: clusterv1.MachineNodeReference{
				Name: "machine-node",
			},
		},
	}
	for _, t := range transforms {
		t(m)
	}
	return m
}

func nodeNamed(name string) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return node
}
