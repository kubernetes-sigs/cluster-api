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
	"time"

	"github.com/blang/semver"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
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

func Byf(format string, a ...interface{}) {
	By(fmt.Sprintf(format, a...))
}

func setupSpecNamespace(ctx context.Context, specName string, clusterProxy framework.ClusterProxy, artifactFolder string) (*corev1.Namespace, context.CancelFunc) {
	Byf("Creating a namespace for hosting the %q test spec", specName)
	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   clusterProxy.GetClient(),
		ClientSet: clusterProxy.GetClientSet(),
		Name:      fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
		LogFolder: filepath.Join(artifactFolder, "clusters", clusterProxy.GetName()),
	})

	return namespace, cancelWatches
}

func dumpSpecResourcesAndCleanup(ctx context.Context, specName string, clusterProxy framework.ClusterProxy, artifactFolder string, namespace *corev1.Namespace, cancelWatches context.CancelFunc, cluster *clusterv1.Cluster, intervalsGetter func(spec, key string) []interface{}, skipCleanup bool) {
	Byf("Dumping logs from the %q workload cluster", cluster.Name)

	// Dump all the logs from the workload cluster before deleting them.
	clusterProxy.CollectWorkloadClusterLogs(ctx, cluster.Namespace, cluster.Name, filepath.Join(artifactFolder, "clusters", cluster.Name))

	Byf("Dumping all the Cluster API resources in the %q namespace", namespace.Name)

	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:    clusterProxy.GetClient(),
		Namespace: namespace.Name,
		LogPath:   filepath.Join(artifactFolder, "clusters", clusterProxy.GetName(), "resources"),
	})

	if !skipCleanup {
		i, j := InterfaceToDuration(intervalsGetter(specName, "wait-delete-cluster"))
		Byf("Deleting cluster %s", klog.KObj(cluster))
		// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
		// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
		// instead of DeleteClusterAndWait
		framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
			Client:    clusterProxy.GetClient(),
			Namespace: namespace.Name,
		}, i, j)

		i, j = InterfaceToDuration()
		Byf("Deleting namespace used for hosting the %q test spec", specName)
		framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
			Deleter: clusterProxy.GetClient(),
			Name:    namespace.Name,
		}, i, j)
	}
	cancelWatches()
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

func InterfaceToDuration(i ...interface{}) (time.Duration, time.Duration) {
	timeout := 1 * time.Hour
	polling := 6 * time.Minute

	if len(i) > 0 {
		switch i[0].(type) {
		case string:
			timeout, _ = time.ParseDuration(i[0].(string))
		default:
			fmt.Printf("%T, %v\n", i[0], i[0])
		}
	}
	if len(i) == 2 {
		switch i[1].(type) {
		case string:
			timeout, _ = time.ParseDuration(i[1].(string))
		default:
			fmt.Printf("%T, %v\n", i[1], i[1])
		}
	}
	return timeout, polling
}
