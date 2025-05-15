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
	"time"

	"github.com/blang/semver/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
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

// verifyV1Beta2Conditions checks the Cluster and Machines of a Cluster that
// the given v1beta2 condition types are set to true without a message, if they exist.
func verifyV1Beta2Conditions(ctx context.Context, c client.Client, clusterName, clusterNamespace string, v1beta2conditionTypes map[string]struct{}) {
	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: clusterNamespace,
		Name:      clusterName,
	}
	Eventually(func() error {
		return c.Get(ctx, key, cluster)
	}, 3*time.Minute, 10*time.Second).Should(Succeed(), "Failed to get Cluster object %s", klog.KRef(clusterNamespace, clusterName))

	if len(cluster.Status.Conditions) > 0 {
		for _, condition := range cluster.Status.Conditions {
			if _, ok := v1beta2conditionTypes[condition.Type]; !ok {
				continue
			}
			Expect(condition.Status).To(Equal(metav1.ConditionTrue), "The v1beta2 condition %q on the Cluster should be set to true", condition.Type)
			Expect(condition.Message).To(BeEmpty(), "The v1beta2 condition %q on the Cluster should have an empty message", condition.Type)
		}
	}

	machineList := &clusterv1.MachineList{}
	Eventually(func() error {
		return c.List(ctx, machineList, client.InNamespace(clusterNamespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel: clusterName,
			})
	}, 3*time.Minute, 10*time.Second).Should(Succeed(), "Failed to list Machines for Cluster %s", klog.KObj(cluster))
	if len(cluster.Status.Conditions) > 0 {
		for _, machine := range machineList.Items {
			if len(machine.Status.Conditions) == 0 {
				continue
			}
			for _, condition := range machine.Status.Conditions {
				if _, ok := v1beta2conditionTypes[condition.Type]; !ok {
					continue
				}
				Expect(condition.Status).To(Equal(metav1.ConditionTrue), "The v1beta2 condition %q on the Machine %q should be set to true", condition.Type, machine.Name)
				Expect(condition.Message).To(BeEmpty(), "The v1beta2 condition %q on the Machine %q should have an empty message", condition.Type, machine.Name)
			}
		}
	}
}
