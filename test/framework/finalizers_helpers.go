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

package framework

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

// CoreFinalizersAssertionWithLegacyClusters maps Cluster API core types to their expected finalizers for legacy Clusters.
var CoreFinalizersAssertionWithLegacyClusters = map[string]func(types.NamespacedName) []string{
	clusterKind: func(_ types.NamespacedName) []string { return []string{clusterv1.ClusterFinalizer} },
	machineKind: func(_ types.NamespacedName) []string { return []string{clusterv1.MachineFinalizer} },
}

// CoreFinalizersAssertionWithClassyClusters maps Cluster API core types to their expected finalizers for classy Clusters.
var CoreFinalizersAssertionWithClassyClusters = func() map[string]func(types.NamespacedName) []string {
	r := map[string]func(types.NamespacedName) []string{}
	for k, v := range CoreFinalizersAssertionWithLegacyClusters {
		r[k] = v
	}
	r[machineSetKind] = func(_ types.NamespacedName) []string { return []string{clusterv1.MachineSetTopologyFinalizer} }
	r[machineDeploymentKind] = func(_ types.NamespacedName) []string { return []string{clusterv1.MachineDeploymentTopologyFinalizer} }
	return r
}()

// ExpFinalizersAssertion maps experimental resource types to their expected finalizers.
var ExpFinalizersAssertion = map[string]func(types.NamespacedName) []string{
	clusterResourceSetKind: func(_ types.NamespacedName) []string { return []string{addonsv1.ClusterResourceSetFinalizer} },
	machinePoolKind:        func(_ types.NamespacedName) []string { return []string{expv1.MachinePoolFinalizer} },
}

// DockerInfraFinalizersAssertion maps docker infrastructure resource types to their expected finalizers.
var DockerInfraFinalizersAssertion = map[string]func(types.NamespacedName) []string{
	dockerMachineKind:     func(_ types.NamespacedName) []string { return []string{infrav1.MachineFinalizer} },
	dockerClusterKind:     func(_ types.NamespacedName) []string { return []string{infrav1.ClusterFinalizer} },
	dockerMachinePoolKind: func(_ types.NamespacedName) []string { return []string{infraexpv1.MachinePoolFinalizer} },
}

// KubeadmControlPlaneFinalizersAssertion maps Kubeadm resource types to their expected finalizers.
var KubeadmControlPlaneFinalizersAssertion = map[string]func(types.NamespacedName) []string{
	kubeadmControlPlaneKind: func(_ types.NamespacedName) []string { return []string{controlplanev1.KubeadmControlPlaneFinalizer} },
}

// ValidateFinalizersResilience checks that expected finalizers are in place, deletes them, and verifies that expected finalizers are properly added again.
func ValidateFinalizersResilience(ctx context.Context, proxy ClusterProxy, namespace, clusterName string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction, finalizerAssertions ...map[string]func(name types.NamespacedName) []string) {
	clusterKey := client.ObjectKey{Namespace: namespace, Name: clusterName}
	allFinalizerAssertions, err := concatenateFinalizerAssertions(finalizerAssertions...)
	Expect(err).ToNot(HaveOccurred())

	// Collect all objects where finalizers were initially set
	byf("Check that the finalizers are as expected")
	err = checkObjectsWithFinalizers(ctx, proxy, namespace, allFinalizerAssertions, ownerGraphFilterFunction)
	Expect(err).ToNot(HaveOccurred(), "Finalizers are not as expected")

	byf("Removing all the finalizers")
	// Setting the paused property on the Cluster resource will pause reconciliations, thereby having no effect on Finalizers.
	// This also makes debugging easier.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, true)

	// We are testing the worst-case scenario, i.e. all finalizers are deleted.
	// Once all Clusters are paused remove all the Finalizers from all objects in the graph.
	// The reconciliation loop should be able to recover from this, by adding the required Finalizers back.
	removeFinalizers(ctx, proxy, namespace, ownerGraphFilterFunction)

	// Unpause the cluster.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, false)

	// Check that the Finalizers are as expected after further reconciliations.
	byf("Check that the finalizers are rebuilt as expected")
	Eventually(func() error {
		return checkObjectsWithFinalizers(ctx, proxy, namespace, allFinalizerAssertions, ownerGraphFilterFunction)
	}).WithTimeout(1*time.Minute).WithPolling(2*time.Second).Should(Succeed(), "Finalizers are not rebuilt as expected")
}

// removeFinalizers removes all Finalizers from objects in the owner graph.
func removeFinalizers(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
	Expect(err).ToNot(HaveOccurred())
	for _, object := range graph {
		ref := object.Object
		obj := new(unstructured.Unstructured)
		obj.SetAPIVersion(ref.APIVersion)
		obj.SetKind(ref.Kind)
		obj.SetName(ref.Name)

		Expect(proxy.GetClient().Get(ctx, client.ObjectKey{Namespace: namespace, Name: object.Object.Name}, obj)).To(Succeed())
		helper, err := patch.NewHelper(obj, proxy.GetClient())
		Expect(err).ToNot(HaveOccurred())
		obj.SetFinalizers([]string{})
		Expect(helper.Patch(ctx, obj)).To(Succeed())
	}
}

func checkObjectsWithFinalizers(ctx context.Context, proxy ClusterProxy, namespace string, allFinalizerAssertions map[string]func(name types.NamespacedName) []string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) error {
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
	if err != nil {
		return err
	}

	var allErrs []error
	for _, node := range graph {
		nodeNamespacedName := client.ObjectKey{Namespace: node.Object.Namespace, Name: node.Object.Name}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(node.Object.APIVersion)
		obj.SetKind(node.Object.Kind)
		err = proxy.GetClient().Get(ctx, nodeNamespacedName, obj)
		if err != nil {
			return errors.Wrapf(err, "failed to get object %s, %s", node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name))
		}

		// assert if the expected finalizers are set on the resource (including also checking if there are unexpected finalizers)
		setFinalizers := obj.GetFinalizers()
		var expectedFinalizers []string
		if assertion, ok := allFinalizerAssertions[node.Object.Kind]; ok {
			expectedFinalizers = assertion(types.NamespacedName{Namespace: node.Object.Namespace, Name: node.Object.Name})
		}

		if !sets.NewString(setFinalizers...).Equal(sets.NewString(expectedFinalizers...)) {
			allErrs = append(allErrs, fmt.Errorf("unexpected finalizers for %s, %s: expected: %v, found: %v",
				node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name), expectedFinalizers, setFinalizers))
		}
	}
	return kerrors.NewAggregate(allErrs)
}

// concatenateFinalizerAssertions concatenates all finalizer assertions into one map. It reports errors if assertions already exist.
func concatenateFinalizerAssertions(finalizerAssertions ...map[string]func(name types.NamespacedName) []string) (map[string]func(name types.NamespacedName) []string, error) {
	var allErrs []error
	allFinalizerAssertions := make(map[string]func(name types.NamespacedName) []string, 0)

	for i := range finalizerAssertions {
		for kind, finalizers := range finalizerAssertions[i] {
			if _, alreadyExists := allFinalizerAssertions[kind]; alreadyExists {
				allErrs = append(allErrs, fmt.Errorf("finalizer assertion cannot be applied as it already exists for kind: %s", kind))
				continue
			}

			allFinalizerAssertions[kind] = finalizers
		}
	}

	return allFinalizerAssertions, kerrors.NewAggregate(allErrs)
}
