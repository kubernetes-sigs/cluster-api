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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	fake2 "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestUpdateEtcdConditions(t *testing.T) {
	tests := []struct {
		name                      string
		kcp                       *controlplanev1.KubeadmControlPlane
		machines                  []*clusterv1.Machine
		injectClient              client.Client // This test is injecting a fake client because it is required to create nodes with a controlled Status or to fail with a specific error.
		injectEtcdClientGenerator etcdClientFor // This test is injecting a fake etcdClientGenerator because it is required to nodes with a controlled Status or to fail with a specific error.
		expectedKCPCondition      *clusterv1.Condition
		expectedMachineConditions map[string]clusterv1.Conditions
	}{
		{
			name: "if list nodes return an error should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			injectClient: &fakeClient{
				listErr: errors.New("failed to list nodes"),
			},
			expectedKCPCondition: conditions.UnknownCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterInspectionFailedReason, "Failed to list nodes which are hosting the etcd members"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to get the node which is hosting the etcd member"),
				},
			},
		},
		{
			name: "node without machine should be ignored if there are provisioning machines",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"), // without NodeRef (provisioning)
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedKCPCondition: nil,
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
		},
		{
			name:     "node without machine should report a problem at KCP level if there are no provisioning machines",
			machines: []*clusterv1.Machine{},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Control plane node %s does not have a corresponding machine", "n1"),
		},
		{
			name: "failure creating the etcd client should report unknown condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesErr: errors.New("failed to get client for node"),
			},
			expectedKCPCondition: conditions.UnknownCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnknownReason, "Following machines are reporting unknown etcd member status: m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to connect to the etcd pod on the %s node: failed to get client for node", "n1"),
				},
			},
		},
		{
			name: "etcd client reporting status errors should be reflected into a false condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						EtcdEndpoints: []string{},
					},
					Errors: []string{"some errors"},
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting etcd member errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member status reports errors: %s", "some errors"),
				},
			},
		},
		{
			name: "failure listing members should report false condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						EtcdEndpoints: []string{},
						ErrorResponse: errors.New("failed to list members"),
					},
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting etcd member errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Failed get answer from the etcd member on the %s node", "n1"),
				},
			},
		},
		{
			name: "an etcd member with alarms should report false condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClient: &etcd.Client{
					EtcdClient: &fake2.FakeEtcdClient{
						EtcdEndpoints: []string{},
						MemberListResponse: &clientv3.MemberListResponse{
							Members: []*pb.Member{
								{Name: "n1", ID: uint64(1)},
							},
						},
						AlarmResponse: &clientv3.AlarmResponse{
							Alarms: []*pb.AlarmMember{
								{MemberID: uint64(1), Alarm: 1}, // NOSPACE
							},
						},
					},
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting etcd member errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports alarms: %s", "NOSPACE"),
				},
			},
		},
		{
			name: "etcd members with different Cluster ID should report false condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
				fakeMachine("m2", withNodeRef("n2")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{
						*fakeNode("n1"),
						*fakeNode("n2"),
					},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(n []string) (*etcd.Client, error) {
					switch n[0] {
					case "n1":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(1),
									},
									Members: []*pb.Member{
										{Name: "n1", ID: uint64(1)},
										{Name: "n2", ID: uint64(2)},
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					case "n2":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(2), // different Cluster ID
									},
									Members: []*pb.Member{
										{Name: "n1", ID: uint64(1)},
										{Name: "n2", ID: uint64(2)},
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					default:
						return nil, errors.New("no client for this node")
					}
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting etcd member errors: %s", "m2"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member has cluster ID %d, but all previously seen etcd members have cluster ID %d", uint64(2), uint64(1)),
				},
			},
		},
		{
			name: "etcd members with different member list should report false condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
				fakeMachine("m2", withNodeRef("n2")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{
						*fakeNode("n1"),
						*fakeNode("n2"),
					},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(n []string) (*etcd.Client, error) {
					switch n[0] {
					case "n1":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(1),
									},
									Members: []*pb.Member{
										{Name: "n1", ID: uint64(1)},
										{Name: "n2", ID: uint64(2)},
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					case "n2":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(1),
									},
									Members: []*pb.Member{ // different member list
										{Name: "n2", ID: uint64(2)},
										{Name: "n3", ID: uint64(3)},
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					default:
						return nil, errors.New("no client for this node")
					}
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting etcd member errors: %s", "m2"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member reports the cluster is composed by members [n2 n3], but all previously seen etcd members are reporting [n1 n2]"),
				},
			},
		},
		{
			name: "a machine without a member should report false condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
				fakeMachine("m2", withNodeRef("n2")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{
						*fakeNode("n1"),
						*fakeNode("n2"),
					},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(n []string) (*etcd.Client, error) {
					switch n[0] {
					case "n1":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(1),
									},
									Members: []*pb.Member{
										{Name: "n1", ID: uint64(1)},
										// member n2 is missing
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					default:
						return nil, errors.New("no client for this node")
					}
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting etcd member errors: %s", "m2"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Missing etcd member"),
				},
			},
		},
		{
			name: "healthy etcd members should report true",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
				fakeMachine("m2", withNodeRef("n2")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{
						*fakeNode("n1"),
						*fakeNode("n2"),
					},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(n []string) (*etcd.Client, error) {
					switch n[0] {
					case "n1":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(1),
									},
									Members: []*pb.Member{
										{Name: "n1", ID: uint64(1)},
										{Name: "n2", ID: uint64(2)},
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					case "n2":
						return &etcd.Client{
							EtcdClient: &fake2.FakeEtcdClient{
								EtcdEndpoints: []string{},
								MemberListResponse: &clientv3.MemberListResponse{
									Header: &pb.ResponseHeader{
										ClusterId: uint64(1),
									},
									Members: []*pb.Member{
										{Name: "n1", ID: uint64(1)},
										{Name: "n2", ID: uint64(2)},
									},
								},
								AlarmResponse: &clientv3.AlarmResponse{
									Alarms: []*pb.AlarmMember{},
								},
							},
						}, nil
					default:
						return nil, errors.New("no client for this node")
					}
				},
			},
			expectedKCPCondition: conditions.TrueCondition(controlplanev1.EtcdClusterHealthyCondition),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
			},
		},
		{
			name: "Eternal etcd should set a condition at KCP level",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							Etcd: bootstrapv1.Etcd{
								External: &bootstrapv1.ExternalEtcd{},
							},
						},
					},
				},
			},
			expectedKCPCondition: conditions.TrueCondition(controlplanev1.EtcdClusterHealthyCondition),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.kcp == nil {
				tt.kcp = &controlplanev1.KubeadmControlPlane{}
			}
			w := &Workload{
				Client:              tt.injectClient,
				etcdClientGenerator: tt.injectEtcdClientGenerator,
			}
			controlPane := &ControlPlane{
				KCP:      tt.kcp,
				Machines: collections.FromMachines(tt.machines...),
			}
			w.UpdateEtcdConditions(ctx, controlPane)

			if tt.expectedKCPCondition != nil {
				g.Expect(*conditions.Get(tt.kcp, controlplanev1.EtcdClusterHealthyCondition)).To(conditions.MatchCondition(*tt.expectedKCPCondition))
			}
			for _, m := range tt.machines {
				g.Expect(tt.expectedMachineConditions).To(HaveKey(m.Name))
				g.Expect(m.GetConditions()).To(conditions.MatchConditions(tt.expectedMachineConditions[m.Name]), "unexpected conditions for machine %s", m.Name)
			}
		})
	}
}

func TestUpdateStaticPodConditions(t *testing.T) {
	n1APIServerPodName := staticPodName("kube-apiserver", "n1")
	n1APIServerPodkey := client.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      n1APIServerPodName,
	}.String()
	n1ControllerManagerPodName := staticPodName("kube-controller-manager", "n1")
	n1ControllerManagerPodNKey := client.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      n1ControllerManagerPodName,
	}.String()
	n1SchedulerPodName := staticPodName("kube-scheduler", "n1")
	n1SchedulerPodKey := client.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      n1SchedulerPodName,
	}.String()
	n1EtcdPodName := staticPodName("etcd", "n1")
	n1EtcdPodKey := client.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      n1EtcdPodName,
	}.String()
	tests := []struct {
		name                      string
		kcp                       *controlplanev1.KubeadmControlPlane
		machines                  []*clusterv1.Machine
		injectClient              client.Client // This test is injecting a fake client because it is required to create nodes with a controlled Status or to fail with a specific error.
		expectedKCPCondition      *clusterv1.Condition
		expectedMachineConditions map[string]clusterv1.Conditions
	}{
		{
			name: "if list nodes return an error, it should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			injectClient: &fakeClient{
				listErr: errors.New("failed to list nodes"),
			},
			expectedKCPCondition: conditions.UnknownCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to list nodes which are hosting control plane components: failed to list nodes"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the node which is hosting this component: failed to list nodes"),
					*conditions.UnknownCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the node which is hosting this component: failed to list nodes"),
					*conditions.UnknownCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the node which is hosting this component: failed to list nodes"),
					*conditions.UnknownCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the node which is hosting this component: failed to list nodes"),
				},
			},
		},
		{
			name: "If there are provisioning machines, a node without machine should be ignored",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"), // without NodeRef (provisioning)
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedKCPCondition: nil,
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
		},
		{
			name:     "If there are no provisioning machines, a node without machine should be reported as False condition at KCP level",
			machines: []*clusterv1.Machine{},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnhealthyReason, clusterv1.ConditionSeverityError, "Control plane node %s does not have a corresponding machine", "n1"),
		},
		{
			name: "A node with unreachable taint should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1", withUnreachableTaint())},
				},
			},
			expectedKCPCondition: conditions.UnknownCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnknownReason, "Following machines are reporting unknown control plane status: m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
					*conditions.UnknownCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
					*conditions.UnknownCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
					*conditions.UnknownCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
				},
			},
		},
		{
			name: "A provisioning machine without node should be ignored",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"), // without NodeRef (provisioning)
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{},
			},
			expectedKCPCondition: nil,
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
		},
		{
			name: "A provisioned machine without node should report all the conditions as false",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting control plane errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node"),
					*conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node"),
					*conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node"),
					*conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node"),
				},
			},
		},
		{
			name: "Should surface control plane components errors",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
				get: map[string]interface{}{
					n1APIServerPodkey: fakePod(n1APIServerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					n1ControllerManagerPodNKey: fakePod(n1ControllerManagerPodName,
						withPhase(corev1.PodPending),
						withCondition(corev1.PodScheduled, corev1.ConditionFalse),
					),
					n1SchedulerPodKey: fakePod(n1SchedulerPodName,
						withPhase(corev1.PodFailed),
					),
					n1EtcdPodKey: fakePod(n1EtcdPodName,
						withPhase(corev1.PodSucceeded),
					),
				},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnhealthyReason, clusterv1.ConditionSeverityError, "Following machines are reporting control plane errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
					*conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting to be scheduled"),
					*conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
					*conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
				},
			},
		},
		{
			name: "Should surface control plane components health",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
				get: map[string]interface{}{
					n1APIServerPodkey: fakePod(n1APIServerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					n1ControllerManagerPodNKey: fakePod(n1ControllerManagerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					n1SchedulerPodKey: fakePod(n1SchedulerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					n1EtcdPodKey: fakePod(n1EtcdPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
				},
			},
			expectedKCPCondition: conditions.TrueCondition(controlplanev1.ControlPlaneComponentsHealthyCondition),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
					*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
					*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
					*conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyCondition),
				},
			},
		},
		{
			name: "Should surface control plane components health with eternal etcd",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
							Etcd: bootstrapv1.Etcd{
								External: &bootstrapv1.ExternalEtcd{},
							},
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
				get: map[string]interface{}{
					n1APIServerPodkey: fakePod(n1APIServerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					n1ControllerManagerPodNKey: fakePod(n1ControllerManagerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					n1SchedulerPodKey: fakePod(n1SchedulerPodName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
					// no static pod for etcd
				},
			},
			expectedKCPCondition: conditions.TrueCondition(controlplanev1.ControlPlaneComponentsHealthyCondition),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
					*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
					*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
					// no condition for etcd Pod
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.kcp == nil {
				tt.kcp = &controlplanev1.KubeadmControlPlane{}
			}
			w := &Workload{
				Client: tt.injectClient,
			}
			controlPane := &ControlPlane{
				KCP:      tt.kcp,
				Machines: collections.FromMachines(tt.machines...),
			}
			w.UpdateStaticPodConditions(ctx, controlPane)

			if tt.expectedKCPCondition != nil {
				g.Expect(*conditions.Get(tt.kcp, controlplanev1.ControlPlaneComponentsHealthyCondition)).To(conditions.MatchCondition(*tt.expectedKCPCondition))
			}
			for _, m := range tt.machines {
				g.Expect(tt.expectedMachineConditions).To(HaveKey(m.Name))
				g.Expect(m.GetConditions()).To(conditions.MatchConditions(tt.expectedMachineConditions[m.Name]))
			}
		})
	}
}

func TestUpdateStaticPodCondition(t *testing.T) {
	machine := &clusterv1.Machine{}
	nodeName := "node"
	component := "kube-component"
	condition := clusterv1.ConditionType("kubeComponentHealthy")
	podName := staticPodName(component, nodeName)
	podkey := client.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      podName,
	}.String()

	tests := []struct {
		name              string
		injectClient      client.Client // This test is injecting a fake client because it is required to create pods with a controlled Status or to fail with a specific error.
		node              *corev1.Node
		expectedCondition clusterv1.Condition
	}{
		{
			name:              "if node Ready is unknown, assume pod status is stale",
			node:              fakeNode(nodeName, withReadyCondition(corev1.ConditionUnknown)),
			expectedCondition: *conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedReason, "Node Ready condition is unknown, pod data might be stale"),
		},
		{
			name: "if gets pod return a NotFound error should report PodCondition=False, PodMissing",
			injectClient: &fakeClient{
				getErr: apierrors.NewNotFound(schema.ParseGroupResource("Pod"), component),
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodMissingReason, clusterv1.ConditionSeverityError, "Pod kube-component-node is missing"),
		},
		{
			name: "if gets pod return a generic error should report PodCondition=Unknown, PodInspectionFailed",
			injectClient: &fakeClient{
				getErr: errors.New("get failure"),
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedReason, "Failed to get pod status"),
		},
		{
			name: "pending pod not yet scheduled should report PodCondition=False, PodProvisioning",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodPending),
						withCondition(corev1.PodScheduled, corev1.ConditionFalse),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting to be scheduled"),
		},
		{
			name: "pending pod running init containers should report PodCondition=False, PodProvisioning",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodPending),
						withCondition(corev1.PodScheduled, corev1.ConditionTrue),
						withCondition(corev1.PodInitialized, corev1.ConditionFalse),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Running init containers"),
		},
		{
			name: "pending pod with PodScheduled and PodInitialized report PodCondition=False, PodProvisioning",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodPending),
						withCondition(corev1.PodScheduled, corev1.ConditionTrue),
						withCondition(corev1.PodInitialized, corev1.ConditionTrue),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, ""),
		},
		{
			name: "running pod with podReady should report PodCondition=true",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodRunning),
						withCondition(corev1.PodReady, corev1.ConditionTrue),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.TrueCondition(condition),
		},
		{
			name: "running pod with ContainerStatus Waiting should report PodCondition=False, PodProvisioning",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodRunning),
						withContainerStatus(corev1.ContainerStatus{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "Waiting something"},
							},
						}),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting something"),
		},
		{
			name: "running pod with ContainerStatus Waiting but with exit code != 0 should report PodCondition=False, PodFailed",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodRunning),
						withContainerStatus(corev1.ContainerStatus{
							State: corev1.ContainerState{
								Waiting: &corev1.ContainerStateWaiting{Reason: "Waiting something"},
							},
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 1,
								},
							},
						}),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Waiting something"),
		},
		{
			name: "running pod with ContainerStatus Terminated should report PodCondition=False, PodFailed",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodRunning),
						withContainerStatus(corev1.ContainerStatus{
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{Reason: "Something failed"},
							},
						}),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Something failed"),
		},
		{
			name: "running pod without podReady and without Container status messages should report PodCondition=False, PodProvisioning",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodRunning),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting for startup or readiness probes"),
		},
		{
			name: "failed pod should report PodCondition=False, PodFailed",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodFailed),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
		},
		{
			name: "succeeded pod should report PodCondition=False, PodFailed",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodSucceeded),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
		},
		{
			name: "pod in unknown phase should report PodCondition=Unknown, PodInspectionFailed",
			injectClient: &fakeClient{
				get: map[string]interface{}{
					podkey: fakePod(podName,
						withPhase(corev1.PodUnknown),
					),
				},
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedReason, "Pod is reporting unknown status"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			w := &Workload{
				Client: tt.injectClient,
			}
			w.updateStaticPodCondition(ctx, machine, *tt.node, component, condition)

			g.Expect(*conditions.Get(machine, condition)).To(conditions.MatchCondition(tt.expectedCondition))
		})
	}
}

type fakeNodeOption func(*corev1.Node)

func fakeNode(name string, options ...fakeNodeOption) *corev1.Node {
	p := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

func withUnreachableTaint() fakeNodeOption {
	return func(node *corev1.Node) {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    corev1.TaintNodeUnreachable,
			Effect: corev1.TaintEffectNoExecute,
		})
	}
}

func withReadyCondition(status corev1.ConditionStatus) fakeNodeOption {
	return func(node *corev1.Node) {
		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
			Type:   corev1.NodeReady,
			Status: status,
		})
	}
}

type fakeMachineOption func(*clusterv1.Machine)

func fakeMachine(name string, options ...fakeMachineOption) *clusterv1.Machine {
	p := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

func withNodeRef(ref string) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		machine.Status.NodeRef = &corev1.ObjectReference{
			Kind: "Node",
			Name: ref,
		}
	}
}

type fakePodOption func(*corev1.Pod)

func fakePod(name string, options ...fakePodOption) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceSystem,
		},
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}

func withPhase(phase corev1.PodPhase) fakePodOption {
	return func(pod *corev1.Pod) {
		pod.Status.Phase = phase
	}
}

func withContainerStatus(status corev1.ContainerStatus) fakePodOption {
	return func(pod *corev1.Pod) {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, status)
	}
}

func withCondition(condition corev1.PodConditionType, status corev1.ConditionStatus) fakePodOption {
	return func(pod *corev1.Pod) {
		c := corev1.PodCondition{
			Type:   condition,
			Status: status,
		}
		pod.Status.Conditions = append(pod.Status.Conditions, c)
	}
}
