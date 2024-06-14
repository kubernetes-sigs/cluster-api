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
	"reflect"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
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

// CoreFinalizersAssertion maps Cluster API core types to their expected finalizers.
var CoreFinalizersAssertion = map[string][]string{
	"Cluster":           {clusterv1.ClusterFinalizer},
	"Machine":           {clusterv1.MachineFinalizer},
	"MachineSet":        {clusterv1.MachineSetTopologyFinalizer},
	"MachineDeployment": {clusterv1.MachineDeploymentTopologyFinalizer},
}

// ExpFinalizersAssertion maps experimental resource types to their expected finalizers.
var ExpFinalizersAssertion = map[string][]string{
	"ClusterResourceSet": {addonsv1.ClusterResourceSetFinalizer},
	"MachinePool":        {expv1.MachinePoolFinalizer},
}

// DockerInfraFinalizersAssertion maps docker infrastructure resource types to their expected finalizers.
var DockerInfraFinalizersAssertion = map[string][]string{
	"DockerMachine":     {infrav1.MachineFinalizer},
	"DockerCluster":     {infrav1.ClusterFinalizer},
	"DockerMachinePool": {infraexpv1.MachinePoolFinalizer},
}

// KubeadmControlPlaneFinalizersAssertion maps Kubeadm resource types to their expected finalizers.
var KubeadmControlPlaneFinalizersAssertion = map[string][]string{
	"KubeadmControlPlane": {controlplanev1.KubeadmControlPlaneFinalizer},
}

// ValidateFinalizersResilience checks that expected finalizers are in place, deletes them, and verifies that expected finalizers are properly added again.
func ValidateFinalizersResilience(ctx context.Context, proxy ClusterProxy, namespace, clusterName string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction, finalizerAssertions ...map[string][]string) {
	clusterKey := client.ObjectKey{Namespace: namespace, Name: clusterName}
	allFinalizerAssertions, err := concatenateFinalizerAssertions(finalizerAssertions...)
	Expect(err).ToNot(HaveOccurred())

	// Collect all objects where finalizers were initially set
	byf("Check that the finalizers are as expected")
	objectsWithFinalizers, err := getObjectsWithFinalizers(ctx, proxy, namespace, allFinalizerAssertions, ownerGraphFilterFunction)
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

func getObjectsWithFinalizers(ctx context.Context, proxy ClusterProxy, namespace string, allFinalizerAssertions map[string][]string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) (map[string]*unstructured.Unstructured, error) {
	graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction)
	if err != nil {
		return nil, err
	}

	var allErrs []error
	objsWithFinalizers := map[string]*unstructured.Unstructured{}

	for _, node := range graph {
		nodeNamespacedName := client.ObjectKey{Namespace: node.Object.Namespace, Name: node.Object.Name}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(node.Object.APIVersion)
		obj.SetKind(node.Object.Kind)
		err = proxy.GetClient().Get(ctx, nodeNamespacedName, obj)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get object %s, %s", node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name))
		}

		setFinalizers := obj.GetFinalizers()

		if len(setFinalizers) > 0 {
			// assert if the expected finalizers are set on the resource
			expectedFinalizers := allFinalizerAssertions[node.Object.Kind]
			if !reflect.DeepEqual(setFinalizers, expectedFinalizers) {
				allErrs = append(allErrs, fmt.Errorf("unexpected finalizers for %s, %s: expected: %v, found: %v",
					node.Object.Kind, klog.KRef(node.Object.Namespace, node.Object.Name), expectedFinalizers, setFinalizers))
			}
			objsWithFinalizers[fmt.Sprintf("%s/%s/%s", node.Object.Kind, node.Object.Namespace, node.Object.Name)] = obj
		}
	}

	return objsWithFinalizers, kerrors.NewAggregate(allErrs)
}

// assertFinalizersExist ensures that current Finalizers match those in the initialObjectsWithFinalizers.
func assertFinalizersExist(ctx context.Context, proxy ClusterProxy, namespace string, initialObjsWithFinalizers map[string]*unstructured.Unstructured, allFinalizerAssertions map[string][]string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
	Eventually(func() error {
		var allErrs []error
		finalObjsWithFinalizers, err := getObjectsWithFinalizers(ctx, proxy, namespace, allFinalizerAssertions, ownerGraphFilterFunction)
		if err != nil {
			return err
		}

		for objKindNamespacedName, obj := range initialObjsWithFinalizers {
			// verify if finalizers for this resource were set on reconcile
			if _, valid := finalObjsWithFinalizers[objKindNamespacedName]; !valid {
				allErrs = append(allErrs, fmt.Errorf("no finalizers set for %s",
					objKindNamespacedName))
				continue
			}

			// verify if this resource has the appropriate Finalizers set
			expectedFinalizers, assert := allFinalizerAssertions[obj.GetKind()]
			if !assert {
				continue
			}

			setFinalizers := finalObjsWithFinalizers[objKindNamespacedName].GetFinalizers()
			if !reflect.DeepEqual(expectedFinalizers, setFinalizers) {
				allErrs = append(allErrs, fmt.Errorf("expected finalizers do not exist for %s: expected: %v, found: %v",
					objKindNamespacedName, expectedFinalizers, setFinalizers))
			}
		}

		return kerrors.NewAggregate(allErrs)
	}).WithTimeout(1 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
}

// concatenateFinalizerAssertions concatenates all finalizer assertions into one map. It reports errors if assertions already exist.
func concatenateFinalizerAssertions(finalizerAssertions ...map[string][]string) (map[string][]string, error) {
	var allErrs []error
	allFinalizerAssertions := make(map[string][]string, 0)

	for i := range finalizerAssertions {
		for kind, finalizers := range finalizerAssertions[i] {
			if _, alreadyExists := allFinalizerAssertions[kind]; alreadyExists {
				allErrs = append(allErrs, fmt.Errorf("finalizer assertion cannot be applied as it already exists for kind: %s, existing value: %v, new value: %v",
					kind, allFinalizerAssertions[kind], finalizers))

				continue
			}

			allFinalizerAssertions[kind] = finalizers
		}
	}

	return allFinalizerAssertions, kerrors.NewAggregate(allErrs)
}
