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
	"sigs.k8s.io/cluster-api/util/patch"
)

func RemoveOwnerReferences(ctx context.Context, cli client.Client, namespace string) error {
	graph, err := clusterctlcluster.GetOwnerGraph(namespace)
	if err != nil {
		return err
	}
	for _, object := range graph {
		// First pause each Cluster.
		if object.Object.Kind == clusterKind {
			c := &clusterv1.Cluster{}
			err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: object.Object.Name}, c)
			if err != nil {
				return err
			}
			defer func() {
				unpausePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%s}}", "false")))
				_ = cli.Patch(ctx, c, unpausePatch)
				// If the Cluster is topology managed label the ClusterClass to speed up its reconciliation of ownerRefs
				if c.Spec.Topology != nil {
					class := &clusterv1.ClusterClass{}
					_ = cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: c.Spec.Topology.Class}, class)
					annotationPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"metadata\":{\"annotations\":{\"cluster.x-k8s.io/modifiedAt\":\"%v\"}}}", time.Now().Format(time.RFC3339))))
					err = cli.Patch(ctx, class, annotationPatch)
					if err != nil {
						fmt.Printf(err.Error())
					}
				}
			}()
			pausePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"spec\":{\"paused\":%s}}", "true")))
			err = cli.Patch(ctx, c, pausePatch)
			if err != nil {
				return err
			}
		}
	}

	for _, object := range graph {
		ref := object.Object
		// Once all Clusters are paused remove the OwnerReference from all objects in the graph.
		obj := new(unstructured.Unstructured)
		obj.SetAPIVersion(ref.APIVersion)
		obj.SetKind(ref.Kind)
		obj.SetName(ref.Name)

		err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: object.Object.Name}, obj)
		if err != nil {
			return err
		}
		helper, err := patch.NewHelper(obj, cli)
		if err != nil {
			return err
		}
		obj.SetOwnerReferences([]metav1.OwnerReference{})
		err = helper.Patch(ctx, obj)
		if err != nil {
			return err
		}
	}
	return nil
}
func AssertOwnerReferences(namespace string, assertFuncs ...map[string]func(reference []metav1.OwnerReference) error) {
	allAssertFuncs := map[string]func(reference []metav1.OwnerReference) error{}
	for _, m := range assertFuncs {
		for k, v := range m {
			allAssertFuncs[k] = v
		}
	}
	graph, err := clusterctlcluster.GetOwnerGraph(namespace)
	Expect(err).To(BeNil())
	allErrs := []error{}
	for _, v := range graph {
		if _, ok := allAssertFuncs[v.Object.Kind]; !ok {
			allErrs = append(allErrs, errors.New(fmt.Sprintf("kind %s does not have an associated ownerRef assertion function", v.Object.Kind)))
			continue
		}
		if err := allAssertFuncs[v.Object.Kind](v.Owners); err != nil {
			allErrs = append(allErrs, errors.Wrapf(err, "Unexpected ownerReferences for %s/%s", v.Object.Kind, v.Object.Name))
		}
	}
	var errStrings string
	for _, err := range allErrs {
		errStrings = fmt.Sprintf("%s\n%s", errStrings, err.Error())
	}
	Expect(kerrors.NewAggregate(allErrs)).To(BeNil(), errStrings)
}

// Kind and GVK for types in the core API package.
var (
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

var CoreTypeOwnerReferenceAssertion = map[string]func([]metav1.OwnerReference) error{
	clusterClassKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	clusterKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	machineDeploymentKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
	machineSetKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{machineDeploymentGVK})
	},
	machineKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{machineSetGVK}, []schema.GroupVersionKind{kubeadmControlPlaneGVK})
	},
	machineHealthCheckKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
}

// Kind and GVK for types in the exp package.
var (
	clusterResourceSetKind        = "ClusterResourceSet"
	clusterResourceSetBindingKind = "ClusterResourceSetBinding"

	clusterResourceSetGVK = addonsv1.GroupVersion.WithKind(clusterResourceSetKind)
)
var ExpOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	clusterResourceSetKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{})
	},
	clusterResourceSetBindingKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK, clusterResourceSetGVK})
	},
}

var (
	configMapKind = "ConfigMap"
	secretKind    = "Secret"
)

var SecretOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	secretKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{kubeadmControlPlaneGVK}, []schema.GroupVersionKind{kubeadmConfigGVK})
	},
}
var ConfigMapReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	configMapKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterResourceSetGVK}, []schema.GroupVersionKind{})
	},
}

// Kind and GVK for types in the Kubeadm ControlPlane package.
var (
	kubeadmControlPlaneKind         = "KubeadmControlPlane"
	kubeadmControlPlaneTemplateKind = "KubeadmControlPlaneTemplate"

	kubeadmControlPlaneGVK = controlplanev1.GroupVersion.WithKind(kubeadmControlPlaneKind)
)
var KubeadmControlPlaneOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	kubeadmControlPlaneKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
	kubeadmControlPlaneTemplateKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterClassGVK})
	},
}

// Kind and GVK for types in the Kubeadm Bootstrap package.
var (
	kubeadmConfigKind         = "KubeadmConfig"
	kubeadmConfigTemplateKind = "KubeadmConfigTemplate"

	kubeadmConfigGVK = bootstrapv1.GroupVersion.WithKind(kubeadmConfigKind)
)

var KubeadmBootstrapOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	kubeadmConfigTemplateKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK}, []schema.GroupVersionKind{clusterClassGVK})
	},
	kubeadmConfigKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{machineGVK}, []schema.GroupVersionKind{machineGVK, kubeadmControlPlaneGVK})
	},
}

// Kind and GVK for types in the Docker infrastructure package.
var (
	dockerMachineKind         = "DockerMachine"
	dockerMachineTemplateKind = "DockerMachineTemplate"
	dockerClusterKind         = "DockerCluster"
	dockerClusterTemplateKind = "DockerClusterTemplate"
)
var DockerInfraOwnerReferenceAssertions = map[string]func([]metav1.OwnerReference) error{
	dockerMachineKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{machineGVK}, []schema.GroupVersionKind{machineGVK, kubeadmControlPlaneGVK})
	},
	dockerMachineTemplateKind: func(owners []metav1.OwnerReference) error {
		return hasOneOfExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK}, []schema.GroupVersionKind{clusterClassGVK})
	},
	dockerClusterKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterGVK})
	},
	dockerClusterTemplateKind: func(owners []metav1.OwnerReference) error {
		return hasExactOwnersByGVK(owners, []schema.GroupVersionKind{clusterClassGVK})
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
		return errors.New(fmt.Sprintf("wanted %v, actual %v", wantGVKs, refGVKs))
	}
	return nil
}

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
