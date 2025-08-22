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
	"sort"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ValidateOwnerReferencesOnUpdate checks that expected owner references are updated to the correct apiVersion.
func ValidateOwnerReferencesOnUpdate(ctx context.Context, proxy ClusterProxy, namespace, clusterName string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction, assertFuncs ...map[string]func(obj types.NamespacedName, reference []metav1.OwnerReference) error) {
	clusterKey := client.ObjectKey{Namespace: namespace, Name: clusterName}

	byf("Changing all the ownerReferences to a different API version")

	// Pause the cluster.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, true)

	// Change the version of the OwnerReferences on each object in the Graph to "v1alpha1"
	changeOwnerReferencesAPIVersion(ctx, proxy, namespace, ownerGraphFilterFunction)

	// Unpause the cluster.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, false)

	// Force ClusterClass reconciliation. This ensures ClusterClass ownerReferences  are re-reconciled before asserting
	// the owner reference graph.
	forceClusterClassReconcile(ctx, proxy.GetClient(), clusterKey)

	// Force ClusterResourceSet reconciliation. This ensures ClusterResourceBinding ownerReferences are re-reconciled before
	// asserting the owner reference graph.
	forceClusterResourceSetReconcile(ctx, proxy.GetClient(), namespace)

	// Check that the ownerReferences have updated their apiVersions to current versions after reconciliation.
	byf("Check that the ownerReferences are rebuilt as expected")
	AssertOwnerReferences(namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction, assertFuncs...)
}

// ValidateOwnerReferencesResilience checks that expected owner references are in place, deletes them, and verifies that expect owner references are properly rebuilt.
func ValidateOwnerReferencesResilience(ctx context.Context, proxy ClusterProxy, namespace, clusterName string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction, assertFuncs ...map[string]func(obj types.NamespacedName, reference []metav1.OwnerReference) error) {
	// Check that the ownerReferences are as expected on the first iteration.
	byf("Check that the ownerReferences are as expected")
	AssertOwnerReferences(namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction, assertFuncs...)

	clusterKey := client.ObjectKey{Namespace: namespace, Name: clusterName}

	// Removes all the owner references.
	// NOTE: we are testing the worst-case scenario, where all the owner references are removed while the
	// reconcilers are not working; this mimics what happens with the current velero backup/restore, which is an
	// edge case where an external system intentionally nukes all the owner references in a single operation.
	// The assumption is that if the system can recover from this edge case, it can also handle use cases where an owner
	// reference is deleted by mistake.
	byf("Removing all the ownerReferences")

	// Setting the paused property on the Cluster resource will pause reconciliations, thereby having no effect on OwnerReferences.
	// This also makes debugging easier.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, true)

	// Once all Clusters are paused remove the OwnerReference from all objects in the graph.
	removeOwnerReferences(ctx, proxy, namespace, ownerGraphFilterFunction)

	// Unpause the cluster.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, false)

	// Annotate the ClusterClass, if one is in use, to speed up reconciliation. This ensures ClusterClass ownerReferences
	// are re-reconciled before asserting the owner reference graph.
	forceClusterClassReconcile(ctx, proxy.GetClient(), clusterKey)

	// Check that the ownerReferences are as expected after additional reconciliations.
	byf("Check that the ownerReferences are rebuilt as expected")
	AssertOwnerReferences(namespace, proxy.GetKubeconfigPath(), ownerGraphFilterFunction, assertFuncs...)
}

func AssertOwnerReferences(namespace, kubeconfigPath string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction, assertFuncs ...map[string]func(obj types.NamespacedName, reference []metav1.OwnerReference) error) {
	allAssertFuncs := map[string][]func(obj types.NamespacedName, reference []metav1.OwnerReference) error{}
	for _, m := range assertFuncs {
		for k, v := range m {
			allAssertFuncs[k] = append(allAssertFuncs[k], v)
		}
	}
	Eventually(func() error {
		allErrs := []error{}
		ctx := context.Background()

		graph, err := clusterctlcluster.GetOwnerGraph(ctx, namespace, kubeconfigPath, ownerGraphFilterFunction)
		if err != nil {
			return err
		}

		for _, v := range graph {
			if _, ok := allAssertFuncs[v.Object.Kind]; !ok {
				allErrs = append(allErrs, fmt.Errorf("kind %s does not have an associated ownerRef assertion function", v.Object.Kind))
				continue
			}
			for _, f := range allAssertFuncs[v.Object.Kind] {
				if err := f(types.NamespacedName{Namespace: v.Object.Namespace, Name: v.Object.Name}, v.Owners); err != nil {
					allErrs = append(allErrs, errors.Wrapf(err, "unexpected ownerReferences for %s, %s", v.Object.Kind, klog.KRef(v.Object.Namespace, v.Object.Name)))
				}
			}
		}
		return kerrors.NewAggregate(allErrs)
	}).WithTimeout(5 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
}

// Kinds and Owners for types in the core API package.
var (
	coreGroupVersion = clusterv1.GroupVersion.String()

	extensionConfigKind    = "ExtensionConfig"
	clusterClassKind       = "ClusterClass"
	clusterKind            = "Cluster"
	machineKind            = "Machine"
	machineSetKind         = "MachineSet"
	machineDeploymentKind  = "MachineDeployment"
	machineHealthCheckKind = "MachineHealthCheck"

	clusterOwner                = metav1.OwnerReference{Kind: clusterKind, APIVersion: coreGroupVersion}
	clusterController           = metav1.OwnerReference{Kind: clusterKind, APIVersion: coreGroupVersion, Controller: ptr.To(true)}
	clusterClassOwner           = metav1.OwnerReference{Kind: clusterClassKind, APIVersion: coreGroupVersion}
	machineDeploymentController = metav1.OwnerReference{Kind: machineDeploymentKind, APIVersion: coreGroupVersion, Controller: ptr.To(true)}
	machineSetController        = metav1.OwnerReference{Kind: machineSetKind, APIVersion: coreGroupVersion, Controller: ptr.To(true)}
	machineController           = metav1.OwnerReference{Kind: machineKind, APIVersion: coreGroupVersion, Controller: ptr.To(true)}
)

// CoreOwnerReferenceAssertion maps Cluster API core types to functions which return an error if the passed
// OwnerReferences aren't as expected.
// Note: These relationships are documented in https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src/reference/owner_references.md.
// That document should be updated if these references change.
var CoreOwnerReferenceAssertion = map[string]func(types.NamespacedName, []metav1.OwnerReference) error{
	extensionConfigKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// ExtensionConfig should have no owners.
		return HasExactOwners(owners)
	},
	clusterClassKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// ClusterClass doesn't have ownerReferences (it is a clusterctl move-hierarchy root).
		return HasExactOwners(owners)
	},
	clusterKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// Cluster doesn't have ownerReferences (it is a clusterctl move-hierarchy root).
		return HasExactOwners(owners)
	},
	machineDeploymentKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// MachineDeployments must be owned by a Cluster.
		return HasExactOwners(owners, clusterOwner)
	},
	machineSetKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// MachineSets must be owned and controlled by a MachineDeployment.
		return HasExactOwners(owners, machineDeploymentController)
	},
	machineKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// Machines must be owned and controlled by a MachineSet, MachinePool, or a KubeadmControlPlane, depending on if this Machine is part of a Machine Deployment, MachinePool, or ControlPlane.
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{machineSetController}, []metav1.OwnerReference{machinePoolController}, []metav1.OwnerReference{kubeadmControlPlaneController})
	},
	machineHealthCheckKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// MachineHealthChecks must be owned by the Cluster.
		return HasExactOwners(owners, clusterOwner)
	},
}

// Kinds and Owners for types in the exp package.
var (
	clusterResourceSetKind        = "ClusterResourceSet"
	clusterResourceSetBindingKind = "ClusterResourceSetBinding"
	machinePoolKind               = "MachinePool"

	machinePoolController = metav1.OwnerReference{Kind: machinePoolKind, APIVersion: clusterv1.GroupVersion.String(), Controller: ptr.To(true)}

	clusterResourceSetOwner = metav1.OwnerReference{Kind: clusterResourceSetKind, APIVersion: addonsv1.GroupVersion.String()}
)

// ExpOwnerReferenceAssertions maps experimental types to functions which return an error if the passed OwnerReferences
// aren't as expected.
// Note: These relationships are documented in https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src/reference/owner_references.md.
// That document should be updated if these references change.
var ExpOwnerReferenceAssertions = map[string]func(types.NamespacedName, []metav1.OwnerReference) error{
	clusterResourceSetKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// ClusterResourcesSet doesn't have ownerReferences (it is a clusterctl move-hierarchy root).
		return HasExactOwners(owners)
	},
	// ClusterResourcesSetBinding has ClusterResourceSet set as owners on creation.
	clusterResourceSetBindingKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{clusterResourceSetOwner}, []metav1.OwnerReference{clusterResourceSetOwner, clusterResourceSetOwner})
	},
	// MachinePool must be owned by a Cluster.
	machinePoolKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// MachinePools must be owned by a Cluster.
		return HasExactOwners(owners, clusterOwner)
	},
}

var (
	configMapKind = "ConfigMap"
	secretKind    = "Secret"
)

// KubernetesReferenceAssertions maps Kubernetes types to functions which return an error if the passed OwnerReferences
// aren't as expected.
// Note: These relationships are documented in https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src/reference/owner_references.md.
// That document should be updated if these references change.
var KubernetesReferenceAssertions = map[string]func(types.NamespacedName, []metav1.OwnerReference) error{
	secretKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// Secrets for cluster certificates must be owned and controlled by the KubeadmControlPlane. The bootstrap secret should be owned and controlled by a KubeadmControlPlane.
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{kubeadmControlPlaneController}, []metav1.OwnerReference{kubeadmConfigController})
	},
	configMapKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// The only configMaps considered here are those owned by a ClusterResourceSet.
		return HasExactOwners(owners, clusterResourceSetOwner)
	},
}

// Kind and Owners for types in the Kubeadm ControlPlane package.
var (
	kubeadmControlPlaneKind         = "KubeadmControlPlane"
	kubeadmControlPlaneTemplateKind = "KubeadmControlPlaneTemplate"

	kubeadmControlPlaneGroupVersion = controlplanev1.GroupVersion.String()

	kubeadmControlPlaneController = metav1.OwnerReference{Kind: kubeadmControlPlaneKind, APIVersion: kubeadmControlPlaneGroupVersion, Controller: ptr.To(true)}
)

// KubeadmControlPlaneOwnerReferenceAssertions maps Kubeadm control plane types to functions which return an error if the passed
// OwnerReferences aren't as expected.
// Note: These relationships are documented in https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src/reference/owner_references.md.
// That document should be updated if these references change.
var KubeadmControlPlaneOwnerReferenceAssertions = map[string]func(types.NamespacedName, []metav1.OwnerReference) error{
	kubeadmControlPlaneKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// The KubeadmControlPlane must be owned and controlled by a Cluster.
		return HasExactOwners(owners, clusterController)
	},
	kubeadmControlPlaneTemplateKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// The KubeadmControlPlaneTemplate must be owned by a ClusterClass.
		return HasExactOwners(owners, clusterClassOwner)
	},
}

// Owners and kinds for types in the Kubeadm Bootstrap package.
var (
	kubeadmConfigKind         = "KubeadmConfig"
	kubeadmConfigTemplateKind = "KubeadmConfigTemplate"

	kubeadmConfigGroupVersion = bootstrapv1.GroupVersion.String()
	kubeadmConfigController   = metav1.OwnerReference{Kind: kubeadmConfigKind, APIVersion: kubeadmConfigGroupVersion, Controller: ptr.To(true)}
)

// KubeadmBootstrapOwnerReferenceAssertions maps KubeadmBootstrap types to functions which return an error if the passed OwnerReferences
// aren't as expected.
// Note: These relationships are documented in https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src/reference/owner_references.md.
// That document should be updated if these references change.
var KubeadmBootstrapOwnerReferenceAssertions = map[string]func(types.NamespacedName, []metav1.OwnerReference) error{
	kubeadmConfigKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// The KubeadmConfig must be owned and controlled by a Machine or MachinePool.
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{machineController}, []metav1.OwnerReference{machinePoolController, clusterOwner})
	},
	kubeadmConfigTemplateKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// The KubeadmConfigTemplate must be owned by a ClusterClass.
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{clusterOwner}, []metav1.OwnerReference{clusterClassOwner})
	},
}

// Kinds for types in the Docker infrastructure package.
var (
	dockerMachineKind             = "DockerMachine"
	dockerMachineTemplateKind     = "DockerMachineTemplate"
	dockerMachinePoolKind         = "DockerMachinePool"
	dockerMachinePoolTemplateKind = "DockerMachinePoolTemplate"
	dockerClusterKind             = "DockerCluster"
	dockerClusterTemplateKind     = "DockerClusterTemplate"

	dockerMachinePoolController = metav1.OwnerReference{Kind: dockerMachinePoolKind, APIVersion: infraexpv1.GroupVersion.String()}
)

// DockerInfraOwnerReferenceAssertions maps Docker Infrastructure types to functions which return an error if the passed
// OwnerReferences aren't as expected.
// Note: These relationships are documented in https://github.com/kubernetes-sigs/cluster-api/tree/main/docs/book/src/reference/owner_references.md.
// That document should be updated if these references change.
var DockerInfraOwnerReferenceAssertions = map[string]func(types.NamespacedName, []metav1.OwnerReference) error{
	dockerMachineKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// The DockerMachine must be owned and controlled by a Machine or a DockerMachinePool.
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{machineController}, []metav1.OwnerReference{machineController, dockerMachinePoolController})
	},
	dockerMachineTemplateKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// Base DockerMachineTemplates referenced in a ClusterClass must be owned by the ClusterClass.
		// DockerMachineTemplates created for specific Clusters in the Topology controller must be owned by a Cluster.
		return HasOneOfExactOwners(owners, []metav1.OwnerReference{clusterOwner}, []metav1.OwnerReference{clusterClassOwner})
	},
	dockerClusterKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// DockerCluster must be owned and controlled by a Cluster.
		return HasExactOwners(owners, clusterController)
	},
	dockerClusterTemplateKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// DockerClusterTemplate must be owned by a ClusterClass.
		return HasExactOwners(owners, clusterClassOwner)
	},
	dockerMachinePoolKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// DockerMachinePool must be owned and controlled by a MachinePool.
		return HasExactOwners(owners, machinePoolController, clusterOwner)
	},
	dockerMachinePoolTemplateKind: func(_ types.NamespacedName, owners []metav1.OwnerReference) error {
		// DockerMachinePoolTemplate must be owned by a ClusterClass.
		return HasExactOwners(owners, clusterClassOwner)
	},
}

func HasExactOwners(gotOwners []metav1.OwnerReference, wantOwners ...metav1.OwnerReference) error {
	wantComparable := []string{}
	gotComparable := []string{}
	for _, ref := range gotOwners {
		gotComparable = append(gotComparable, ownerReferenceString(ref))
	}
	for _, ref := range wantOwners {
		wantComparable = append(wantComparable, ownerReferenceString(ref))
	}
	sort.Strings(gotComparable)
	sort.Strings(wantComparable)

	if !reflect.DeepEqual(gotComparable, wantComparable) {
		return fmt.Errorf("wanted %v, actual %v", wantComparable, gotComparable)
	}
	return nil
}

func ownerReferenceString(ref metav1.OwnerReference) string {
	var controller bool
	if ref.Controller != nil && *ref.Controller {
		controller = true
	}
	return fmt.Sprintf("%s/%s/%v", ref.APIVersion, ref.Kind, controller)
}

// HasOneOfExactOwners is a convenience approach for checking owner references on objects that can have different owner references depending on the cluster.
// In a follow-up iteration we can make improvements to check owner references according to the specific use cases vs checking generically "oneOf".
func HasOneOfExactOwners(refList []metav1.OwnerReference, possibleOwners ...[]metav1.OwnerReference) error {
	var allErrs []error
	for _, wantOwner := range possibleOwners {
		err := HasExactOwners(refList, wantOwner...)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		return nil
	}
	return kerrors.NewAggregate(allErrs)
}

func setClusterPause(ctx context.Context, cli client.Client, clusterKey types.NamespacedName, value bool) {
	cluster := &clusterv1.Cluster{}
	Expect(cli.Get(ctx, clusterKey, cluster)).To(Succeed())

	pausePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%v}}", value)))
	Expect(cli.Patch(ctx, cluster, pausePatch)).To(Succeed())
}

// forceClusterClassReconcile force reconciliation of the ClusterClass associated with the Cluster if one exists. If the
// Cluster has no ClusterClass this is a no-op.
func forceClusterClassReconcile(ctx context.Context, cli client.Client, clusterKey types.NamespacedName) {
	cluster := &clusterv1.Cluster{}
	Expect(cli.Get(ctx, clusterKey, cluster)).To(Succeed())

	if cluster.Spec.Topology.IsDefined() {
		class := &clusterv1.ClusterClass{}
		Expect(cli.Get(ctx, cluster.GetClassKey(), class)).To(Succeed())
		annotationPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/modifiedAt\":\"%v\"}}}", time.Now().Format(time.RFC3339))))
		Expect(cli.Patch(ctx, class, annotationPatch)).To(Succeed())
	}
}

// forceClusterResourceSetReconcile forces reconciliation of all ClusterResourceSets.
func forceClusterResourceSetReconcile(ctx context.Context, cli client.Client, namespace string) {
	crsList := &addonsv1.ClusterResourceSetList{}
	Expect(cli.List(ctx, crsList, client.InNamespace(namespace))).To(Succeed())
	for _, crs := range crsList.Items {
		annotationPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/modifiedAt\":\"%v\"}}}", time.Now().Format(time.RFC3339))))
		Expect(cli.Patch(ctx, crs.DeepCopy(), annotationPatch)).To(Succeed())
	}
}

func changeOwnerReferencesAPIVersion(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
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

		newOwners := []metav1.OwnerReference{}
		for _, owner := range obj.GetOwnerReferences() {
			gv, err := schema.ParseGroupVersion(owner.APIVersion)
			Expect(err).To(Succeed())
			gv.Version = "v1alpha1"
			owner.APIVersion = gv.String()
			newOwners = append(newOwners, owner)
		}

		obj.SetOwnerReferences(newOwners)
		Expect(helper.Patch(ctx, obj)).To(Succeed())
	}
}

func removeOwnerReferences(ctx context.Context, proxy ClusterProxy, namespace string, ownerGraphFilterFunction clusterctlcluster.GetOwnerGraphFilterFunction) {
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
		obj.SetOwnerReferences([]metav1.OwnerReference{})
		Expect(helper.Patch(ctx, obj)).To(Succeed())
	}
}
