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
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

// BeforeClusterCreateRequest is the request of the BeforeClusterCreate hook.
// +kubebuilder:object:root=true
type BeforeClusterCreateRequest struct {
	metav1.TypeMeta `json:",inline"`

	// The cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`
}

var _ AggregatableResponse = &BeforeClusterCreateResponse{}

// BeforeClusterCreateResponse is the response of BeforeClusterCreate hook.
// +kubebuilder:object:root=true
type BeforeClusterCreateResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// RetryAfterSeconds when set to a non-zero signifies that the hook
	// will be called again at a future time.
	RetryAfterSeconds int `json:"retryAfterSeconds"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`
}

// Aggregate combines individual extension handler responses into a single response of the hook.
func (r *BeforeClusterCreateResponse) Aggregate(responses []*ExtensionHandlerResponse) error {
	if err := aggregateExtensionResponses(r, responses); err != nil {
		return errors.Wrap(err, "failed to aggregate extension responses")
	}
	return nil
}

// BeforeClusterCreate is the runtime hook that will be called right before a Cluster is created.
func BeforeClusterCreate(*BeforeClusterCreateRequest, *BeforeClusterCreateResponse) {}

// AfterControlPlaneInitializedRequest is the request of the hook.
// +kubebuilder:object:root=true
type AfterControlPlaneInitializedRequest struct {
	metav1.TypeMeta `json:",inline"`

	// The cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`
}

var _ AggregatableResponse = &AfterControlPlaneInitializedResponse{}

// AfterControlPlaneInitializedResponse is the response of AfterControlPlaneInitialized hook.
// +kubebuilder:object:root=true
type AfterControlPlaneInitializedResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`
}

// Aggregate combines individual extension handler responses into a single response of the hook.
func (r *AfterControlPlaneInitializedResponse) Aggregate(responses []*ExtensionHandlerResponse) error {
	if err := aggregateExtensionResponses(r, responses); err != nil {
		return errors.Wrap(err, "failed to aggregate extension responses")
	}
	return nil
}

// AfterControlPlaneInitialized is the runtime hook that will be called after the first control plane is available.
func AfterControlPlaneInitialized(*AfterControlPlaneInitializedRequest, *AfterControlPlaneInitializedResponse) {
}

// BeforeClusterUpgradeRequest is the request of the hook.
// +kubebuilder:object:root=true
type BeforeClusterUpgradeRequest struct {
	metav1.TypeMeta `json:",inline"`

	// The cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`

	// The current version of the cluster.
	FromKubernetesVersion string `json:"fromKubernetesVersion"`
	// The target version of upgrade.
	ToKubernetesVersion string `json:"toKubernetesVersion"`
}

var _ AggregatableResponse = &BeforeClusterUpgradeResponse{}

// BeforeClusterUpgradeResponse is the response of BeforeClusterUpgrade hook.
// +kubebuilder:object:root=true
type BeforeClusterUpgradeResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// RetryAfterSeconds when set to a non-zero signifies that the hook
	// needs to be retried at a future time.
	RetryAfterSeconds int `json:"retryAfterSeconds"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`
}

// Aggregate combines individual extension handler responses into a single response of the hook.
func (r *BeforeClusterUpgradeResponse) Aggregate(responses []*ExtensionHandlerResponse) error {
	if err := aggregateRetryAfterSeconds(r, responses); err != nil {
		return errors.Wrap(err, "failed to compute aggregate retryAfterSeconds")
	}
	if err := aggregateExtensionResponses(r, responses); err != nil {
		return errors.Wrap(err, "failed to aggregate extension responses")
	}
	return nil
}

// BeforeClusterUpgrade is the runtime hook that will be called after a cluster.spec.version is upgraded and
// before the updated version is propagated to the underlying objects.
func BeforeClusterUpgrade(*BeforeClusterUpgradeRequest, *BeforeClusterUpgradeResponse) {}

// AfterControlPlaneUpgradeRequest is the request of the hook.
// +kubebuilder:object:root=true
type AfterControlPlaneUpgradeRequest struct {
	metav1.TypeMeta `json:",inline"`

	// The cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`

	// The version after upgrade.
	KubernetesVersion string `json:"kubernetesVersion"`
}

var _ AggregatableResponse = &AfterClusterUpgradeResponse{}

// AfterControlPlaneUpgradeResponse is the response of AfterControlPlaneUpgrade hook.
// +kubebuilder:object:root=true
type AfterControlPlaneUpgradeResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// RetryAfterSeconds when set to a non-zero signifies that the hook
	// needs to be retried at a future time.
	RetryAfterSeconds int `json:"retryAfterSeconds"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`
}

// Aggregate combines individual extension handler responses into a single response of the hook.
func (r *AfterControlPlaneUpgradeResponse) Aggregate(responses []*ExtensionHandlerResponse) error {
	if err := aggregateRetryAfterSeconds(r, responses); err != nil {
		return errors.Wrap(err, "failed to compute aggregate retryAfterSeconds")
	}
	if err := aggregateExtensionResponses(r, responses); err != nil {
		return errors.Wrap(err, "failed to aggregate extension responses")
	}
	return nil
}

// AfterControlPlaneUpgrade is the runtime hook after the control plane is successfully upgraded to the target
// kubernetes version and before the target version is propagated to the workload machines.
func AfterControlPlaneUpgrade(*AfterControlPlaneUpgradeRequest, *AfterControlPlaneUpgradeResponse) {}

// AfterClusterUpgradeRequest is the request of the hook.
// +kubebuilder:object:root=true
type AfterClusterUpgradeRequest struct {
	metav1.TypeMeta `json:",inline"`

	// The cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`

	// The version after upgrade.
	KubernetesVersion string `json:"kubernetesVersion"`
}

var _ AggregatableResponse = &AfterClusterUpgradeResponse{}

// AfterClusterUpgradeResponse is the response of AfterClusterUpgrade hook.
// +kubebuilder:object:root=true
type AfterClusterUpgradeResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`
}

// Aggregate combines individual extension handler responses into a single response of the hook.
func (r *AfterClusterUpgradeResponse) Aggregate(responses []*ExtensionHandlerResponse) error {
	if err := aggregateExtensionResponses(r, responses); err != nil {
		return errors.Wrap(err, "failed to aggregate extension responses")
	}
	return nil
}

// AfterClusterUpgrade is the runtime hook that is called after all of the cluster is updated
// to the target kubernetes version.
func AfterClusterUpgrade(*AfterClusterUpgradeRequest, *AfterClusterUpgradeResponse) {}

// BeforeClusterDeleteRequest is the request of the hook.
// +kubebuilder:object:root=true
type BeforeClusterDeleteRequest struct {
	metav1.TypeMeta `json:",inline"`

	// The cluster object the lifecycle hook corresponds to.
	Cluster clusterv1.Cluster `json:"cluster"`
}

var _ AggregatableResponse = &BeforeClusterDeleteResponse{}

// BeforeClusterDeleteResponse is the response of BeforeClusterDelete hook.
// +kubebuilder:object:root=true
type BeforeClusterDeleteResponse struct {
	metav1.TypeMeta `json:",inline"`

	// Status of the call. One of "Success" or "Failure".
	Status ResponseStatus `json:"status"`

	// RetryAfterSeconds when set to a non-zero signifies that the hook
	// needs to be retried at a future time.
	RetryAfterSeconds int `json:"retryAfterSeconds"`

	// A human-readable description of the status of the call.
	Message string `json:"message"`
}

// Aggregate combines individual extension handler responses into a single response of the hook.
func (r *BeforeClusterDeleteResponse) Aggregate(responses []*ExtensionHandlerResponse) error {
	if err := aggregateRetryAfterSeconds(r, responses); err != nil {
		return errors.Wrap(err, "failed to compute aggregate retryAfterSeconds")
	}
	if err := aggregateExtensionResponses(r, responses); err != nil {
		return errors.Wrap(err, "failed to aggregate extension responses")
	}
	return nil
}

// BeforeClusterDelete is the runtime hook that is called after a delete is issued on a cluster
// and before the cluster and its underlying objects are deleted.
func BeforeClusterDelete(*BeforeClusterDeleteRequest, *BeforeClusterDeleteResponse) {}

func init() {
	catalogBuilder.RegisterHook(BeforeClusterCreate, &catalog.HookMeta{
		Tags:        []string{"Lifecycle Hooks"},
		Summary:     "Called before Cluster topology is created",
		Description: "This blocking hook is called after the Cluster is created by the user and immediately before all the objects which are part of a Cluster topology are going to be created",
	})

	catalogBuilder.RegisterHook(AfterControlPlaneInitialized, &catalog.HookMeta{
		Tags:        []string{"Lifecycle Hooks"},
		Summary:     "Called after the Control Plane is available for the first time",
		Description: "This non-blocking hook is called after the ControlPlane for the Cluster is marked as available for the first time",
	})

	catalogBuilder.RegisterHook(BeforeClusterUpgrade, &catalog.HookMeta{
		Tags:        []string{"Lifecycle Hooks"},
		Summary:     "Called before the Cluster begins upgrade",
		Description: "This hook is called after the Cluster object has been updated with a new spec.topology.version by the user, and immediately before the new version is propagated to the Control Plane",
	})

	catalogBuilder.RegisterHook(AfterControlPlaneUpgrade, &catalog.HookMeta{
		Tags:        []string{"Lifecycle Hooks"},
		Summary:     "Called after the Control Plane finished upgrade",
		Description: "This blocking hook is called after the Control Plane has been upgraded to the version specified in spec.topology.version, and immediately before the new version is propagated the MachineDeployments of the Cluster",
	})

	catalogBuilder.RegisterHook(AfterClusterUpgrade, &catalog.HookMeta{
		Tags:        []string{"Lifecycle Hooks"},
		Summary:     "Called after the Cluster finished upgrade",
		Description: "This non-blocking hook is called after the Cluster, Control Plane and Workers have been upgraded to the version specified in spec.topology.version",
	})

	catalogBuilder.RegisterHook(BeforeClusterDelete, &catalog.HookMeta{
		Tags:        []string{"Lifecycle Hooks"},
		Summary:     "Called before the Cluster is deleted",
		Description: "This blocking hook is called after the Cluster has been deleted by the user, and immediately before objects of the Cluster are going to be deleted",
	})
}
