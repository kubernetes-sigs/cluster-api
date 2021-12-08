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

package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// GetCAPIResourcesInput is the input for GetCAPIResources.
type GetCAPIResourcesInput struct {
	Lister    Lister
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
			Fail(fmt.Sprintf("failed to list %q resources: %v", typeList.GroupVersionKind(), err))
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
func getClusterAPITypes(ctx context.Context, lister Lister) []metav1.TypeMeta {
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

// DumpAllResourcesInput is the input for DumpAllResources.
type DumpAllResourcesInput struct {
	Lister    Lister
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

	resourceFilePath := filepath.Clean(path.Join(logPath, namespace, kind, name+".yaml"))
	Expect(os.MkdirAll(filepath.Dir(resourceFilePath), 0750)).To(Succeed(), "Failed to create folder %s", filepath.Dir(resourceFilePath))

	f, err := os.OpenFile(resourceFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	Expect(err).ToNot(HaveOccurred(), "Failed to open %s", resourceFilePath)
	defer f.Close()

	Expect(os.WriteFile(f.Name(), resourceYAML, 0600)).To(Succeed(), "Failed to write %s", resourceFilePath)
}

// capiProviderOptions returns a set of ListOptions that allows to identify all the objects belonging to Cluster API providers.
func capiProviderOptions() []client.ListOption {
	return []client.ListOption{
		client.HasLabels{clusterv1.ProviderLabelName},
	}
}

// CreateRelatedResourcesInput is the input type for CreateRelatedResources.
type CreateRelatedResourcesInput struct {
	Creator          Creator
	RelatedResources []client.Object
}

// CreateRelatedResources is used to create runtime.Objects.
func CreateRelatedResources(ctx context.Context, input CreateRelatedResourcesInput, intervals ...interface{}) {
	By("creating related resources")
	for i := range input.RelatedResources {
		obj := input.RelatedResources[i]
		By(fmt.Sprintf("creating a/an %s resource", obj.GetObjectKind().GroupVersionKind()))
		Eventually(func() error {
			return input.Creator.Create(ctx, obj)
		}, intervals...).Should(Succeed())
	}
}

// PrettyPrint returns a formatted JSON version of the object given.
func PrettyPrint(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(b)
}
