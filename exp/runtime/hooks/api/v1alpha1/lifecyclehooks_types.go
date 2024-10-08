/*
Copyright 2022 The Kubernetes Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// BeforeClusterCreateRequest is the request of the BeforeClusterCreate hook.
// +kubebuilder:object:root=true
type BeforeClusterCreateRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`
}

var _ RetryResponseObject = &BeforeClusterCreateResponse{}

// BeforeClusterCreateResponse is the response of the BeforeClusterCreate hook.
// +kubebuilder:object:root=true
type BeforeClusterCreateResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRetryResponse contains Status, Message and RetryAfterSeconds fields.
	CommonRetryResponse `json:",inline"`
}

// BeforeClusterCreate is the hook that will be called right before the topology of the Cluster is created.
func BeforeClusterCreate(*BeforeClusterCreateRequest, *BeforeClusterCreateResponse) {}

// AfterControlPlaneInitializedRequest is the request of the AfterControlPlaneInitialized hook.
// +kubebuilder:object:root=true
type AfterControlPlaneInitializedRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`
}

var _ ResponseObject = &AfterControlPlaneInitializedResponse{}

// AfterControlPlaneInitializedResponse is the response of the AfterControlPlaneInitialized hook.
// +kubebuilder:object:root=true
type AfterControlPlaneInitializedResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`
}

// AfterControlPlaneInitialized is the hook that will be called after the control plane is initialized for the first time.
func AfterControlPlaneInitialized(*AfterControlPlaneInitializedRequest, *AfterControlPlaneInitializedResponse) {
}

// BeforeClusterUpgradeRequest is the request of the BeforeClusterUpgrade hook.
// +kubebuilder:object:root=true
type BeforeClusterUpgradeRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`

	// fromKubernetesVersion is the current Kubernetes version of the cluster.
	FromKubernetesVersion string `json:"fromKubernetesVersion"`

	// toKubernetesVersion is the target Kubernetes version of the upgrade.
	ToKubernetesVersion string `json:"toKubernetesVersion"`
}

var _ RetryResponseObject = &BeforeClusterUpgradeResponse{}

// BeforeClusterUpgradeResponse is the response of the BeforeClusterUpgrade hook.
// +kubebuilder:object:root=true
type BeforeClusterUpgradeResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRetryResponse contains Status, Message and RetryAfterSeconds fields.
	CommonRetryResponse `json:",inline"`
}

// BeforeClusterUpgrade is the hook that will be called after a Cluster.spec.version is upgraded and
// before the updated version is propagated to the underlying objects.
func BeforeClusterUpgrade(*BeforeClusterUpgradeRequest, *BeforeClusterUpgradeResponse) {}

// AfterControlPlaneUpgradeRequest is the request of the AfterControlPlaneUpgrade hook.
// +kubebuilder:object:root=true
type AfterControlPlaneUpgradeRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`

	// kubernetesVersion is the Kubernetes version of the Control Plane after the upgrade.
	KubernetesVersion string `json:"kubernetesVersion"`
}

var _ RetryResponseObject = &AfterControlPlaneUpgradeResponse{}

// AfterControlPlaneUpgradeResponse is the response of the AfterControlPlaneUpgrade hook.
// +kubebuilder:object:root=true
type AfterControlPlaneUpgradeResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRetryResponse contains Status, Message and RetryAfterSeconds fields.
	CommonRetryResponse `json:",inline"`
}

// AfterControlPlaneUpgrade is the hook called after the control plane is successfully upgraded to the target
// Kubernetes version and before the target version is propagated to the workload machines.
func AfterControlPlaneUpgrade(*AfterControlPlaneUpgradeRequest, *AfterControlPlaneUpgradeResponse) {}

// AfterClusterUpgradeRequest is the request of the AfterClusterUpgrade hook.
// +kubebuilder:object:root=true
type AfterClusterUpgradeRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`

	// kubernetesVersion is the Kubernetes version after upgrade.
	KubernetesVersion string `json:"kubernetesVersion"`
}

var _ ResponseObject = &AfterClusterUpgradeResponse{}

// AfterClusterUpgradeResponse is the response of the AfterClusterUpgrade hook.
// +kubebuilder:object:root=true
type AfterClusterUpgradeResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`
}

// AfterClusterUpgrade is the hook that is called after the entire cluster is updated
// to the target Kubernetes version.
func AfterClusterUpgrade(*AfterClusterUpgradeRequest, *AfterClusterUpgradeResponse) {}

// BeforeClusterDeleteRequest is the request of the BeforeClusterDelete hook.
// +kubebuilder:object:root=true
type BeforeClusterDeleteRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`
}

var _ RetryResponseObject = &BeforeClusterDeleteResponse{}

// BeforeClusterDeleteResponse is the response of the BeforeClusterDelete hook.
// +kubebuilder:object:root=true
type BeforeClusterDeleteResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRetryResponse contains Status, Message and RetryAfterSeconds fields.
	CommonRetryResponse `json:",inline"`
}

// BeforeClusterDelete is the hook that is called after delete is issued on a cluster
// and before the cluster and its underlying objects are deleted.
func BeforeClusterDelete(*BeforeClusterDeleteRequest, *BeforeClusterDeleteResponse) {}

func init() {
	catalogBuilder.RegisterHook(BeforeClusterCreate, &runtimecatalog.HookMeta{
		Tags:    []string{"Lifecycle Hooks"},
		Summary: "Cluster API Runtime will call this hook before a Cluster's topology is created",
		Description: "Cluster API Runtime will call this hook after the Cluster is created by the user and immediately before " +
			"all the objects which are part of a Cluster's topology are going to be created.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook will be called only for Clusters with a managed topology\n" +
			"- The call's request contains the Cluster object\n" +
			"- This is a blocking hook; Runtime Extension implementers can use this hook to execute\n" +
			"tasks before the objects which are part of a Cluster's topology are created",
	})

	catalogBuilder.RegisterHook(AfterControlPlaneInitialized, &runtimecatalog.HookMeta{
		Tags:    []string{"Lifecycle Hooks"},
		Summary: "Cluster API Runtime will call this hook after the control plane is reachable for the first time",
		Description: "Cluster API Runtime will call this hook after the control plane for the Cluster is reachable for the first time.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook will be called only for Clusters with a managed topology\n" +
			"- This is a non-blocking hook",
	})

	catalogBuilder.RegisterHook(BeforeClusterUpgrade, &runtimecatalog.HookMeta{
		Tags:    []string{"Lifecycle Hooks"},
		Summary: "Cluster API Runtime will call this hook before the Cluster is upgraded",
		Description: "Cluster API Runtime will call this hook after the Cluster object has been updated with a new spec.topology.version by the user, " +
			"and immediately before the new version is propagated to the control plane.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook will be called only for Clusters with a managed topology\n" +
			"- The call's request contains the Cluster object, the current Kubernetes version and the Kubernetes version we are upgrading to\n" +
			"- This is a blocking hook; Runtime Extension implementers can use this hook to execute " +
			"tasks before the new version is propagated to the control plane",
	})

	catalogBuilder.RegisterHook(AfterControlPlaneUpgrade, &runtimecatalog.HookMeta{
		Tags:    []string{"Lifecycle Hooks"},
		Summary: "Cluster API Runtime will call this hook after the control plane is upgraded",
		Description: "Cluster API Runtime will call this hook after the a cluster's control plane has been upgraded to the version specified " +
			"in spec.topology.version, and immediately before the new version is going to be propagated to the MachineDeployments. " +
			"A control plane upgrade is completed when all the machines in the control plane have been upgraded.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook will be called only for Clusters with a managed topology\n" +
			"- The call's request contains the Cluster object and the Kubernetes version we upgraded to\n" +
			"- This is a blocking hook; Runtime Extension implementers can use this hook to execute " +
			"tasks before the new version is propagated to the MachineDeployments",
	})

	catalogBuilder.RegisterHook(AfterClusterUpgrade, &runtimecatalog.HookMeta{
		Tags:    []string{"Lifecycle Hooks"},
		Summary: "Cluster API Runtime will call this hook after a Cluster is upgraded",
		Description: "Cluster API Runtime will call this hook after a Cluster has been upgraded to the version specified " +
			"in spec.topology.version. An upgrade is completed when all control plane and MachineDeployment's Machines have been upgraded.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook will be called only for Clusters with a managed topology\n" +
			"- The call's request contains the Cluster object and the Kubernetes version we upgraded to \n" +
			"- This is a non-blocking hook",
	})

	catalogBuilder.RegisterHook(BeforeClusterDelete, &runtimecatalog.HookMeta{
		Tags:    []string{"Lifecycle Hooks"},
		Summary: "Cluster API Runtime will call this hook before the Cluster is deleted",
		Description: "Cluster API Runtime will call this hook after the Cluster deletion has been triggered by the user, " +
			"and immediately before objects of the Cluster are going to be deleted.\n" +
			"\n" +
			"Notes:\n" +
			"- This hook will be called only for Clusters with a managed topology\n" +
			"- The call's request contains the Cluster object \n" +
			"- This is a blocking hook; Runtime Extension implementers can use this hook  to execute " +
			"tasks before objects of the Cluster are deleted",
	})
}
