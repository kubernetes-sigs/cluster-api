/*
Copyright 2025 The Kubernetes Authors.

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

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// GenerateUpgradePlanRequest is the request of the GenerateUpgradePlan hook.
// +kubebuilder:object:root=true
type GenerateUpgradePlanRequest struct {
	metav1.TypeMeta `json:",inline"`

	// CommonRequest contains fields common to all request types.
	CommonRequest `json:",inline"`

	// cluster is the cluster ofject the lifecycle hook correspods to.
	// +required
	Cluster clusterv1beta1.Cluster `json:"cluster,omitempty,omitzero"`

	// fromControlPlaneKubernetesVersion is the current Kubernetes version of the control plane.
	// +required
	// +kubebuilder:validation:MinLength=1
	FromControlPlaneKubernetesVersion string `json:"fromControlPlaneKubernetesVersion,omitempty"`

	// fromWorkersKubernetesVersion is the current Kubernetes version of the workers.
	// +optional
	// +kubebuilder:validation:MinLength=1
	FromWorkersKubernetesVersion string `json:"fromWorkersKubernetesVersion,omitempty"`

	// toKubernetesVersion is the target Kubernetes version for the upgrade.
	// +required
	// +kubebuilder:validation:MinLength=1
	ToKubernetesVersion string `json:"toKubernetesVersion,omitempty"`
}

var _ ResponseObject = &GenerateUpgradePlanResponse{}

// GenerateUpgradePlanResponse is the response of the GenerateUpgradePlan hook.
// +kubebuilder:object:root=true
type GenerateUpgradePlanResponse struct {
	metav1.TypeMeta `json:",inline"`

	// CommonResponse contains Status and Message fields common to all response types.
	CommonResponse `json:",inline"`

	// controlPlaneUpgrades is the list of version upgrade steps for the control plane.
	// Each entry represents an intermediate version that must be applied in sequence.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	ControlPlaneUpgrades []UpgradeStep `json:"controlPlaneUpgrades,omitempty"`

	// workersUpgrades is the list of version upgrade steps for the workers.
	// Each entry represents an intermediate version that must be applied in sequence.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1000
	WorkersUpgrades []UpgradeStep `json:"workersUpgrades,omitempty"`
}

// UpgradeStep represents a single version upgrade step.
type UpgradeStep struct {
	// version is the Kubernetes version for this upgrade step.
	// +required
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version,omitempty"`
}

// GenerateUpgradePlan is the hook that will be called to generate an upgrade plan
// for a cluster. This hook allows runtime extensions to specify intermediate
// Kubernetes versions that must be applied during an upgrade from the current
// version to the target version.
func GenerateUpgradePlan(*GenerateUpgradePlanRequest, *GenerateUpgradePlanResponse) {}

func init() {
	catalogBuilder.RegisterHook(GenerateUpgradePlan, &runtimecatalog.HookMeta{
		Tags:    []string{"Chained Upgrade Hook"},
		Summary: "Cluster API Runtime will call this hook to generate an upgrade plan for a cluster",
		Description: "Cluster API Runtime will call this hook to generate an upgrade plan for a cluster. " +
			"Runtime Extension implementers can use this hook to specify intermediate Kubernetes versions " +
			"that must be applied during an upgrade from the current version to the target version.\n" +
			"\n" +
			"For example, if upgrading from v1.29.0 to v1.33.0 requires intermediate versions v1.30.0, " +
			"v1.31.0, and v1.32.0, the hook should return these intermediate versions in the response.\n" +
			"\n" +
			"Notes:\n" +
			"- The response may include separate upgrade paths for control plane and workers\n" +
			"- Each upgrade step represents a version that must be applied in sequence",
	})
}
