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
	"strings"
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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	fake2 "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
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
		expectedKCPV1Beta1Condition               *clusterv1.Condition
		expectedKCPCondition                      *metav1.Condition
		expectedMachineV1Beta1Conditions          map[string]clusterv1.Conditions
		expectedMachineConditions                 map[string][]metav1.Condition
		expectedEtcdMembers                       []string
		expectedEtcdMembersAndMachinesAreMatching bool
	}{
		{
			name: "if list nodes return an error should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			injectClient: &fakeClient{
				listErr: errors.New("something went wrong"),
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.UnknownCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterInspectionFailedV1Beta1Reason, "Failed to list Nodes which are hosting the etcd members"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, controlplanev1.EtcdMemberInspectionFailedV1Beta1Reason, "Failed to get the Node which is hosting the etcd member"),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedReason,
				Message: "Failed to get Nodes hosting the etcd cluster",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason, Message: "Failed to get the Node hosting the etcd member"},
				},
			},
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
			expectedKCPV1Beta1Condition: nil,
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID n1 to exist",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
				},
			},
			expectedEtcdMembersAndMachinesAreMatching: true, // there is a single provisioning machine, member list not yet available (too early to say members and machines do not match).
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
			expectedKCPV1Beta1Condition: nil,
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Waiting for a Node with spec.providerID dummy-provider-id to exist",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
				},
			},
			expectedEtcdMembersAndMachinesAreMatching: true,
		},

		{
			name:     "If there are no provisioning machines, a node without machine should be reported as False condition at KCP level",
			machines: []*clusterv1.Machine{},
			injectClient: &fakeClient{
				list: &corev1.NodeList{
					Items: []corev1.Node{*fakeNode("n1")},
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Control plane Node %s does not have a corresponding Machine", "n1"),
			expectedKCPCondition: &metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyReason,
				Message: "* Control plane Node n1 does not have a corresponding Machine",
			},
			expectedEtcdMembersAndMachinesAreMatching: true,
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
				forNodesErr: errors.New("something went wrong"),
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.UnknownCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterUnknownV1Beta1Reason, "Following Machines are reporting unknown etcd member status: m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, controlplanev1.EtcdMemberInspectionFailedV1Beta1Reason, "Failed to connect to etcd: something went wrong"),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Failed to connect to etcd: something went wrong",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason, Message: "Failed to connect to etcd: something went wrong"},
				},
			},
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
					Endpoint: "n1",
					Errors:   []string{"something went wrong"},
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, controlplanev1.EtcdMemberUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Etcd endpoint n1 reports errors: %s", "something went wrong"),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyReason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Etcd endpoint n1 reports errors: something went wrong",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason, Message: "Etcd endpoint n1 reports errors: something went wrong"},
				},
			},
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
						EtcdEndpoints:   []string{},
						MemberListError: errors.New("something went wrong"),
					},
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, controlplanev1.EtcdMemberUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Failed to get etcd members"),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Failed to get etcd members: something went wrong",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedReason, Message: "Failed to get etcd members: something went wrong"},
				},
			},
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
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, controlplanev1.EtcdMemberUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Etcd member reports alarms: %s", "NOSPACE"),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyReason,
				Message: "* Machine m1:\n" +
					"  * EtcdMemberHealthy: Etcd reports alarms: NOSPACE",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason, Message: "Etcd reports alarms: NOSPACE"},
				},
			},
			expectedEtcdMembers:                       []string{"n1"},
			expectedEtcdMembersAndMachinesAreMatching: true,
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
				forNodesClientFunc: func(nodeNames []string) (*etcd.Client, error) {
					var errs []error
					for _, n := range nodeNames {
						switch n {
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
							errs = append(errs, errors.New("no client for this node"))
						}
					}
					return nil, errors.Wrapf(kerrors.NewAggregate(errs), "could not establish a connection to etcd members hosted on %s", strings.Join(nodeNames, ","))
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition, controlplanev1.EtcdClusterUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting etcd member errors: %s", "m2"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition),
				},
				"m2": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition, controlplanev1.EtcdMemberUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing etcd member"),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyReason,
				Message: "* Machine m2:\n" +
					"  * EtcdMemberHealthy: Etcd doesn't have an etcd member for Node n2",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason, Message: ""},
				},
				"m2": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason, Message: "Etcd doesn't have an etcd member for Node n2"},
				},
			},
			expectedEtcdMembers:                       []string{"n1"},
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
				forNodesClientFunc: func(nodeNames []string) (*etcd.Client, error) {
					var errs []error
					for _, n := range nodeNames {
						switch n {
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
							errs = append(errs, errors.New("no client for this node"))
						}
					}
					return nil, errors.Wrapf(kerrors.NewAggregate(errs), "could not establish a connection to etcd members hosted on %s", strings.Join(nodeNames, ","))
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.TrueCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition),
				},
				"m2": {
					*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyV1Beta1Condition),
				},
			},
			expectedKCPCondition: &metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyReason,
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason, Message: ""},
				},
				"m2": {
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason, Message: ""},
				},
			},
			expectedEtcdMembers:                       []string{"n1", "n2"},
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

			w.updateManagedEtcdConditions(ctx, controlPane)

			if tt.expectedKCPV1Beta1Condition != nil {
				g.Expect(*v1beta1conditions.Get(tt.kcp, controlplanev1.EtcdClusterHealthyV1Beta1Condition)).To(v1beta1conditions.MatchCondition(*tt.expectedKCPV1Beta1Condition))
			}
			if tt.expectedKCPCondition != nil {
				g.Expect(*conditions.Get(tt.kcp, controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition)).To(conditions.MatchCondition(*tt.expectedKCPCondition, conditions.IgnoreLastTransitionTime(true)))
			}

			for _, m := range tt.machines {
				g.Expect(tt.expectedMachineV1Beta1Conditions).To(HaveKey(m.Name))
				g.Expect(m.GetV1Beta1Conditions()).To(v1beta1conditions.MatchConditions(tt.expectedMachineV1Beta1Conditions[m.Name]), "unexpected conditions for Machine %s", m.Name)
				g.Expect(m.GetConditions()).To(conditions.MatchConditions(tt.expectedMachineConditions[m.Name], conditions.IgnoreLastTransitionTime(true)), "unexpected conditions for Machine %s", m.Name)
			}

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
		expectedKCPV1Beta1Condition *clusterv1.Condition
		expectedKCPCondition        *metav1.Condition
	}{
		{
			name: "External etcd should set a condition at KCP level for v1beta1, not for v1beta2",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							Etcd: bootstrapv1.Etcd{
								External: bootstrapv1.ExternalEtcd{
									Endpoints: []string{"1.2.3.4"},
								},
							},
						},
					},
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.TrueCondition(controlplanev1.EtcdClusterHealthyV1Beta1Condition),
			expectedKCPCondition:        nil,
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
			if tt.expectedKCPV1Beta1Condition != nil {
				g.Expect(*v1beta1conditions.Get(tt.kcp, controlplanev1.EtcdClusterHealthyV1Beta1Condition)).To(v1beta1conditions.MatchCondition(*tt.expectedKCPV1Beta1Condition))
			}
			if tt.expectedKCPCondition != nil {
				g.Expect(*conditions.Get(tt.kcp, controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition)).To(conditions.MatchCondition(*tt.expectedKCPCondition, conditions.IgnoreLastTransitionTime(true)))
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
		kubeadmConfigs                   map[string]*bootstrapv1.KubeadmConfig
		injectClient                     client.Client // This test is injecting a fake client because it is required to create nodes with a controlled Status or to fail with a specific error.
		expectedKCPV1Beta1Condition      *clusterv1.Condition
		expectedKCPCondition             metav1.Condition
		expectedMachineConditions        map[string][]metav1.Condition
		expectedMachineV1Beta1Conditions map[string]clusterv1.Conditions
	}{
		{
			name: "if list nodes return an error, it should report all the conditions Unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1"),
			},
			injectClient: &fakeClient{
				listErr: errors.New("failed to list Nodes"),
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.UnknownCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsInspectionFailedV1Beta1Reason, "Failed to list Nodes which are hosting control plane components: failed to list Nodes"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Failed to get the Node which is hosting this component: failed to list Nodes"),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedReason,
				Message: "Failed to get Nodes hosting control plane components: failed to list Nodes",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Failed to get the Node hosting the Pod: failed to list Nodes"},
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
			expectedKCPV1Beta1Condition: nil,
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
				Message: "",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for GenericInfraMachine to report spec.providerID"},
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
			expectedKCPV1Beta1Condition: nil,
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Waiting for a Node with spec.providerID dummy-provider-id to exist",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID dummy-provider-id to exist"},
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
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Control plane Node %s does not have a corresponding Machine", "n1"),
			expectedKCPCondition: metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsNotHealthyReason,
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
			expectedKCPV1Beta1Condition: v1beta1conditions.UnknownCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsUnknownV1Beta1Reason, "Following Machines are reporting unknown control plane status: m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Node is unreachable"),
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Node is unreachable"),
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Node is unreachable"),
					*v1beta1conditions.UnknownCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Node is unreachable"),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 is unreachable",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 is unreachable"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 is unreachable"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 is unreachable"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 is unreachable"},
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
			expectedKCPV1Beta1Condition: nil,
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Waiting for a Node with spec.providerID n1 to exist",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Waiting for a Node with spec.providerID n1 to exist"},
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
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting control plane errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 does not exist",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not exist"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not exist"},
				},
			},
		},
		{
			name: "A provisioned machine with a node without the control plane label should report all the conditions as false in v1beta1, unknown in v1beta2",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{},
				get: map[string]interface{}{
					"/n1": &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "n1",
							Labels: map[string]string{},
						},
					},
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting control plane errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 does not have the node-role.kubernetes.io/control-plane label",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label"},
				},
			},
		},
		{
			name: "A provisioned machine with a node without the control plane label and taint should report all the conditions as false in v1beta1, unknown in v1beta2",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
			},
			kubeadmConfigs: map[string]*bootstrapv1.KubeadmConfig{
				"m1": {}, // A default kubeadm config requires the control plane taint
			},
			injectClient: &fakeClient{
				list: &corev1.NodeList{},
				get: map[string]interface{}{
					"/n1": &corev1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name:   "n1",
							Labels: map[string]string{},
						},
					},
				},
			},
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting control plane errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Missing Node"),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 does not have the node-role.kubernetes.io/control-plane label and the node-role.kubernetes.io/control-plane:NoSchedule taint",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label and the node-role.kubernetes.io/control-plane:NoSchedule taint"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label and the node-role.kubernetes.io/control-plane:NoSchedule taint"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label and the node-role.kubernetes.io/control-plane:NoSchedule taint"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane label and the node-role.kubernetes.io/control-plane:NoSchedule taint"},
				},
			},
		},
		{
			name: "Should surface control plane nodes without the default control plane taint",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withProviderID("n1"), withNodeRef("n1")),
			},
			kubeadmConfigs: map[string]*bootstrapv1.KubeadmConfig{
				"m1": {}, // A default kubeadm config requires the control plane taint
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
			expectedKCPV1Beta1Condition:      nil,
			expectedMachineV1Beta1Conditions: nil,
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionUnknown,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownReason,
				Message: "* Machine m1:\n" +
					"  * Control plane components: Node n1 does not have the node-role.kubernetes.io/control-plane:NoSchedule taint",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane:NoSchedule taint"},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane:NoSchedule taint"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane:NoSchedule taint"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionUnknown, Reason: controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason, Message: "Node n1 does not have the node-role.kubernetes.io/control-plane:NoSchedule taint"},
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
			expectedKCPV1Beta1Condition: v1beta1conditions.FalseCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition, controlplanev1.ControlPlaneComponentsUnhealthyV1Beta1Reason, clusterv1.ConditionSeverityError, "Following Machines are reporting control plane errors: %s", "m1"),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition, controlplanev1.PodProvisioningV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting to be scheduled"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
					*v1beta1conditions.FalseCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionFalse,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsNotHealthyReason,
				Message: "* Machine m1:\n" +
					"  * ControllerManagerPodHealthy: Waiting to be scheduled\n" +
					"  * SchedulerPodHealthy: All the containers have been terminated\n" +
					"  * EtcdPodHealthy: All the containers have been terminated",
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodProvisioningReason, Message: "Waiting to be scheduled"},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodFailedReason, Message: "All the containers have been terminated"},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionFalse, Reason: controlplanev1.KubeadmControlPlaneMachinePodFailedReason, Message: "All the containers have been terminated"},
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
			expectedKCPV1Beta1Condition: v1beta1conditions.TrueCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition),
					*v1beta1conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition),
					*v1beta1conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition),
					*v1beta1conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyV1Beta1Condition),
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
				},
			},
		},
		{
			name: "Should surface control plane components health with external etcd",
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
						ClusterConfiguration: bootstrapv1.ClusterConfiguration{
							Etcd: bootstrapv1.Etcd{
								External: bootstrapv1.ExternalEtcd{
									Endpoints: []string{"1.2.3.4"},
								},
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
			expectedKCPV1Beta1Condition: v1beta1conditions.TrueCondition(controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition),
			expectedMachineV1Beta1Conditions: map[string]clusterv1.Conditions{
				"m1": {
					*v1beta1conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyV1Beta1Condition),
					*v1beta1conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyV1Beta1Condition),
					*v1beta1conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyV1Beta1Condition),
					// no condition for etcd Pod
				},
			},
			expectedKCPCondition: metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyReason,
			},
			expectedMachineConditions: map[string][]metav1.Condition{
				"m1": {
					{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
					{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason, Message: ""},
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
				KCP:            tt.kcp,
				Machines:       collections.FromMachines(tt.machines...),
				KubeadmConfigs: tt.kubeadmConfigs,
			}
			w.UpdateStaticPodConditions(ctx, controlPane)

			if tt.expectedKCPV1Beta1Condition != nil {
				g.Expect(*v1beta1conditions.Get(tt.kcp, controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition)).To(v1beta1conditions.MatchCondition(*tt.expectedKCPV1Beta1Condition))
			}
			g.Expect(*conditions.Get(tt.kcp, controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition)).To(conditions.MatchCondition(tt.expectedKCPCondition, conditions.IgnoreLastTransitionTime(true)))

			for _, m := range tt.machines {
				if tt.expectedMachineV1Beta1Conditions != nil {
					g.Expect(tt.expectedMachineV1Beta1Conditions).To(HaveKey(m.Name))
					g.Expect(m.GetV1Beta1Conditions()).To(v1beta1conditions.MatchConditions(tt.expectedMachineV1Beta1Conditions[m.Name]))
				}
				g.Expect(m.GetConditions()).To(conditions.MatchConditions(tt.expectedMachineConditions[m.Name], conditions.IgnoreLastTransitionTime(true)))
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
		expectedV1Beta1Condition clusterv1.Condition
		expectedCondition        metav1.Condition
	}{
		{
			name:                     "if node Ready is unknown, assume pod status is stale",
			node:                     fakeNode(nodeName, withReadyCondition(corev1.ConditionUnknown)),
			expectedV1Beta1Condition: *v1beta1conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Node Ready condition is Unknown, Pod data might be stale"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
				Message: "Node Ready condition is Unknown, Pod data might be stale",
			},
		},
		{
			name: "if gets pod return a NotFound error should report PodCondition=False, PodMissing",
			injectClient: &fakeClient{
				getErr: apierrors.NewNotFound(schema.ParseGroupResource("Pod"), component),
			},
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodMissingV1Beta1Reason, clusterv1.ConditionSeverityError, "Pod kube-component-node is missing"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodDoesNotExistReason,
				Message: "Pod does not exist",
			},
		},
		{
			name: "if gets pod return a generic error should report PodCondition=Unknown, PodInspectionFailed",
			injectClient: &fakeClient{
				getErr: errors.New("get failure"),
			},
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Failed to get Pod status"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodProvisioningV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting to be scheduled"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodProvisioningV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Running init containers"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodProvisioningV1Beta1Reason, clusterv1.ConditionSeverityInfo, ""),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.TrueCondition(condition),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionTrue,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodRunningReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodProvisioningV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting something"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Waiting something"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "Something failed"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodProvisioningV1Beta1Reason, clusterv1.ConditionSeverityInfo, "Waiting for startup or readiness probes"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.FalseCondition(condition, controlplanev1.PodFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "All the containers have been terminated"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedReason,
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
			node:                     fakeNode(nodeName),
			expectedV1Beta1Condition: *v1beta1conditions.UnknownCondition(condition, controlplanev1.PodInspectionFailedV1Beta1Reason, "Pod is reporting Unknown status"),
			expectedCondition: metav1.Condition{
				Type:    v1beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedReason,
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

			g.Expect(*v1beta1conditions.Get(machine, condition)).To(v1beta1conditions.MatchCondition(tt.expectedV1Beta1Condition))
			g.Expect(*conditions.Get(machine, v1beta2Condition)).To(conditions.MatchCondition(tt.expectedCondition, conditions.IgnoreLastTransitionTime(true)))
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
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
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
		machine.Status.NodeRef = clusterv1.MachineNodeReference{
			Name: ref,
		}
	}
}

func withProviderID(providerID string) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		machine.Spec.ProviderID = providerID
	}
}

func withMachineReadyCondition(status corev1.ConditionStatus, severity clusterv1.ConditionSeverity) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		if machine.Status.Deprecated == nil {
			machine.Status.Deprecated = &clusterv1.MachineDeprecatedStatus{}
		}
		if machine.Status.Deprecated.V1Beta1 == nil {
			machine.Status.Deprecated.V1Beta1 = &clusterv1.MachineV1Beta1DeprecatedStatus{}
		}
		machine.Status.Deprecated.V1Beta1.Conditions = append(machine.Status.Deprecated.V1Beta1.Conditions, clusterv1.Condition{
			Type:     clusterv1.MachinesReadyV1Beta1Condition,
			Status:   status,
			Severity: severity,
		})
	}
}

func withMachineReadyV1beta2Condition(status metav1.ConditionStatus) fakeMachineOption {
	return func(machine *clusterv1.Machine) {
		machine.Status.Conditions = append(machine.Status.Conditions, metav1.Condition{
			Type:    clusterv1.MachineReadyCondition,
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

func TestAggregateV1Beta1ConditionsFromMachinesToKCP(t *testing.T) {
	conditionType := controlplanev1.ControlPlaneComponentsHealthyV1Beta1Condition
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
			expectedCondition: *v1beta1conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityError, "%s", fmt.Sprintf("Following Machines are reporting %s errors: %s", note, "m1")),
		},
		{
			name: "input kcp errors",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionTrue, clusterv1.ConditionSeverityNone)),
			},
			kcpErrors:         []string{"something error"},
			expectedCondition: *v1beta1conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityError, "something error"),
		},
		{
			name: "kcp machines with warnings",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionFalse, clusterv1.ConditionSeverityWarning)),
			},
			expectedCondition: *v1beta1conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityWarning, "%s", fmt.Sprintf("Following Machines are reporting %s warnings: %s", note, "m1")),
		},
		{
			name: "kcp machines with info",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionFalse, clusterv1.ConditionSeverityInfo)),
			},
			expectedCondition: *v1beta1conditions.FalseCondition(conditionType, unhealthyReason, clusterv1.ConditionSeverityInfo, "%s", fmt.Sprintf("Following Machines are reporting %s info: %s", note, "m1")),
		},
		{
			name: "kcp machines with true",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionTrue, clusterv1.ConditionSeverityNone)),
			},
			expectedCondition: *v1beta1conditions.TrueCondition(conditionType),
		},
		{
			name: "kcp machines with unknown",
			machines: []*clusterv1.Machine{
				fakeMachine("m1", withMachineReadyCondition(corev1.ConditionUnknown, clusterv1.ConditionSeverityNone)),
			},
			expectedCondition: *v1beta1conditions.UnknownCondition(conditionType, unknownReason, "%s", fmt.Sprintf("Following Machines are reporting unknown %s status: %s", note, "m1")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			input := aggregateV1Beta1ConditionsFromMachinesToKCPInput{
				controlPlane: &ControlPlane{
					KCP:      &controlplanev1.KubeadmControlPlane{},
					Machines: collections.FromMachines(tt.machines...),
				},
				machineConditions: []clusterv1.ConditionType{clusterv1.MachinesReadyV1Beta1Condition},
				kcpErrors:         tt.kcpErrors,
				condition:         conditionType,
				unhealthyReason:   unhealthyReason,
				unknownReason:     unknownReason,
				note:              note,
			}
			aggregateV1Beta1ConditionsFromMachinesToKCP(input)

			g.Expect(*v1beta1conditions.Get(input.controlPlane.KCP, conditionType)).To(v1beta1conditions.MatchCondition(tt.expectedCondition))
		})
	}
}

func TestAggregateConditionsFromMachinesToKCP(t *testing.T) {
	conditionType := controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition
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

			input := aggregateConditionsFromMachinesToKCPInput{
				controlPlane: &ControlPlane{
					KCP:      &controlplanev1.KubeadmControlPlane{},
					Machines: collections.FromMachines(tt.machines...),
				},
				machineConditions: []string{clusterv1.MachineReadyCondition},
				kcpErrors:         tt.kcpErrors,
				condition:         conditionType,
				trueReason:        trueReason,
				unknownReason:     unknownReason,
				falseReason:       falseReason,
				note:              note,
			}
			aggregateConditionsFromMachinesToKCP(input)

			g.Expect(*conditions.Get(input.controlPlane.KCP, conditionType)).To(conditions.MatchCondition(tt.expectedCondition, conditions.IgnoreLastTransitionTime(true)))
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
