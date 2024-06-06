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
	"strings"
	"time"

	. "github.com/onsi/gomega"
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
	objectsWithFinalizers := getObjectsWithFinalizers(ctx, proxy, namespace, allFinalizerAssertions, ownerGraphFilterFunction)

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
	assertFinalizersExist(ctx, proxy, namespace, objectsWithFinalizers, allFinalizerAssertions, ownerGraphFilterFunction)
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

func getObjectsWithFinalizers(ctx context.Context, proxy ClusterProxy, namespace string, allFinalizerAssertions map[string]func(name types.NamespacedName) []string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) map[string]*unstructured.Unstructured {
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
	Expect(err).ToNot(HaveOccurred())

	objsWithFinalizers := map[string]*unstructured.Unstructured{}

	for _, node := range graph {
		nodeNamespacedName := client.ObjectKey{Namespace: node.Object.Namespace, Name: node.Object.Name}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(node.Object.APIVersion)
		obj.SetKind(node.Object.Kind)
		err = proxy.GetClient().Get(ctx, nodeNamespacedName, obj)
		Expect(err).ToNot(HaveOccurred())

		// assert if the expected finalizers are set on the resource (including also checking if there are unexpected finalizers)
		setFinalizers := obj.GetFinalizers()
		var expectedFinalizers []string
		if assertion, ok := allFinalizerAssertions[node.Object.Kind]; ok {
			expectedFinalizers = assertion(types.NamespacedName{Namespace: node.Object.Namespace, Name: node.Object.Name})
		}

		Expect(sets.NewString(setFinalizers...)).To(Equal(sets.NewString(expectedFinalizers...)), "for resource type %s, %s", node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name))
		if len(setFinalizers) > 0 {
			objsWithFinalizers[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj
		}
	}

	return objsWithFinalizers
}

// assertFinalizersExist ensures that current Finalizers match those in the initialObjectsWithFinalizers.
func assertFinalizersExist(ctx context.Context, proxy ClusterProxy, namespace string, initialObjsWithFinalizers map[string]*unstructured.Unstructured, allFinalizerAssertions map[string]func(name types.NamespacedName) []string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	Eventually(func() error {
		var allErrs []error
		finalObjsWithFinalizers := getObjectsWithFinalizers(ctx, proxy, namespace, allFinalizerAssertions, ownerGraphFilterFunction)

		// Check if all the initial objects with finalizers have them back.
		for objKindNamespacedName, obj := range initialObjsWithFinalizers {
			// verify if finalizers for this resource were set on reconcile
			if _, valid := finalObjsWithFinalizers[objKindNamespacedName]; !valid {
				allErrs = append(allErrs, fmt.Errorf("no finalizers set for %s, at the beginning of the test it has %s",
					objKindNamespacedName, obj.GetFinalizers()))
				continue
			}

			// verify if this resource has the appropriate Finalizers set
			expectedFinalizersF, assert := allFinalizerAssertions[obj.GetKind()]
			// NOTE: this case should never happen because all the initialObjsWithFinalizers have been already checked
			// against a finalizer assertion.
			Expect(assert).To(BeTrue(), "finalizer assertions for %s are missing", objKindNamespacedName)
			parts := strings.Split(objKindNamespacedName, "/")
			expectedFinalizers := expectedFinalizersF(types.NamespacedName{Namespace: parts[1], Name: parts[2]})

			setFinalizers := finalObjsWithFinalizers[objKindNamespacedName].GetFinalizers()
			if !sets.NewString(setFinalizers...).Equal(sets.NewString(expectedFinalizers...)) {
				allErrs = append(allErrs, fmt.Errorf("expected finalizers do not exist for %s: expected: %v, found: %v",
					objKindNamespacedName, expectedFinalizers, setFinalizers))
			}
		}

		// Check if there are objects with finalizers not existing initially
		for objKindNamespacedName, obj := range finalObjsWithFinalizers {
			// verify if finalizers for this resource were set on reconcile
			if _, valid := initialObjsWithFinalizers[objKindNamespacedName]; !valid {
				allErrs = append(allErrs, fmt.Errorf("%s has finalizers not existing at the beginning of the test: %s",
					objKindNamespacedName, obj.GetFinalizers()))
			}
		}

		return kerrors.NewAggregate(allErrs)
	}).WithTimeout(1 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
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
