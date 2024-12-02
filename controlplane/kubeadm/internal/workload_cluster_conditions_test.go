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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	fake2 "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

func TestUpdateEtcdConditions(t *testing.T) {
	var callCount int
	tests := []struct {
		name                      string
		kcp                       *controlplanev1.KubeadmControlPlane
		machines                  []*clusterv1.Machine
		injectClient              client.Client // This test is injecting a fake client because it is required to create nodes with a controlled Status or to fail with a specific error.
		injectEtcdClientGenerator etcdClientFor // This test is injecting a fake etcdClientGenerator because it is required to nodes with a controlled Status or to fail with a specific error.
		expectedRetry             bool
	}{
		{
			name: "retry retryable errors when KCP is not stable (e.g. scaling up)",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: ptr.To(int32(3)),
				},
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(_ []string) (*etcd.Client, error) {
					callCount++
					return nil, errors.New("fake error")
				},
			},
			expectedRetry: true,
		},
		{
			name: "do not retry retryable errors when KCP is stable",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: ptr.To(int32(1)),
				},
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(_ []string) (*etcd.Client, error) {
					callCount++
					return nil, errors.New("fake error")
				},
			},
			expectedRetry: false,
		},
		{
			name: "do not retry for other errors when KCP is scaling up",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Replicas: ptr.To(int32(3)),
				},
			},
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{
						*fakeNode("n1"),
					},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesClientFunc: func(_ []string) (*etcd.Client, error) {
					callCount++
					return &etcd.Client{
						EtcdClient: &fake2.FakeEtcdClient{
							EtcdEndpoints: []string{},
							MemberListResponse: &clientv3.MemberListResponse{
								Header: &pb.ResponseHeader{
									ClusterId: uint64(1),
								},
								Members: []*pb.Member{
									{Name: "n1", ID: uint64(1)},
								},
							},
							AlarmResponse: &clientv3.AlarmResponse{
								Alarms: []*pb.AlarmMember{},
							},
						},
					}, nil
				},
			},
			expectedRetry: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			w := &Workload{
				Client:              tt.injectClient,
				etcdClientGenerator: tt.injectEtcdClientGenerator,
			}
			controlPane := &ControlPlane{
				KCP:      tt.kcp,
				Machines: collections.FromMachines(tt.machines...),
			}

			callCount = 0
			w.UpdateEtcdConditions(ctx, controlPane)
			if tt.expectedRetry {
				// Note we keep the code implementing retry support so we can easily re-activate it if we need to.
				//	g.Expect(callCount).To(Equal(3))
				// } else {
				g.Expect(callCount).To(Equal(1))
			}
		})
	}
}

func TestUpdateManagedEtcdConditions(t *testing.T) {
	tests := []struct {
		name                                      string
		kcp                                       *controlplanev1.KubeadmControlPlane
		machines                                  []*clusterv1.Machine
		injectClient                              client.Client // This test is injecting a fake client because it is required to create nodes with a controlled Status or to fail with a specific error.
		injectEtcdClientGenerator                 etcdClientFor // This test is injecting a fake etcdClientGenerator because it is required to nodes with a controlled Status or to fail with a specific error.
		expectedRetryableError                    bool
		expectedKCPCondition                      *clusterv1.Condition
		expectedKCPV1Beta2Condition               *metav1.Condition
		expectedMachineConditions                 map[string]clusterv1.Conditions
		expectedMachineV1Beta2Conditions          map[string][]metav1.Condition
		expectedEtcdMembers                       []string
		expectedEtcdMembersAgreeOnMemberList      bool
		expectedEtcdMembersAgreeOnClusterID       bool
		expectedEtcdMembersAndMachinesAreMatching bool
	}{
		{
			name: "if list nodes return an error should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			injectClient: &fakeClient{
				listErr: errors.New("failed to list Nodes"),
			},
			expectedRetryableError: false,
			expectedKCPCondition:   conditions.UnknownCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterInspectionFailedReason, "Failed to list Nodes which are hosting the etcd members"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to get the Node which is hosting the etcd member"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedV1Beta2Reason,
				Message: "Failed to get Nodes hosting the etcd cluster",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason, Message: "Failed to get the Node hosting the etcd member"},
				},
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // without reading nodes, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // without reading nodes, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // without reading nodes, we can not make assumptions.
		},
		{
			name: "If there are provisioning machines, a node without machine should be ignored in v1beta1, reported in v1beta2 (without providerID)",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1")), // without NodeRef (provisioning)
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedRetryableError: false,
			expectedKCPCondition:   nil,
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID n1 to exist",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
				},
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // without reading members, we can not make assumptions.
		},
		{
			name: "If there are provisioning machines, a node without machine should be ignored in v1beta1, reported in v1beta2 (with providerID)",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("dummy-provider-id")), // without NodeRef (provisioning)
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedRetryableError: false,
			expectedKCPCondition:   nil,
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID dummy-provider-id to exist",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
				},
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // without reading members, we can not make assumptions.
		},
		{
			name:     "If there are no provisioning machines, a node without machine should be reported as False condition at KCP level",
			machines: []*clusterv1.Machine{},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedRetryableError: false,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Control plane Node %s does not have a corresponding Machine", "n1"),
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
				Message: "* Control plane Node n1 does not have a corresponding Machine",
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // without reading members, we can not make assumptions.
		},
		{
			name: "failure creating the etcd client should report unknown condition",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			injectEtcdClientGenerator: &fakeEtcdClientGenerator{
				forNodesErr: errors.New("failed to get client for node"),
			},
			expectedRetryableError: true,
			expectedKCPCondition:   conditions.UnknownCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnknownReason, "Following Machines are reporting unknown etcd member status: m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to connect to the etcd Pod on the %s Node: failed to get client for node", "n1"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Failed to connect to the etcd Pod on the n1 Node: failed to get client for node",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason, Message: "Failed to connect to the etcd Pod on the n1 Node: failed to get client for node"},
				},
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // failure in reading members, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // failure in reading members, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // failure in reading members, we can not make assumptions.
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
			expectedRetryableError: true,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member status reports errors: %s", "some errors"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Etcd reports errors: some errors",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason, Message: "Etcd reports errors: some errors"},
				},
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // without reading members, we can not make assumptions.
		},
		{
			name: "failure listing members should report false condition in v1beta1, unknown in v1beta2",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
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
			expectedRetryableError: true,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Failed to get answer from the etcd member on the %s Node", "n1"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Failed to get answer from the etcd member on the n1 Node: failed to get list of members for etcd cluster: failed to list members",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason, Message: "Failed to get answer from the etcd member on the n1 Node: failed to get list of members for etcd cluster: failed to list members"},
				},
			},
			expectedEtcdMembersAgreeOnMemberList:      false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAgreeOnClusterID:       false, // without reading members, we can not make assumptions.
			expectedEtcdMembersAndMachinesAreMatching: false, // without reading members, we can not make assumptions.
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
			expectedRetryableError: false,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports alarms: %s", "NOSPACE"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Etcd reports alarms: NOSPACE",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason, Message: "Etcd reports alarms: NOSPACE"},
				},
			},
			expectedEtcdMembers:                       []string{"n1"},
			expectedEtcdMembersAgreeOnMemberList:      true,
			expectedEtcdMembersAgreeOnClusterID:       true,
			expectedEtcdMembersAndMachinesAreMatching: true,
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
			expectedRetryableError: false,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m2"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member has cluster ID %d, but all previously seen etcd members have cluster ID %d", uint64(2), uint64(1)),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
				Message: "* Machine m2:\n" +
					"  * EtcdMemberHealthy: Etcd member has cluster ID 2, but all previously seen etcd members have cluster ID 1",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason, Message: ""},
				},
				"m2": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason, Message: "Etcd member has cluster ID 2, but all previously seen etcd members have cluster ID 1"},
				},
			},
			expectedEtcdMembers:                       []string{"n1", "n2"},
			expectedEtcdMembersAgreeOnMemberList:      true,
			expectedEtcdMembersAgreeOnClusterID:       false,
			expectedEtcdMembersAndMachinesAreMatching: false,
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
			expectedRetryableError: true,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m2"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports the cluster is composed by members [n2 n3], but all previously seen etcd members are reporting [n1 n2]"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
				Message: "* Machine m2:\n" +
					"  * EtcdMemberHealthy: The etcd member hosted on this Machine reports the cluster is composed by [n2 n3], but all previously seen etcd members are reporting [n1 n2]",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason, Message: ""},
				},
				"m2": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason, Message: "The etcd member hosted on this Machine reports the cluster is composed by [n2 n3], but all previously seen etcd members are reporting [n1 n2]"},
				},
			},
			expectedEtcdMembers:                       []string{"n1", "n2"},
			expectedEtcdMembersAgreeOnMemberList:      false,
			expectedEtcdMembersAgreeOnClusterID:       true,
			expectedEtcdMembersAndMachinesAreMatching: false,
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
			expectedRetryableError: true,
			expectedKCPCondition:   conditions.FalseCondition(controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m2"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Missing etcd member"),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
				Message: "* Machine m2:\n" +
					"  * EtcdMemberHealthy: Etcd doesn't have an etcd member for Node n2",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason, Message: ""},
				},
				"m2": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason, Message: "Etcd doesn't have an etcd member for Node n2"},
				},
			},
			expectedEtcdMembers:                       []string{"n1"},
			expectedEtcdMembersAgreeOnMemberList:      true,
			expectedEtcdMembersAgreeOnClusterID:       true,
			expectedEtcdMembersAndMachinesAreMatching: false,
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
			expectedRetryableError: false,
			expectedKCPCondition:   conditions.TrueCondition(controlplanev1.EtcdClusterHealthyCondition),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
				"m2": {
					*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
				},
			},
			expectedKCPV1Beta2Condition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Reason,
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason, Message: ""},
				},
				"m2": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason, Message: ""},
				},
			},
			expectedEtcdMembers:                       []string{"n1", "n2"},
			expectedEtcdMembersAgreeOnMemberList:      true,
			expectedEtcdMembersAgreeOnClusterID:       true,
			expectedEtcdMembersAndMachinesAreMatching: true,
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

			retryableError := w.updateManagedEtcdConditions(ctx, controlPane)
			g.Expect(retryableError).To(Equal(tt.expectedRetryableError))

			if tt.expectedKCPCondition != nil {
				g.Expect(*conditions.Get(tt.kcp, controlplanev1.EtcdClusterHealthyCondition)).To(conditions.MatchCondition(*tt.expectedKCPCondition))
			}
			if tt.expectedKCPV1Beta2Condition != nil {
				g.Expect(*v1beta2conditions.Get(tt.kcp, controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition)).To(v1beta2conditions.MatchCondition(*tt.expectedKCPV1Beta2Condition, v1beta2conditions.IgnoreLastTransitionTime(true)))
			}

			for _, m := range tt.machines {
				g.Expect(tt.expectedMachineConditions).To(HaveKey(m.Name))
				g.Expect(m.GetConditions()).To(conditions.MatchConditions(tt.expectedMachineConditions[m.Name]), "unexpected conditions for Machine %s", m.Name)
				g.Expect(m.GetV1Beta2Conditions()).To(v1beta2conditions.MatchConditions(tt.expectedMachineV1Beta2Conditions[m.Name], v1beta2conditions.IgnoreLastTransitionTime(true)), "unexpected conditions for Machine %s", m.Name)
			}

			g.Expect(controlPane.EtcdMembersAgreeOnMemberList).To(Equal(tt.expectedEtcdMembersAgreeOnMemberList), "EtcdMembersAgreeOnMemberList does not match")
			g.Expect(controlPane.EtcdMembersAgreeOnClusterID).To(Equal(tt.expectedEtcdMembersAgreeOnClusterID), "EtcdMembersAgreeOnClusterID does not match")
			g.Expect(controlPane.EtcdMembersAndMachinesAreMatching).To(Equal(tt.expectedEtcdMembersAndMachinesAreMatching), "EtcdMembersAndMachinesAreMatching does not match")

			var membersNames []string
			for _, m := range controlPane.EtcdMembers {
				membersNames = append(membersNames, m.Name)
			}
			g.Expect(membersNames).To(Equal(tt.expectedEtcdMembers))
		})
	}
}

func TestUpdateExternalEtcdConditions(t *testing.T) {
	tests := []struct {
		name                        string
		kcp                         *controlplanev1.KubeadmControlPlane
		expectedKCPCondition        *clusterv1.Condition
		expectedKCPV1Beta2Condition *metav1.Condition
	}{
		{
			name: "External etcd should set a condition at KCP level for v1beta1, not for v1beta2",
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
			expectedKCPCondition:        conditions.TrueCondition(controlplanev1.EtcdClusterHealthyCondition),
			expectedKCPV1Beta2Condition: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.kcp == nil {
				tt.kcp = &controlplanev1.KubeadmControlPlane{}
			}
			w := &Workload{}
			controlPane := &ControlPlane{
				KCP: tt.kcp,
			}

			w.updateExternalEtcdConditions(ctx, controlPane)
			if tt.expectedKCPCondition != nil {
				g.Expect(*conditions.Get(tt.kcp, controlplanev1.EtcdClusterHealthyCondition)).To(conditions.MatchCondition(*tt.expectedKCPCondition))
			}
			if tt.expectedKCPV1Beta2Condition != nil {
				g.Expect(*v1beta2conditions.Get(tt.kcp, controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition)).To(v1beta2conditions.MatchCondition(*tt.expectedKCPV1Beta2Condition, v1beta2conditions.IgnoreLastTransitionTime(true)))
			}
		})
	}
}

func TestUpdateStaticPodConditions(t *testing.T) {
	n1APIServerPodName := staticPodName("kube-apiserver", "n1")
	n1APIServerPodKey := client.ObjectKey{
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
		name                             string
		kcp                              *controlplanev1.KubeadmControlPlane
		machines                         []*clusterv1.Machine
		injectClient                     client.Client // This test is injecting a fake client because it is required to create nodes with a controlled Status or to fail with a specific error.
		expectedKCPCondition             *clusterv1.Condition
		expectedKCPV1Beta2Condition      metav1.Condition
		expectedMachineV1Beta2Conditions map[string][]metav1.Condition
		expectedMachineConditions        map[string]clusterv1.Conditions
	}{
		{
			name: "if list nodes return an error, it should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			injectClient: &fakeClient{
				listErr: errors.New("failed to list Nodes"),
			},
			expectedKCPCondition: conditions.UnknownCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to list Nodes which are hosting control plane components: failed to list Nodes"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
					*conditions.UnknownCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
					*conditions.UnknownCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
					*conditions.UnknownCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
				},
			},
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedV1Beta2Reason,
				Message: "Failed to get Nodes hosting control plane components: failed to list Nodes",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
				},
			},
		},
		{
			name: "If there are provisioning machines, a node without machine should be ignored in v1beta1, reported in v1beta2 (without providerID)",
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
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Reason,
				Message: "",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
				},
			},
		},
		{
			name: "If there are provisioning machines, a node without machine should be ignored in v1beta1, reported in v1beta2 (with providerID)",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("dummy-provider-id")), // without NodeRef (provisioning)
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
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Waiting for a Node with spec.providerID dummy-provider-id to exist",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
				},
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
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnhealthyReason, clusterv1.ConditionSeverityError, "Control plane Node %s does not have a corresponding Machine", "n1"),
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsNotHealthyV1Beta2Reason,
				Message: "* Control plane Node n1 does not have a corresponding Machine",
			},
		},
		{
			name: "A node with unreachable taint should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1", withUnreachableTaint())},
				},
			},
			expectedKCPCondition: conditions.UnknownCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnknownReason, "Following Machines are reporting unknown control plane status: m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.UnknownCondition(controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
					*conditions.UnknownCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
					*conditions.UnknownCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
					*conditions.UnknownCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodInspectionFailedReason, "Node is unreachable"),
				},
			},
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 is unreachable",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 is unreachable"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 is unreachable"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 is unreachable"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 is unreachable"},
				},
			},
		},
		{
			name: "A provisioning machine without node should be ignored in v1beta1, should surface in v1beta2",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1")), // without NodeRef (provisioning)
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{},
			},
			expectedKCPCondition: nil,
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Waiting for a Node with spec.providerID n1 to exist",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
				},
			},
		},
		{
			name: "A provisioned machine without node should report all the conditions as false in v1beta1, unknown in v1beta2",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{},
			},
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting control plane errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing Node"),
					*conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing Node"),
					*conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing Node"),
					*conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing Node"),
				},
			},
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 does not exist",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 does not exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 does not exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 does not exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason, Message: "Node n1 does not exist"},
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
					n1APIServerPodKey: fakePod(n1APIServerPodName,
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
			expectedKCPCondition: conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsUnhealthyReason, clusterv1.ConditionSeverityError, "Following Machines are reporting control plane errors: %s", "m1"),
			expectedMachineConditions: map[string]clusterv1.Conditions{
				"m1": {
					*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
					*conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting to be scheduled"),
					*conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
					*conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
				},
			},
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsNotHealthyV1Beta2Reason,
				Message: "* Machine m1:\n" +
					"  * ControllerManagerPodHealthy: Waiting to be scheduled\n" +
					"  * SchedulerPodHealthy: All the containers have been terminated\n" +
					"  * EtcdPodHealthy: All the containers have been terminated",
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason, Message: "Waiting to be scheduled"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason, Message: "All the containers have been terminated"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason, Message: "All the containers have been terminated"},
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
					n1APIServerPodKey: fakePod(n1APIServerPodName,
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
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Reason,
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
				},
			},
		},
		{
			name: "Should surface control plane components health with external etcd",
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
					n1APIServerPodKey: fakePod(n1APIServerPodName,
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
			expectedKCPV1Beta2Condition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Reason,
			},
			expectedMachineV1Beta2Conditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason, Message: ""},
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
			g.Expect(*v1beta2conditions.Get(tt.kcp, controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition)).To(v1beta2conditions.MatchCondition(tt.expectedKCPV1Beta2Condition, v1beta2conditions.IgnoreLastTransitionTime(true)))

			for _, m := range tt.machines {
				g.Expect(tt.expectedMachineConditions).To(HaveKey(m.Name))
				g.Expect(m.GetConditions()).To(conditions.MatchConditions(tt.expectedMachineConditions[m.Name]))
				g.Expect(m.GetV1Beta2Conditions()).To(v1beta2conditions.MatchConditions(tt.expectedMachineV1Beta2Conditions[m.Name], v1beta2conditions.IgnoreLastTransitionTime(true)))
			}
		})
	}
}

func TestUpdateStaticPodCondition(t *testing.T) {
	machine := &clusterv1.Machine{}
	nodeName := "node"
	component := "kube-component"
	condition := clusterv1.ConditionType("kubeComponentHealthy")
	v1beta2Condition := "kubeComponentHealthy"
	podName := staticPodName(component, nodeName)
	podkey := client.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      podName,
	}.String()

	tests := []struct {
		name                     string
		injectClient             client.Client // This test is injecting a fake client because it is required to create pods with a controlled Status or to fail with a specific error.
		node                     *corev1.Node
		expectedCondition        clusterv1.Condition
		expectedV1Beta2Condition metav1.Condition
	}{
		{
			name:              "if node Ready is unknown, assume pod status is stale",
			node:              fakeNode(nodeName, withReadyCondition(corev1.ConditionUnknown)),
			expectedCondition: *conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedReason, "Node Ready condition is Unknown, Pod data might be stale"),
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
				Message: "Node Ready condition is Unknown, Pod data might be stale",
			},
		},
		{
			name: "if gets pod return a NotFound error should report PodCondition=False, PodMissing",
			injectClient: &fakeClient{
				getErr: apierrors.NewNotFound(schema.ParseGroupResource("Pod"), component),
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.FalseCondition(condition, controlplanev1.PodMissingReason, clusterv1.ConditionSeverityError, "Pod kube-component-node is missing"),
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodDoesNotExistV1Beta2Reason,
				Message: "Pod does not exist",
			},
		},
		{
			name: "if gets pod return a generic error should report PodCondition=Unknown, PodInspectionFailed",
			injectClient: &fakeClient{
				getErr: errors.New("get failure"),
			},
			node:              fakeNode(nodeName),
			expectedCondition: *conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedReason, "Failed to get Pod status"),
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
				Message: "Please check controller logs for errors",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "Waiting to be scheduled",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "Running init containers",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason,
				Message: "",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "Waiting something",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
				Message: "Waiting something",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
				Message: "Something failed",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "Waiting for startup or readiness probes",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
				Message: "All the containers have been terminated",
			},
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
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
				Message: "All the containers have been terminated",
			},
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
			expectedCondition: *conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedReason, "Pod is reporting Unknown status"),
			expectedV1Beta2Condition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
				Message: "Pod is reporting Unknown status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			w := &Workload{
				Client: tt.injectClient,
			}
			w.updateStaticPodCondition(ctx, machine, *tt.node, component, condition, v1beta2Condition)

			g.Expect(*conditions.Get(machine, condition)).To(conditions.MatchCondition(tt.expectedCondition))
			g.Expect(*v1beta2conditions.Get(machine, v1beta2Condition)).To(v1beta2conditions.MatchCondition(tt.expectedV1Beta2Condition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

type fakeNodeOption func(*corev1.Node)

func fakeNode(name string, options ...fakeNodeOption) *corev1.Node {
	p := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
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
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind: "GenericInfraMachine",
				Name: fmt.Sprintf("infra-%s", name),
			},
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

func withProviderID(providerID string) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		machine.Spec.ProviderID = ptr.To(providerID)
	}
}

func withMachineReadyCondition(status corev1.ConditionStatus, severity clusterv1.ConditionSeverity) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		machine.Status.Conditions = append(machine.Status.Conditions, clusterv1.Condition{
			Type:     clusterv1.MachinesReadyCondition,
			Status:   status,
			Severity: severity,
		})
	}
}

func withMachineReadyV1beta2Condition(status metav1.ConditionStatus) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		if machine.Status.V1Beta2 == nil {
			machine.Status.V1Beta2 = &clusterv1.MachineV1Beta2Status{}
		}
		machine.Status.V1Beta2.Conditions = append(machine.Status.V1Beta2.Conditions, metav1.Condition{
			Type:    clusterv1.MachineReadyV1Beta2Condition,
			Status:  status,
			Reason:  "SomeReason",
			Message: fmt.Sprintf("ready condition is %s", status),
		})
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

func TestAggregateConditionsFromMachinesToKCP(t *testing.T) {
	conditionType := controlplanev1.ControlPlaneComponentsHealthyCondition
	unhealthyReason := "unhealthy reason"
	unknownReason := "unknown reason"
	note := "some notes"

	tests := []struct {
		name              string
		machines          []*clusterv1.Machine
		kcpErrors         []string
		expectedCondition clusterv1.Condition
	}{
		{
			name: "kcp machines with errors",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionFalse, clusterv1.ConditionSeverityError)),
			},
			expectedCondition: *conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityError, fmt.Sprintf("Following Machines are reporting %s errors: %s", note, "m1")),
		},
		{
			name: "input kcp errors",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionTrue, clusterv1.ConditionSeverityNone)),
			},
			kcpErrors:         []string{"something error"},
			expectedCondition: *conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityError, "something error"),
		},
		{
			name: "kcp machines with warnings",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionFalse, clusterv1.ConditionSeverityWarning)),
			},
			expectedCondition: *conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityWarning, fmt.Sprintf("Following Machines are reporting %s warnings: %s", note, "m1")),
		},
		{
			name: "kcp machines with info",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionFalse, clusterv1.ConditionSeverityInfo)),
			},
			expectedCondition: *conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityInfo, fmt.Sprintf("Following Machines are reporting %s info: %s", note, "m1")),
		},
		{
			name: "kcp machines with true",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionTrue, clusterv1.ConditionSeverityNone)),
			},
			expectedCondition: *conditions.TrueCondition(conditionType),
		},
		{
			name: "kcp machines with unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionUnknown, clusterv1.ConditionSeverityNone)),
			},
			expectedCondition: *conditions.UnknownCondition(conditionType, unknownReason, fmt.Sprintf("Following Machines are reporting unknown %s status: %s", note, "m1")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			input := aggregateConditionsFromMachinesToKCPInput{
				controlPlane: &ControlPlane{
					KCP:      &controlplanev1.KubeadmControlPlane{},
					Machines: collections.FromMachines(tt.machines...),
				},
				machineConditions: []clusterv1.ConditionType{clusterv1.MachinesReadyCondition},
				kcpErrors:         tt.kcpErrors,
				condition:         conditionType,
				unhealthyReason:   unhealthyReason,
				unknownReason:     unknownReason,
				note:              note,
			}
			aggregateConditionsFromMachinesToKCP(input)

			g.Expect(*conditions.Get(input.controlPlane.KCP, conditionType)).To(conditions.MatchCondition(tt.expectedCondition))
		})
	}
}

func TestAggregateV1Beta2ConditionsFromMachinesToKCP(t *testing.T) {
	conditionType := controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition
	trueReason := "true reason"
	unknownReason := "unknown reason"
	falseReason := "false reason"
	note := "something"

	tests := []struct {
		name              string
		machines          []*clusterv1.Machine
		kcpErrors         []string
		expectedCondition metav1.Condition
	}{
		{
			name: "kcp machines with errors",
			machines: []*clusterv1.Machine{
				fakeMachine("m2", withMachineReadyV1beta2Condition(metav1.ConditionFalse)), // machines are intentionally not ordered
				fakeMachine("m4", withMachineReadyV1beta2Condition(metav1.ConditionUnknown), withProviderID("m4")),
				fakeMachine("m1", withMachineReadyV1beta2Condition(metav1.ConditionFalse)),
				fakeMachine("m3", withMachineReadyV1beta2Condition(metav1.ConditionTrue)),
			},
			expectedCondition: metav1.Condition{
				Type:   conditionType,
				Status: metav1.ConditionFalse,
				Reason: falseReason,
				Message: "* Machines m1, m2:\n" +
					"  * Ready: ready condition is False\n" +
					"* Machine m4:\n" +
					"  * Ready: ready condition is Unknown",
			},
		},
		{
			name: "kcp errors",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyV1beta2Condition(metav1.ConditionTrue)),
			},
			kcpErrors: []string{"something error"},
			expectedCondition: metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionFalse,
				Reason:  falseReason,
				Message: "* something error",
			},
		},
		{
			name: "kcp machines with unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m3", withMachineReadyV1beta2Condition(metav1.ConditionUnknown), withProviderID("m1")), // machines are intentionally not ordered
				fakeMachine("m1", withMachineReadyV1beta2Condition(metav1.ConditionUnknown), withProviderID("m1")),
				fakeMachine("m2", withMachineReadyV1beta2Condition(metav1.ConditionTrue)),
			},
			expectedCondition: metav1.Condition{
				Type:   conditionType,
				Status: metav1.ConditionUnknown,
				Reason: unknownReason,
				Message: "* Machines m1, m3:\n" +
					"  * Ready: ready condition is Unknown",
			},
		},
		{
			name: "kcp machines with unknown but without provided ID are ignored",
			machines: []*clusterv1.Machine{
				fakeMachine("m3", withMachineReadyV1beta2Condition(metav1.ConditionUnknown)), // machines are intentionally not ordered
				fakeMachine("m1", withMachineReadyV1beta2Condition(metav1.ConditionUnknown)),
				fakeMachine("m2", withMachineReadyV1beta2Condition(metav1.ConditionTrue)),
			},
			expectedCondition: metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionTrue,
				Reason:  trueReason,
				Message: "",
			},
		},
		{
			name: "kcp machines with true",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyV1beta2Condition(metav1.ConditionTrue)),
				fakeMachine("m2", withMachineReadyV1beta2Condition(metav1.ConditionTrue)),
			},
			expectedCondition: metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionTrue,
				Reason:  trueReason,
				Message: "",
			},
		},
		{
			name:     "kcp without machines",
			machines: []*clusterv1.Machine{},
			expectedCondition: metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  unknownReason,
				Message: "No Machines reporting something status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			input := aggregateV1Beta2ConditionsFromMachinesToKCPInput{
				controlPlane: &ControlPlane{
					KCP:      &controlplanev1.KubeadmControlPlane{},
					Machines: collections.FromMachines(tt.machines...),
				},
				machineConditions: []string{clusterv1.MachineReadyV1Beta2Condition},
				kcpErrors:         tt.kcpErrors,
				condition:         conditionType,
				trueReason:        trueReason,
				unknownReason:     unknownReason,
				falseReason:       falseReason,
				note:              note,
			}
			aggregateV1Beta2ConditionsFromMachinesToKCP(input)

			g.Expect(*v1beta2conditions.Get(input.controlPlane.KCP, conditionType)).To(v1beta2conditions.MatchCondition(tt.expectedCondition, v1beta2conditions.IgnoreLastTransitionTime(true)))
		})
	}
}

func TestCompareMachinesAndMembers(t *testing.T) {
	tests := []struct {
		name                                string
		controlPlane                        *ControlPlane
		nodes                               *corev1.NodeList
		members                             []*etcd.Member
		expectMembersAndMachinesAreMatching bool
		expectKCPErrors                     []string
	}{
		{
			name: "true if the list of members is empty and there are no provisioned machines",
			controlPlane: &ControlPlane{
				KCP:      &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(fakeMachine("m1")),
			},
			members:                             nil,
			nodes:                               nil,
			expectMembersAndMachinesAreMatching: true,
			expectKCPErrors:                     nil,
		},
		{
			name: "false if the list of members is empty and there are provisioned machines",
			controlPlane: &ControlPlane{
				KCP:      &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(fakeMachine("m1", withNodeRef("m1"))),
			},
			members:                             nil,
			nodes:                               nil,
			expectMembersAndMachinesAreMatching: false,
			expectKCPErrors:                     nil,
		},
		{
			name: "true if the list of members match machines",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					fakeMachine("m2", withNodeRef("m2")),
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
			},
			nodes:                               nil,
			expectMembersAndMachinesAreMatching: true,
			expectKCPErrors:                     nil,
		},
		{
			name: "true if there is a machine without a member but at least a machine is still provisioning",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					fakeMachine("m2", withNodeRef("m2")),
					fakeMachine("m3"), // m3 is still provisioning
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
				// m3 is missing
			},
			nodes:                               nil,
			expectMembersAndMachinesAreMatching: true,
			expectKCPErrors:                     nil,
		},
		{
			name: "true if there is a machine without a member but node on this machine does not exist yet",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					fakeMachine("m2", withNodeRef("m2")),
					fakeMachine("m3", withNodeRef("m3")),
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
				// m3 is missing
			},
			nodes: &corev1.NodeList{Items: []corev1.Node{
				// m3 is missing
			}},
			expectMembersAndMachinesAreMatching: true,
			expectKCPErrors:                     nil,
		},
		{
			name: "true if there is a machine without a member but node on this machine has been just created",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					fakeMachine("m2", withNodeRef("m2")),
					fakeMachine("m3", withNodeRef("m3")),
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
				// m3 is missing
			},
			nodes: &corev1.NodeList{Items: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "m3", CreationTimestamp: metav1.Time{Time: time.Now().Add(-110 * time.Second)}}}, // m3 is just provisioned
			}},
			expectMembersAndMachinesAreMatching: true,
			expectKCPErrors:                     nil,
		},
		{
			name: "false if there is a machine without a member and node on this machine is old",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					fakeMachine("m2", withNodeRef("m2")),
					fakeMachine("m3", withNodeRef("m3")),
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
				// m3 is missing
			},
			nodes: &corev1.NodeList{Items: []corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "m3", CreationTimestamp: metav1.Time{Time: time.Now().Add(-10 * time.Minute)}}}, // m3 is old
			}},
			expectMembersAndMachinesAreMatching: false,
			expectKCPErrors:                     nil,
		},
		{
			name: "false if there is a member without a machine",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					// m2 is missing
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
			},
			nodes:                               nil,
			expectMembersAndMachinesAreMatching: false,
			expectKCPErrors:                     []string{"Etcd member m2 does not have a corresponding Machine"},
		},
		{
			name: "true if there is a member without a machine while a machine is still provisioning ",
			controlPlane: &ControlPlane{
				KCP: &controlplanev1.KubeadmControlPlane{},
				Machines: collections.FromMachines(
					fakeMachine("m1", withNodeRef("m1")),
					fakeMachine("m2"), // m2 still provisioning
				),
			},
			members: []*etcd.Member{
				{Name: "m1"},
				{Name: "m2"},
			},
			nodes:                               nil,
			expectMembersAndMachinesAreMatching: true,
			expectKCPErrors:                     []string{"Etcd member m2 does not have a corresponding Machine"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, gotErrors := compareMachinesAndMembers(tt.controlPlane, tt.nodes, tt.members)

			g.Expect(got).To(Equal(tt.expectMembersAndMachinesAreMatching))
			g.Expect(gotErrors).To(Equal(tt.expectKCPErrors))
		})
	}
}
