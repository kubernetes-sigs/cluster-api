// +build e2e

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

package discovery

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Provides methods for discovering Cluster API objects existing in the management cluster.

// GetControllerDeploymentsInput is the input for GetControllerDeployments.
type GetControllerDeploymentsInput struct {
	Lister framework.Lister
}

// GetControllerDeployments returns all the deployment for the cluster API controllers existing in a management cluster.
func GetControllerDeployments(ctx context.Context, input GetControllerDeploymentsInput) []*appsv1.Deployment {
	deploymentList := &appsv1.DeploymentList{}
	Expect(input.Lister.List(ctx, deploymentList, capiProviderOptions()...)).To(Succeed(), "Failed to list deployments for the cluster API controllers")

	deployments := make([]*appsv1.Deployment, len(deploymentList.Items))
	for i := range deploymentList.Items {
		deployments[i] = &deploymentList.Items[i]
	}
	return deployments
}

// GetCAPIResourcesInput is the input for GetCAPIResources.
type GetCAPIResourcesInput struct {
	Lister    framework.Lister
	Namespace string
}

// GetCAPIResources reads all the CAPI resources in a namespace.
// This list includes all the types belonging to CAPI providers.
func GetCAPIResources(ctx context.Context, input GetCAPIResourcesInput) []*unstructured.Unstructured {
	Expect(ctx).NotTo(BeNil(), "ctx is required for GetCAPIResources")
	Expect(input.Lister).NotTo(BeNil(), "input.Deleter is required for GetCAPIResources")
	Expect(input.Namespace).NotTo(BeEmpty(), "input.Namespace is required for GetCAPIResources")

	types := getClusterAPITypes(ctx, input.Lister)

	objList := []*unstructured.Unstructured{}
	for i := range types {
		typeMeta := types[i]
		typeList := new(unstructured.UnstructuredList)
		typeList.SetAPIVersion(typeMeta.APIVersion)
		typeList.SetKind(typeMeta.Kind)

		if err := input.Lister.List(ctx, typeList, client.InNamespace(input.Namespace)); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			ginkgo.Fail(fmt.Sprintf("failed to list %q resources: %v", typeList.GroupVersionKind(), err))
		}
		for i := range typeList.Items {
			obj := typeList.Items[i]
			objList = append(objList, &obj)
		}
	}

	return objList
}

// getClusterAPITypes returns the list of TypeMeta to be considered for the the move discovery phase.
// This list includes all the types belonging to CAPI providers.
func getClusterAPITypes(ctx context.Context, lister framework.Lister) []metav1.TypeMeta {
	discoveredTypes := []metav1.TypeMeta{}

	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	err := lister.List(ctx, crdList, capiProviderOptions()...)
	Expect(err).ToNot(HaveOccurred(), "failed to list CRDs for CAPI providers")

	for _, crd := range crdList.Items {
		for _, version := range crd.Spec.Versions {
			if !version.Storage {
				continue
			}

			discoveredTypes = append(discoveredTypes, metav1.TypeMeta{
				Kind: crd.Spec.Names.Kind,
				APIVersion: metav1.GroupVersion{
					Group:   crd.Spec.Group,
					Version: version.Name,
				}.String(),
			})
		}
	}
	return discoveredTypes
}

// GetClusterByNameInput is the input for GetClusterByName.
type GetClusterByNameInput struct {
	Getter    framework.Getter
	Name      string
	Namespace string
}

// GetClusterByName returns a Cluster object given his name
func GetClusterByName(ctx context.Context, input GetClusterByNameInput) *clusterv1.Cluster {
	cluster := &clusterv1.Cluster{}
	key := client.ObjectKey{
		Namespace: input.Namespace,
		Name:      input.Name,
	}
	Expect(input.Getter.Get(ctx, key, cluster)).To(Succeed(), "Failed to get Cluster object %s/%s", input.Namespace, input.Name)
	return cluster
}

// GetKubeadmControlPlaneByClusterInput is the input for GetKubeadmControlPlaneByCluster.
type GetKubeadmControlPlaneByClusterInput struct {
	Lister      framework.Lister
	ClusterName string
	Namespace   string
}

// GetKubeadmControlPlaneByCluster returns the KubeadmControlPlane objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetKubeadmControlPlaneByCluster(ctx context.Context, input GetKubeadmControlPlaneByClusterInput) *controlplanev1.KubeadmControlPlane {
	controlPlaneList := &controlplanev1.KubeadmControlPlaneList{}
	Expect(input.Lister.List(ctx, controlPlaneList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list KubeadmControlPlane object for Cluster %s/%s", input.Namespace, input.ClusterName)
	Expect(len(controlPlaneList.Items)).ToNot(BeNumerically(">", 1), "Cluster %s/%s should not have more than 1 KubeadmControlPlane object", input.Namespace, input.ClusterName)
	if len(controlPlaneList.Items) == 1 {
		return &controlPlaneList.Items[0]
	}
	return nil
}

// GetMachineDeploymentsByClusterInput is the input for GetMachineDeploymentsByCluster.
type GetMachineDeploymentsByClusterInput struct {
	Lister      framework.Lister
	ClusterName string
	Namespace   string
}

// GetMachineDeploymentsByCluster returns the MachineDeployments objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachineDeploymentsByCluster(ctx context.Context, input GetMachineDeploymentsByClusterInput) []*clusterv1.MachineDeployment {
	deploymentList := &clusterv1.MachineDeploymentList{}
	Expect(input.Lister.List(ctx, deploymentList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list MachineDeployments object for Cluster %s/%s", input.Namespace, input.ClusterName)

	deployments := make([]*clusterv1.MachineDeployment, len(deploymentList.Items))
	for i := range deploymentList.Items {
		deployments[i] = &deploymentList.Items[i]
	}
	return deployments
}

// DumpAllResourcesInput is the input for DumpAllResources.
type DumpAllResourcesInput struct {
	Lister    framework.Lister
	Namespace string
	LogPath   string
}

// DumpAllResources dumps Cluster API related resources to YAML
// This dump includes all the types belonging to CAPI providers.
func DumpAllResources(ctx context.Context, input DumpAllResourcesInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DumpAllResources")
	Expect(input.Lister).NotTo(BeNil(), "input.Deleter is required for DumpAllResources")
	Expect(input.Namespace).NotTo(BeEmpty(), "input.Namespace is required for DumpAllResources")

	resources := GetCAPIResources(ctx, GetCAPIResourcesInput{
		Lister:    input.Lister,
		Namespace: input.Namespace,
	})

	for i := range resources {
		r := resources[i]
		dumpObject(r, input.LogPath)
	}
}

func dumpObject(resource runtime.Object, logPath string) {
	resourceYAML, err := yaml.Marshal(resource)
	Expect(err).ToNot(HaveOccurred(), "Failed to marshal %s", resource.GetObjectKind().GroupVersionKind().String())

	metaObj, err := apimeta.Accessor(resource)
	Expect(err).ToNot(HaveOccurred(), "Failed to get accessor for %s", resource.GetObjectKind().GroupVersionKind().String())

	kind := resource.GetObjectKind().GroupVersionKind().Kind
	namespace := metaObj.GetNamespace()
	name := metaObj.GetName()

	resourceFilePath := path.Join(logPath, namespace, kind, name+".yaml")
	Expect(os.MkdirAll(filepath.Dir(resourceFilePath), 0755)).To(Succeed(), "Failed to create folder %s", filepath.Dir(resourceFilePath))

	f, err := os.OpenFile(resourceFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	Expect(err).ToNot(HaveOccurred(), "Failed to open %s", resourceFilePath)
	defer f.Close()

	Expect(ioutil.WriteFile(f.Name(), resourceYAML, 0644)).To(Succeed(), "Failed to write %s", resourceFilePath)
}

// capiProviderOptions returns a set of ListOptions that allows to identify all the objects belonging to Cluster API providers.
func capiProviderOptions() []client.ListOption {
	return []client.ListOption{
		client.HasLabels{clusterv1.ProviderLabelName},
	}
}

// byClusterOptions returns a set of ListOptions that allows to identify all the objects belonging to a Cluster.
func byClusterOptions(name, namespace string) []client.ListOption {
	return []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: name,
		},
	}
}
