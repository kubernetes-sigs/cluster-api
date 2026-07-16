/*
Copyright 2026 The Kubernetes Authors.

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

package kubeadmcontrolplane

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	utilfeature "k8s.io/component-base/featuregate/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
)

func TestPreflightChecks(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	testCases := []struct {
		name                     string
		cluster                  *clusterv1.Cluster
		kcp                      *controlplanev1.KubeadmControlPlane
		machines                 []*clusterv1.Machine
		isScaleUp                bool
		expectResult             ctrl.Result
		expectPreflight          pkg.PreflightCheckResults
		expectDeferNextReconcile time.Duration
	}{
		{
			name:         "control plane without machines (not initialized) should pass",
			kcp:          &controlplanev1.KubeadmControlPlane{},
			expectResult: ctrl.Result{},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane with a pending upgrade should requeue",
			cluster: &clusterv1.Cluster{
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{
						Version: "v1.33.0",
					},
				},
			},
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.32.0",
				},
			},
			machines: []*clusterv1.Machine{
				{},
			},

			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          true,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane with a pending upgrade, but not yet at the current step of the upgrade plan, should requeue",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyUpgradeStepAnnotation: "v1.32.0",
					},
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{
						Version: "v1.33.0",
					},
				},
			},
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.31.0",
				},
			},
			machines: []*clusterv1.Machine{
				{},
			},

			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          true,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane with a deleting machine should requeue",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               true,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane without certificates should requeue if scale up",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{
							Type:   controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition,
							Status: metav1.ConditionFalse,
							Reason: controlplanev1.KubeadmControlPlaneCertificatesNotAvailableReason,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{},
			},
			isScaleUp:    true,
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               true,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane without certificates should pass if not scale up",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
						{Type: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition, Status: metav1.ConditionTrue},
						{Type: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition, Status: metav1.ConditionTrue},
						{
							Type:   controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition,
							Status: metav1.ConditionFalse,
							Reason: controlplanev1.KubeadmControlPlaneCertificatesNotAvailableReason,
						},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			isScaleUp:    false,
			expectResult: ctrl.Result{},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},

		{
			name: "control plane without a nodeRef should requeue",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						// NodeRef is not set
						// Note: with v1beta1 no conditions are applied to machine when NodeRef is not set, this will change with v1beta2.
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: true,
				EtcdClusterNotHealthy:            true,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane with an unhealthy machine condition should requeue",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionFalse},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: true,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane with an unhealthy machine condition should requeue",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse},
						},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            true,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane where target etcd cluster and k8s control plane will be healthy state should pass when scaling up after remediation, no matter of failures on other machines",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.RemediationInProgressAnnotation: "...",
					},
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-2",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-2",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionFalse},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			isScaleUp:    true,
			expectResult: ctrl.Result{},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "requeue control plane where target k8s control plane will be unhealthy when scaling up after remediation",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.RemediationInProgressAnnotation: "...",
					},
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionFalse},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-2",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-2",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionFalse},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			isScaleUp:    true,
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: true,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "requeue control plane where target etcd cluster will be unhealthy when scaling up after remediation",
			kcp: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						controlplanev1.RemediationInProgressAnnotation: "...",
					},
				},
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-2",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-2",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionFalse},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-3",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-3",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			isScaleUp:    true,
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            true,
				TopologyVersionMismatch:          false,
			},
			expectDeferNextReconcile: 5 * time.Second,
		},
		{
			name: "control plane with an healthy machine and an healthy kcp condition should pass",
			kcp: &controlplanev1.KubeadmControlPlane{
				Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
						{Type: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition, Status: metav1.ConditionTrue},
						{Type: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},
			expectResult: ctrl.Result{},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
		{
			name: "control plane with a pending upgrade, but already at the current step of the upgrade plan, should pass",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyUpgradeStepAnnotation: "v1.32.0",
					},
				},
				Spec: clusterv1.ClusterSpec{
					Topology: clusterv1.Topology{
						Version: "v1.33.0",
					},
				},
			},
			kcp: &controlplanev1.KubeadmControlPlane{
				Spec: controlplanev1.KubeadmControlPlaneSpec{
					Version: "v1.32.0",
				}, Status: controlplanev1.KubeadmControlPlaneStatus{
					Conditions: []metav1.Condition{
						{Type: controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition, Status: metav1.ConditionTrue},
						{Type: controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyCondition, Status: metav1.ConditionTrue},
						{Type: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition, Status: metav1.ConditionTrue},
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "machine-1",
					},
					Status: clusterv1.MachineStatus{
						NodeRef: clusterv1.MachineNodeReference{
							Name: "node-1",
						},
						Conditions: []metav1.Condition{
							{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue},
							{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue},
						},
					},
				},
			},

			expectResult: ctrl.Result{},
			expectPreflight: pkg.PreflightCheckResults{
				HasDeletingMachine:               false,
				CertificateMissing:               false,
				ControlPlaneComponentsNotHealthy: false,
				EtcdClusterNotHealthy:            false,
				TopologyVersionMismatch:          false,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fc := capicontrollerutil.NewFakeController()

			r := &Reconciler{
				controller: fc,
				recorder:   record.NewFakeRecorder(32),
			}
			cluster := &clusterv1.Cluster{}
			if tt.cluster != nil {
				cluster = tt.cluster
			}
			controlPlane := &pkg.ControlPlane{
				Cluster:     cluster,
				KCP:         tt.kcp,
				Machines:    collections.FromMachines(tt.machines...),
				EtcdMembers: etcdMembers(collections.FromMachines(tt.machines...)),
			}
			result := r.preflightChecks(context.TODO(), controlPlane, tt.isScaleUp)
			g.Expect(result).To(BeComparableTo(tt.expectResult))
			g.Expect(controlPlane.PreflightCheckResults).To(Equal(tt.expectPreflight))
			if tt.expectDeferNextReconcile == 0 {
				g.Expect(fc.Deferrals).To(BeEmpty())
			} else {
				g.Expect(fc.Deferrals).To(HaveKeyWithValue(
					reconcile.Request{NamespacedName: client.ObjectKeyFromObject(tt.kcp)},
					BeTemporally("~", time.Now().Add(tt.expectDeferNextReconcile), 1*time.Second)),
				)
			}
		})
	}
}

func TestPreflightCheckCondition(t *testing.T) {
	condition := "fooCondition"
	testCases := []struct {
		name      string
		machine   *clusterv1.Machine
		expectErr bool
	}{
		{
			name:      "missing condition should return error",
			machine:   &clusterv1.Machine{},
			expectErr: true,
		},
		{
			name: "false condition should return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{Type: condition, Status: metav1.ConditionFalse},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "unknown condition should return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{Type: condition, Status: metav1.ConditionUnknown},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "true condition should not return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: []metav1.Condition{
						{Type: condition, Status: metav1.ConditionTrue},
					},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := preflightCheckCondition("machine", tt.machine, condition)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
