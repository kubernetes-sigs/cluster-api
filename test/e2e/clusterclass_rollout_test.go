//go:build e2e
// +build e2e

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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
)

var _ = Describe("When testing ClusterClass rollouts [ClusterClass]", Label("ClusterClass"), func() {
	ClusterClassRolloutSpec(ctx, func() ClusterClassRolloutSpecInput {
		return ClusterClassRolloutSpecInput{
			E2EConfig:                      e2eConfig,
			ClusterctlConfigPath:           clusterctlConfigPath,
			BootstrapClusterProxy:          bootstrapClusterProxy,
			ArtifactFolder:                 artifactFolder,
			SkipCleanup:                    skipCleanup,
			Flavor:                         "topology-taints",
			FilterMetadataBeforeValidation: filterMetadataBeforeValidation,
		}
	})
})

func filterMetadataBeforeValidation(object ctrlclient.Object) clusterv1.ObjectMeta {
	if object.GetObjectKind().GroupVersionKind() == infrav1.GroupVersion.WithKind("DevMachine") {
		// CAPDdev adds an extra label devmachine.infrastructure.cluster.x-k8s.io/weight on DevMachine, we need to filter it out to pass the
		// clusterclass rollout test
		annotations := object.GetAnnotations()
		delete(annotations, infrav1.LoadbalancerWeightAnnotation)
		object.SetAnnotations(annotations)
		return clusterv1.ObjectMeta{Labels: object.GetLabels(), Annotations: object.GetAnnotations()}
	}

	// If the object is not a Machine, just return the default labels and annotations of the object
	return clusterv1.ObjectMeta{Labels: object.GetLabels(), Annotations: object.GetAnnotations()}
}
