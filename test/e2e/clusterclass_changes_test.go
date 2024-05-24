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
	"k8s.io/utils/ptr"
)

var _ = Describe("When testing ClusterClass changes [ClusterClass]", func() {
	ClusterClassChangesSpec(ctx, func() ClusterClassChangesSpecInput {
		return ClusterClassChangesSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			Flavor:                 "topology",
			// ModifyControlPlaneFields are the ControlPlane fields which will be set on the
			// ControlPlaneTemplate of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the ControlPlane.
			ModifyControlPlaneFields: map[string]interface{}{
				"spec.kubeadmConfigSpec.verbosity": int64(4),
			},
			// ModifyMachineDeploymentBootstrapConfigTemplateFields are the fields which will be set on the
			// BootstrapConfigTemplate of all MachineDeploymentClasses of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the MachineDeployments.
			ModifyMachineDeploymentBootstrapConfigTemplateFields: map[string]interface{}{
				"spec.template.spec.verbosity": int64(4),
			},
			// ModifyMachineDeploymentInfrastructureMachineTemplateFields are the fields which will be set on the
			// InfrastructureMachineTemplate of all MachineDeploymentClasses of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the MachineDeployments.
			ModifyMachineDeploymentInfrastructureMachineTemplateFields: map[string]interface{}{
				"spec.template.spec.extraMounts": []interface{}{
					map[string]interface{}{
						"containerPath": "/var/run/docker.sock",
						"hostPath":      "/var/run/docker.sock",
					},
					map[string]interface{}{
						// /tmp cannot be used as containerPath as
						// it already exists.
						"containerPath": "/test",
						"hostPath":      "/tmp",
					},
				},
			},
			// ModifyMachinePoolBootstrapConfigTemplateFields are the fields which will be set on the
			// BootstrapConfigTemplate of all MachinePoolClasses of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the MachinePools.
			ModifyMachinePoolBootstrapConfigTemplateFields: map[string]interface{}{
				"spec.template.spec.verbosity": int64(4),
			},
			// ModifyMachinePoolInfrastructureMachineTemplateFields are the fields which will be set on the
			// InfrastructureMachinePoolTemplate of all MachinePoolClasses of the ClusterClass after the initial Cluster creation.
			// The test verifies that these fields are rolled out to the MachinePools.
			ModifyMachinePoolInfrastructureMachinePoolTemplateFields: map[string]interface{}{
				"spec.template.spec.template.extraMounts": []interface{}{
					map[string]interface{}{
						"containerPath": "/var/run/docker.sock",
						"hostPath":      "/var/run/docker.sock",
					},
					map[string]interface{}{
						// /tmp cannot be used as containerPath as
						// it already exists.
						"containerPath": "/test",
						"hostPath":      "/tmp",
					},
				},
			},
		}
	})
})
