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
	. "github.com/onsi/ginkgo"
)

var _ = Describe("When testing ClusterClass changes [ClusterClass]", func() {
	ClusterClassChangesSpec(ctx, func() ClusterClassChangesSpecInput {
		return ClusterClassChangesSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			Flavor:                "topology",
			// ModifyControlPlaneFields are the ControlPlane fields which will be set on the
			// ControlPlaneTemplate of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the ControlPlane.
			ModifyControlPlaneFields: map[string]interface{}{
				"spec.machineTemplate.nodeDrainTimeout": "10s",
			},
			// ModifyMachineDeploymentBootstrapConfigTemplateFields are the fields which will be set on the
			// BootstrapConfigTemplate of all MachineDeploymentClasses of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the MachineDeployments.
			ModifyMachineDeploymentBootstrapConfigTemplateFields: map[string]interface{}{
				"spec.template.spec.verbosity": int64(4),
			},
		}
	})
})
