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
	"strings"

	. "github.com/onsi/ginkgo"

	"github.com/blang/semver"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util"
)

// Test suite constants for e2e config variables
const (
	KubernetesVersion            = "KUBERNETES_VERSION"
	CNIPath                      = "CNI"
	KubernetesVersionUpgradeFrom = "KUBERNETES_VERSION_UPGRADE_FROM"
	KubernetesVersionUpgradeTo   = "KUBERNETES_VERSION_UPGRADE_TO"
	EtcdVersionUpgradeTo         = "ETCD_VERSION_UPGRADE_TO"
	CoreDNSVersionUpgradeTo      = "COREDNS_VERSION_UPGRADE_TO"
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
	Byf("Dumping all the Cluster API resources in the %q namespace", namespace.Name)
	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:    clusterProxy.GetClient(),
		Namespace: namespace.Name,
		LogPath:   filepath.Join(artifactFolder, "clusters", clusterProxy.GetName(), "resources"),
	})

	if !skipCleanup {
		Byf("Deleting cluster %s/%s", cluster.Namespace, cluster.Name)
		// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
		// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
		// instead of DeleteClusterAndWait
		framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
			Client:    clusterProxy.GetClient(),
			Namespace: namespace.Name,
		}, intervalsGetter(specName, "wait-delete-cluster")...)

		Byf("Deleting namespace used for hosting the %q test spec", specName)
		framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
			Deleter: clusterProxy.GetClient(),
			Name:    namespace.Name,
		})
	}
	cancelWatches()
}

// HaveValidVersion succeeds if version is a valid semver version
func HaveValidVersion(version string) types.GomegaMatcher {
	return &validVersionMatcher{version: version}
}

type validVersionMatcher struct{ version string }

func (m *validVersionMatcher) Match(actual interface{}) (success bool, err error) {
	if _, err := semver.Parse(strings.TrimPrefix(strings.TrimSpace(m.version), "v")); err != nil {
		return false, err
	}
	return true, nil
}

func (m *validVersionMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s", m.version, " to be a valid version ")
}

func (m *validVersionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n%s\n%s", m.version, " not to be a valid version ")
}
