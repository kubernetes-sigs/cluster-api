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
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ValidateOwnerReferencesOnUpdate checks that expected owner references are updated to the correct apiVersion.
func ValidateOwnerReferencesOnUpdate(proxy ClusterProxy, namespace string, assertFuncs ...map[string]func(reference []metav1.OwnerReference) error) {
	// Check that the ownerReferences are as expected on the first iteration.
	AssertOwnerReferences(namespace, proxy.GetKubeconfigPath(), assertFuncs...)
}

// ValidateOwnerReferencesResilience checks that expected owner references are in place, deletes them, and verifies that expect owner references are properly rebuilt.
func ValidateOwnerReferencesResilience(ctx context.Context, proxy ClusterProxy, namespace, clusterName string, assertFuncs ...map[string]func(reference []metav1.OwnerReference) error) {
	// Check that the ownerReferences are as expected on the first iteration.
	AssertOwnerReferences(namespace, proxy.GetKubeconfigPath(), assertFuncs...)

	clusterKey := client.ObjectKey{Namespace: namespace, Name: clusterName}

	// Removes all the owner references.
	// NOTE: we are testing the worst-case scenario, where all the owner references are removed while the
	// reconcilers are not working; this mimics what happens with the current velero backup/restore, which is an
	// edge case where an external system intentionally nukes all the owner references in a single operation.
	// The assumption is that if the system can recover from this edge case, it can also handle use cases where an owner
	// reference is deleted by mistake.
	setClusterPause(ctx, proxy.GetClient(), clusterKey, true)

	// Once all Clusters are paused remove the OwnerReference from all objects in the graph.
	removeOwnerReferences(ctx, proxy, namespace)

	setClusterPause(ctx, proxy.GetClient(), clusterKey, false)

	// Annotate the clusterClass, if one is in use, to speed up reconciliation. This ensures ClusterClass ownerReferences
	// are re-reconciled before asserting the owner reference graph.
	forceClusterClassReconcile(ctx, proxy.GetClient(), clusterKey)

	// Check that the ownerReferences are as expected after additional reconciliations.
	AssertOwnerReferences(namespace, proxy.GetKubeconfigPath(), assertFuncs...)
}

func AssertOwnerReferences(namespace, kubeconfigPath string, assertFuncs ...map[string]func(reference []metav1.OwnerReference) error) {
	allAssertFuncs := map[string]func(reference []metav1.OwnerReference) error{}
	for _, m := range assertFuncs {
		for k, v := range m {
			allAssertFuncs[k] = v
		}
	}
	Eventually(func() error {
		allErrs := []error{}
		graph, err := clusterctlcluster.GetOwnerGraph(namespace, kubeconfigPath)
		Expect(err).To(BeNil())
		for _, v := range graph {
			if _, ok := allAssertFuncs[v.Object.Kind]; !ok {
				allErrs = append(allErrs, fmt.Errorf("kind %s does not have an associated ownerRef assertion function", v.Object.Kind))
				continue
			}
			if err := allAssertFuncs[v.Object.Kind](v.Owners); err != nil {
				allErrs = append(allErrs, errors.Wrapf(err, "Unexpected ownerReferences for %s/%s", v.Object.Kind, v.Object.Name))
			}
		}
		return kerrors.NewAggregate(allErrs)
	}).WithTimeout(5 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
}

// Kind and GVK for types in the core API package.
var (
	extensionConfigKind    = "ExtensionConfig"
	clusterClassKind       = "ClusterClass"
	clusterKind            = "Cluster"
	machineKind            = "Machine"
	machineSetKind         = "MachineSet"
	machineDeploymentKind  = "MachineDeployment"
	machineHealthCheckKind = "MachineHealthCheck"

	clusterClassGVK      = clusterv1.GroupVersion.WithKind(clusterClassKind)
	clusterGVK           = clusterv1.GroupVersion.WithKind(clusterKind)
	machineDeploymentGVK = clusterv1.GroupVersion.WithKind(machineDeploymentKind)
	machineSetGVK        = clusterv1.GroupVersion.WithKind(machineSetKind)
	machineGVK           = clusterv1.GroupVersion.WithKind(machineKind)
)

// CoreOwnerReferenceAssertion maps Cluster API core types to functions which return an error if the passed
// OwnerReferences aren't as expected.
var CoreOwnerReferenceAssertion = map[string]func([]metav1.OwnerReference) error{
	extensionConfigKind: func(owners []metav1.OwnerReference) error {
		// ExtensionConfig should have no owners.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	clusterClassKind: func(owners []metav1.OwnerReference) error {
		// ClusterClass doesn't have ownerReferences (it is a clusterctl move-hierarchy root).
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	clusterKind: func(owners []metav1.OwnerReference) error {
		// Cluster doesn't have ownerReferences (it is a clusterctl move-hierarchy root).
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	machineDeploymentKind: func(owners []metav1.OwnerReference) error {
		// MachineDeployments must be owned by a Cluster.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
	machineSetKind: func(owners []metav1.OwnerReference) error {
		// MachineSets must be owned by a MachineDeployments.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{machineDeploymentGVK})
	},
	machineKind: func(owners []metav1.OwnerReference) error {
		// Machines must be owned by a MachineSet or a KubeadmControlPlane, depending on if this Machine is part of a ControlPlane or not.
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{machineSetGVK}, []schema.GroupVersionKind{kubeadmControlPlaneGVK})
	},
	machineHealthCheckKind: func(owners []metav1.OwnerReference) error {
		// MachineHealthChecks must be owned by the Cluster.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
}

// Kind and GVK for types in the exp package.
var (
	clusterResourceSetKind        = "ClusterResourceSet"
	clusterResourceSetBindingKind = "ClusterResourceSetBinding"
	machinePoolKind               = "MachinePool"

	machinePoolGVK        = expv1.GroupVersion.WithKind(machinePoolKind)
	clusterResourceSetGVK = addonsv1.GroupVersion.WithKind(clusterResourceSetKind)
)

// ExpOwnerReferenceAssertions maps experimental types to functions which return an error if the passed OwnerReferences
// aren't as expected.
var ExpOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	clusterResourceSetKind: func(owners []metav1.OwnerReference) error {
		// ClusterResourcesSet doesn't have ownerReferences (it is a clusterctl move-hierarchy root).
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	// ClusterResourcesSetBinding has ClusterResourceSet set as owners on creation.
	clusterResourceSetBindingKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterResourceSetGVK})
	},
	// MachinePool must be owned by a Cluster.
	machinePoolKind: func(owners []metav1.OwnerReference) error {
		// MachinePools must be owned by a Cluster.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
}

var (
	configMapKind = "ConfigMap"
	secretKind    = "Secret"
)

// KubernetesReferenceAssertions maps Kubernetes types to functions which return an error if the passed OwnerReferences
// aren't as expected.
var KubernetesReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	secretKind: func(owners []metav1.OwnerReference) error {
		// Secrets for cluster certificates must be owned by the KubeadmControlPlane. The bootstrap secret should be owned by a KubeadmControlPlane.
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{kubeadmControlPlaneGVK}, []schema.GroupVersionKind{kubeadmConfigGVK})
	},
	configMapKind: func(owners []metav1.OwnerReference) error {
		// The only configMaps considered here are those owned by a ClusterResourceSet.
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterResourceSetGVK})
	},
}

// Kind and GVK for types in the Kubeadm ControlPlane package.
var (
	kubeadmControlPlaneKind         = "KubeadmControlPlane"
	kubeadmControlPlaneTemplateKind = "KubeadmControlPlaneTemplate"

	kubeadmControlPlaneGVK = controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind)
)

// KubeadmControlPlaneOwnerReferenceAssertions maps Kubeadm control plane types to functions which return an error if the passed
// OwnerReferences aren't as expected.
var KubeadmControlPlaneOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	kubeadmControlPlaneKind: func(owners []metav1.OwnerReference) error {
		// The KubeadmControlPlane must be owned by a Cluster.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
	kubeadmControlPlaneTemplateKind: func(owners []metav1.OwnerReference) error {
		// The KubeadmControlPlaneTemplate must be owned by a ClusterClass.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterClassGVK})
	},
}

// Kind and GVK for types in the Kubeadm Bootstrap package.
var (
	kubeadmConfigKind         = "KubeadmConfig"
	kubeadmConfigTemplateKind = "KubeadmConfigTemplate"

	kubeadmConfigGVK = bootstrapv1.GroupVersion.WithKind(kubeadmConfigKind)
)

// KubeadmBootstrapOwnerReferenceAssertions maps KubeadmBootstrap types to functions which return an error if the passed OwnerReferences
// aren't as expected.
var KubeadmBootstrapOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	kubeadmConfigKind: func(owners []metav1.OwnerReference) error {
		// The KubeadmConfig must be owned by a Cluster or by a MachinePool.
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{machineGVK}, []schema.GroupVersionKind{machinePoolGVK})
	},
	kubeadmConfigTemplateKind: func(owners []metav1.OwnerReference) error {
		// The KubeadmConfigTemplate must be owned by a ClusterClass.
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK}, []schema.GroupVersionKind{clusterClassGVK})
	},
}

// Kind and GVK for types in the Docker infrastructure package.
var (
	dockerMachineKind         = "DockerMachine"
	dockerMachineTemplateKind = "DockerMachineTemplate"
	dockerMachinePoolKind     = "DockerMachinePool"
	dockerClusterKind         = "DockerCluster"
	dockerClusterTemplateKind = "DockerClusterTemplate"
)

// DockerInfraOwnerReferenceAssertions maps Docker Infrastructure types to functions which return an error if the passed
// OwnerReferences aren't as expected.
var DockerInfraOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	dockerMachineKind: func(owners []metav1.OwnerReference) error {
		// The DockerMachine must be owned by a Machine.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{machineGVK})
	},
	dockerMachineTemplateKind: func(owners []metav1.OwnerReference) error {
		// Base DockerMachineTemplates referenced in a ClusterClass must be owned by the ClusterClass.
		// DockerMachineTemplates created for specific Clusters in the Topology controller must be owned by a Cluster.
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK}, []schema.GroupVersionKind{clusterClassGVK})
	},
	dockerClusterKind: func(owners []metav1.OwnerReference) error {
		// DockerCluster must be owned by a Cluster.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
	dockerClusterTemplateKind: func(owners []metav1.OwnerReference) error {
		// DockerClusterTemplate must be owned by a ClusterClass.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterClassGVK})
	},
	dockerMachinePoolKind: func(owners []metav1.OwnerReference) error {
		// DockerMachinePool must be owned by a MachinePool.
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{machinePoolGVK})
	},
}

func hasExactOwnersByGVK(refList []metav1.OwnerReference, wantGVKs []schema.GroupVersionKind) error {
	refGVKs := []schema.GroupVersionKind{}
	for _, ref := range refList {
		refGVK, err := ownerRefGVK(ref)
		if err != nil {
			return err
		}
		refGVKs = append(refGVKs, refGVK)
	}
	sort.SliceStable(refGVKs, func(i int, j int) bool {
		return refGVKs[i].String() > refGVKs[j].String()
	})
	sort.SliceStable(wantGVKs, func(i int, j int) bool {
		return wantGVKs[i].String() > wantGVKs[j].String()
	})
	if !reflect.DeepEqual(wantGVKs, refGVKs) {
		return fmt.Errorf("wanted %v, actual %v", wantGVKs, refGVKs)
	}
	return nil
}

// NOTE: we are using hasOneOfExactOwnersByGVK as a convenience approach for checking owner references on objects that
// can have different owner references depending on the cluster topology.
// In a follow-up iteration we can make improvements to check owner references according to the specific use cases vs checking generically "oneOf".
func hasOneOfExactOwnersByGVK(refList []metav1.OwnerReference, possibleGVKS ...[]schema.GroupVersionKind) error {
	var allErrs []error
	for _, wantGVK := range possibleGVKS {
		err := hasExactOwnersByGVK(refList, wantGVK)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		return nil
	}
	return kerrors.NewAggregate(allErrs)
}

func ownerRefGVK(ref metav1.OwnerReference) (schema.GroupVersionKind, error) {
	refGV, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return schema.GroupVersionKind{Version: refGV.Version, Group: refGV.Group, Kind: ref.Kind}, nil
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

	if cluster.Spec.Topology != nil {
		class := &clusterv1.ClusterClass{}
		Expect(cli.Get(ctx, client.ObjectKey{Namespace: clusterKey.Namespace, Name: cluster.Spec.Topology.Class}, class)).To(Succeed())
		annotationPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/modifiedAt\":\"%v\"}}}", time.Now().Format(time.RFC3339))))
		Expect(cli.Patch(ctx, class, annotationPatch)).To(Succeed())
	}
}

func removeOwnerReferences(ctx context.Context, proxy ClusterProxy, namespace string) {
	graph, err := clusterctlcluster.GetOwnerGraph(namespace, proxy.GetKubeconfigPath())
	Expect(err).To(BeNil())
	for _, object := range graph {
		ref := object.Object
		obj := new(unstructured.Unstructured)
		obj.SetAPIVersion(ref.APIVersion)
		obj.SetKind(ref.Kind)
		obj.SetName(ref.Name)

		Expect(proxy.GetClient().Get(ctx, client.ObjectKey{Namespace: namespace, Name: object.Object.Name}, obj)).To(Succeed())
		helper, err := patch.NewHelper(obj, proxy.GetClient())
		Expect(err).To(BeNil())
		obj.SetOwnerReferences([]metav1.OwnerReference{})
		Expect(helper.Patch(ctx, obj)).To(Succeed())
	}
}
