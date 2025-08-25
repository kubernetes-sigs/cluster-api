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
	"context"
	"fmt"
	"path/filepath"

	"github.com/blang/semver/v4"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

// Test suite constants for e2e config variables.
const (
	KubernetesVersionManagement     = "KUBERNETES_VERSION_MANAGEMENT"
	KubernetesVersion               = "KUBERNETES_VERSION"
	CNIPath                         = "CNI"
	CNIResources                    = "CNI_RESOURCES"
	KubernetesVersionUpgradeFrom    = "KUBERNETES_VERSION_UPGRADE_FROM"
	KubernetesVersionUpgradeTo      = "KUBERNETES_VERSION_UPGRADE_TO"
	CPMachineTemplateUpgradeTo      = "CONTROL_PLANE_MACHINE_TEMPLATE_UPGRADE_TO"
	WorkersMachineTemplateUpgradeTo = "WORKERS_MACHINE_TEMPLATE_UPGRADE_TO"
	EtcdVersionUpgradeTo            = "ETCD_VERSION_UPGRADE_TO"
	CoreDNSVersionUpgradeTo         = "COREDNS_VERSION_UPGRADE_TO"
	IPFamily                        = "IP_FAMILY"
)

var stableReleaseMarkerPrefix = "go://sigs.k8s.io/cluster-api@v%s"
var latestReleaseMarkerPrefix = "go://sigs.k8s.io/cluster-api@latest-v%s"

func Byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...))
}

// HaveValidVersion succeeds if version is a valid semver version.
func HaveValidVersion(version string) types.GomegaMatcher {
	return &validVersionMatcher{version: version}
}

type validVersionMatcher struct{ version string }

func (m *validVersionMatcher) Match(_ interface{}) (success bool, err error) {
	if _, err := semver.ParseTolerant(m.version); err != nil {
		return false, err
	}
	return true, nil
}

func (m *validVersionMatcher) FailureMessage(_ interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s", m.version, " to be a valid version ")
}

func (m *validVersionMatcher) NegatedFailureMessage(_ interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s", m.version, " not to be a valid version ")
}

// GetStableReleaseOfMinor returns latest stable version of minorRelease.
func GetStableReleaseOfMinor(ctx context.Context, minorRelease string) (string, error) {
	releaseMarker := fmt.Sprintf(stableReleaseMarkerPrefix, minorRelease)
	return clusterctl.ResolveRelease(ctx, releaseMarker)
}

// GetLatestReleaseOfMinor returns latest version of minorRelease.
func GetLatestReleaseOfMinor(ctx context.Context, minorRelease string) (string, error) {
	releaseMarker := fmt.Sprintf(latestReleaseMarkerPrefix, minorRelease)
	return clusterctl.ResolveRelease(ctx, releaseMarker)
}

func initScheme() *runtime.Scheme {
	sc := runtime.NewScheme()
	framework.TryAddDefaultSchemes(sc)
	return sc
}

func dumpKindClusterLogs(ctx context.Context, artifactFolder string, clusterProxy framework.ClusterProxy) {
	if clusterProxy == nil {
		return
	}

	clusterLogCollector := clusterProxy.GetLogCollector()
	if clusterLogCollector == nil {
		return
	}

	nodes, err := clusterProxy.GetClientSet().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Failed to get nodes for the cluster: %v\n", err)
		return
	}

	for i := range nodes.Items {
		nodeName := nodes.Items[i].GetName()
		err = clusterLogCollector.CollectMachineLog(
			ctx,
			clusterProxy.GetClient(),
			// The kind cluster is not a CAPI cluster, so in order to re-use the logCollector,
			// we create a fake machine that wraps the node.
			// NOTE: This assumes a naming convention between machines and nodes, which e.g. applies to the clusters generated with kind.
			//       This might not work if you are using an existing cluster provided by other means.
			&clusterv1.Machine{
				Spec:       clusterv1.MachineSpec{ClusterName: nodeName},
				ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			},
			filepath.Join(artifactFolder, "clusters", clusterProxy.GetName(), "machines", nodeName),
		)
		if err != nil {
			fmt.Printf("Failed to get logs for the cluster node %s: %v\n", nodeName, err)
		}
	}
}
