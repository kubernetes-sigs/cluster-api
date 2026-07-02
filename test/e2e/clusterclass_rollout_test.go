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
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

var _ = Describe("When testing ClusterClass rollouts [ClusterClass]", Label("ClusterClass"), func() {
	ClusterClassRolloutSpec(ctx, func() ClusterClassRolloutSpecInput {
		return ClusterClassRolloutSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                "in-memory-topology",
			// The runtime extension gets deployed to the test-extension-system namespace and is exposed
			// by the test-extension-webhook-service.
			// The below values are used when creating the cluster-wide ExtensionConfig to refer
			// the actual service.
			ExtensionServiceNamespace: "test-extension-system",
			ExtensionServiceName:      "test-extension-webhook-service",
			FilterMetadataBeforeValidation: func(object client.Object) clusterv1.ObjectMeta {
				annotations := object.GetAnnotations()
				delete(annotations, "inmemorycluster.infrastructure.cluster.x-k8s.io/listener")
				delete(annotations, "machine.inmemory.infrastructure.cluster.x-k8s.io/bootstrapped")
				return clusterv1.ObjectMeta{
					Labels:      object.GetLabels(),
					Annotations: annotations,
				}
			},
		}
	})
})
