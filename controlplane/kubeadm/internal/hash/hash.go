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

package hash

import (
	"fmt"
	"hash/fnv"

	corev1 "k8s.io/api/core/v1"

	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/mdutil"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
)

type fieldsToHash struct {
	version                string
	infrastructureTemplate corev1.ObjectReference
	kubeadmConfigSpec      cabpkv1.KubeadmConfigSpec
}

// Compute will generate a 32-bit FNV-1a Hash of the Version, InfrastructureTemplate and KubeadmConfigSpec
// fields for the given KubeadmControlPlaneSpec
func Compute(spec *controlplanev1.KubeadmControlPlaneSpec) string {
	// since we only care about spec.Version, spec.InfrastructureTemplate, and
	// spec.KubeadmConfigSpec and to avoid changing the hash if additional fields
	// are added, we copy those values to a fieldsToHash instance
	specToHash := fieldsToHash{
		version:                spec.Version,
		infrastructureTemplate: spec.InfrastructureTemplate,
		kubeadmConfigSpec:      spec.KubeadmConfigSpec,
	}

	hasher := fnv.New32a()
	mdutil.DeepHashObject(hasher, specToHash)

	return fmt.Sprintf("%d", hasher.Sum32())
}
