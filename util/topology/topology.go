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

// Package topology implements topology utility functions.
package topology

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// ShouldSkipImmutabilityChecks returns true if it is a dry-run request and the object has the
// TopologyDryRunAnnotation annotation set, false otherwise.
// This ensures that the immutability check is skipped only when dry-running and when the operations has been invoked by the topology controller.
// Instead, kubectl dry-run behavior remains consistent with the one user gets when doing kubectl apply (immutability is enforced).
//
// Deprecated: Please use IsDryRunRequest instead.
func ShouldSkipImmutabilityChecks(req admission.Request, obj metav1.Object) bool {
	return IsDryRunRequest(req, obj)
}

// IsDryRunRequest returns true if it is a dry-run request and the object has the
// TopologyDryRunAnnotation annotation set, false otherwise.
func IsDryRunRequest(req admission.Request, obj metav1.Object) bool {
	// Check if the request is a dry-run
	if req.DryRun == nil || !*req.DryRun {
		return false
	}

	if obj == nil {
		return false
	}

	// Check for the TopologyDryRunAnnotation
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	if _, ok := annotations[clusterv1.TopologyDryRunAnnotation]; ok {
		return true
	}
	return false
}
