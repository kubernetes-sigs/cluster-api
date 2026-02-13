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

package taints

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
)

// ValidateMachineTaints validates MachineTaints.
func ValidateMachineTaints(taints []clusterv1.MachineTaint, taintsPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if !feature.Gates.Enabled(feature.MachineTaintPropagation) {
		if len(taints) > 0 {
			allErrs = append(allErrs, field.Forbidden(taintsPath, "taints are not allowed to be set when the feature gate MachineTaintPropagation is disabled"))
		}
	}

	for i, taint := range taints {
		idxPath := taintsPath.Index(i)

		// The following validations uses a switch statement, because if one of them matches, then the others won't.

		switch {
		// Validate for keys which are reserved for usage by the cluster-api or providers.
		case taint.Key == clusterv1.NodeUninitializedTaint.Key:
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key is not allowed"))
		case taint.Key == clusterv1.NodeOutdatedRevisionTaint.Key:
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key is not allowed"))
		// Validate for keys which are reserved for usage by the node or node-lifecycle-controller, but allow `node.kubernetes.io/out-of-service`.
		case strings.HasPrefix(taint.Key, "node.kubernetes.io/") && taint.Key != "node.kubernetes.io/out-of-service":
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key must not have the prefix node.kubernetes.io/, except for node.kubernetes.io/out-of-service"))
		// Validate for keys which are reserved for usage by the cloud-controller-manager or kubelet.
		case strings.HasPrefix(taint.Key, "node.cloudprovider.kubernetes.io/"):
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint key must not have the prefix node.cloudprovider.kubernetes.io/"))
		// Validate for the deprecated kubeadm node-role taint.
		case taint.Key == "node-role.kubernetes.io/master":
			allErrs = append(allErrs, field.Invalid(idxPath.Child("key"), taint.Key, "taint is deprecated since 1.24 and should not be used anymore"))
		}
	}

	return allErrs
}
