/*
Copyright 2021 The Kubernetes Authors.

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

package v1alpha3

const (
	// CertManagerVersionAnnotation reports the cert manager version installed by clusterctl.
	CertManagerVersionAnnotation = "cert-manager.clusterctl.cluster.x-k8s.io/version"

	// SkipCRDNamePreflightCheckAnnotation can be placed on provider CRDs, so that clusterctl doesn't emit a
	// warning if the CRD doesn't comply with Cluster APIs naming scheme.
	// Note: Only CRDs that are referenced by core Cluster API CRDs have to comply with the naming scheme.
	// See the following issue for more information: https://github.com/kubernetes-sigs/cluster-api/issues/5686#issuecomment-1260897278
	SkipCRDNamePreflightCheckAnnotation = "clusterctl.cluster.x-k8s.io/skip-crd-name-preflight-check"

	// DeleteForMoveAnnotation will be set to objects that are going to be deleted from the
	// source cluster after being moved to the target cluster during the clusterctl move operation.
	//
	// It will help any validation webhook to take decision based on it.
	DeleteForMoveAnnotation = "clusterctl.cluster.x-k8s.io/delete-for-move"

	// BlockMoveAnnotation prevents the cluster move operation from starting if it is defined on at least one
	// of the objects in scope.
	// Provider controllers are expected to set the annotation on resources that cannot be instantaneously
	// paused and remove the annotation when the resource has been actually paused.
	//
	// e.g. If this annotation is defined with any value on an InfraMachine resource to be moved when
	// `clusterctl move` is invoked, then NO resources for ANY workload cluster will be created on the
	// destination management cluster until the annotation is removed.
	BlockMoveAnnotation = "clusterctl.cluster.x-k8s.io/block-move"
)
