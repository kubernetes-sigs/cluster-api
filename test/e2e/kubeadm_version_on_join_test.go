//go:build e2e
// +build e2e

/*
Copyright 2026 The Kubernetes Authors.

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

var _ = Describe("When a worker joins during a control plane upgrade [ClusterClass]", Label("ClusterClass"), func() {
	KubeadmVersionOnJoinSpec(ctx, func() KubeadmVersionOnJoinSpecInput {
		return KubeadmVersionOnJoinSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			Flavor:                 "topology-kubeadm-version",
		}
	})
})
