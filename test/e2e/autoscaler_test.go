//go:build e2e
// +build e2e

/*
Copyright 2023 The Kubernetes Authors.

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
	"k8s.io/utils/ptr"
)

var _ = Describe("When using the autoscaler with Cluster API using ClusterClass [ClusterClass]", func() {
	AutoscalerSpec(ctx, func() AutoscalerSpecInput {
		return AutoscalerSpecInput{
			E2EConfig:                             e2eConfig,
			ClusterctlConfigPath:                  clusterctlConfigPath,
			BootstrapClusterProxy:                 bootstrapClusterProxy,
			ArtifactFolder:                        artifactFolder,
			SkipCleanup:                           skipCleanup,
			InfrastructureProvider:                ptr.To("docker"),
			InfrastructureMachineTemplateKind:     "dockermachinetemplates",
			InfrastructureMachinePoolTemplateKind: "dockermachinepooltemplates",
			InfrastructureMachinePoolKind:         "dockermachinepools",
			Flavor:                                ptr.To("topology-autoscaler"),
			AutoscalerVersion:                     "v1.30.0",
		}
	})
})
