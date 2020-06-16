/*
Copyright 2019 The Kubernetes Authors.

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

package external

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestGetResourceFound(t *testing.T) {
	g := NewWithT(t)

	namespace := "test"
	testResourceName := "greenTemplate"
	testResourceKind := "GreenTemplate"
	testResourceAPIVersion := "green.io/v1"

	testResource := &unstructured.Unstructured{}
	testResource.SetKind(testResourceKind)
	testResource.SetAPIVersion(testResourceAPIVersion)
	testResource.SetName(testResourceName)
	testResource.SetNamespace(namespace)

	testResourceReference := &corev1.ObjectReference{
		Kind:       testResourceKind,
		APIVersion: testResourceAPIVersion,
		Name:       testResourceName,
		Namespace:  namespace,
	}

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme(), testResource.DeepCopy())
	got, err := Get(context.Background(), fakeClient, testResourceReference, namespace)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal(testResource))
}

func TestGetResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	namespace := "test"

	testResourceReference := &corev1.ObjectReference{
		Kind:       "BlueTemplate",
		APIVersion: "blue.io/v1",
		Name:       "blueTemplate",
		Namespace:  namespace,
	}

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme())
	_, err := Get(context.Background(), fakeClient, testResourceReference, namespace)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(errors.Cause(err))).To(BeTrue())
}

func TestCloneTemplateResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	namespace := "test"
	testClusterName := "bar"

	testResourceReference := &corev1.ObjectReference{
		Kind:       "OrangeTemplate",
		APIVersion: "orange.io/v1",
		Name:       "orangeTemplate",
		Namespace:  namespace,
	}

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme())
	_, err := CloneTemplate(context.Background(), &CloneTemplateInput{
		Client:      fakeClient,
		TemplateRef: testResourceReference,
		Namespace:   namespace,
		ClusterName: testClusterName,
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(errors.Cause(err))).To(BeTrue())
}

func TestCloneTemplateResourceFound(t *testing.T) {
	g := NewWithT(t)

	namespace := "test"
	testClusterName := "test-cluster"

	templateName := "purpleTemplate"
	templateKind := "PurpleTemplate"
	templateAPIVersion := "purple.io/v1"

	template := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       templateKind,
			"apiVersion": templateAPIVersion,
			"metadata": map[string]interface{}{
				"name":      templateName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"test": "annotations",
						},
					},
					"spec": map[string]interface{}{
						"hello": "world",
					},
				},
			},
		},
	}

	templateRef := corev1.ObjectReference{
		Kind:       templateKind,
		APIVersion: templateAPIVersion,
		Name:       templateName,
		Namespace:  namespace,
	}

	owner := metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       "test-cluster",
	}

	expectedKind := "Purple"
	expectedAPIVersion := templateAPIVersion
	expectedMetadata, ok, err := unstructured.NestedMap(template.UnstructuredContent(), "spec", "template", "metadata")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(expectedMetadata).NotTo(BeEmpty())

	expectedSpec, ok, err := unstructured.NestedMap(template.UnstructuredContent(), "spec", "template", "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(expectedSpec).NotTo(BeEmpty())

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme(), template.DeepCopy())

	ref, err := CloneTemplate(context.Background(), &CloneTemplateInput{
		Client:      fakeClient,
		TemplateRef: templateRef.DeepCopy(),
		Namespace:   namespace,
		ClusterName: testClusterName,
		OwnerRef:    owner.DeepCopy(),
		Labels: map[string]string{
			"test-label-1": "value-1",
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ref).NotTo(BeNil())
	g.Expect(ref.Kind).To(Equal(expectedKind))
	g.Expect(ref.APIVersion).To(Equal(expectedAPIVersion))
	g.Expect(ref.Namespace).To(Equal(namespace))
	g.Expect(ref.Name).To(HavePrefix(templateRef.Name))

	clone := &unstructured.Unstructured{}
	clone.SetKind(expectedKind)
	clone.SetAPIVersion(expectedAPIVersion)

	key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, clone)).To(Succeed())
	g.Expect(clone.GetOwnerReferences()).To(HaveLen(1))
	g.Expect(clone.GetOwnerReferences()).To(ContainElement(owner))

	cloneSpec, ok, err := unstructured.NestedMap(clone.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(Equal(expectedSpec))

	cloneLabels := clone.GetLabels()
	g.Expect(cloneLabels).To(HaveKeyWithValue(clusterv1.ClusterLabelName, testClusterName))
	g.Expect(cloneLabels).To(HaveKeyWithValue("test-label-1", "value-1"))

	cloneAnnotations := clone.GetAnnotations()
	g.Expect(cloneAnnotations).To(HaveKeyWithValue("test", "annotations"))

	g.Expect(cloneAnnotations).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, templateRef.Name))
	g.Expect(cloneAnnotations).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, templateRef.GroupVersionKind().GroupKind().String()))
}

func TestCloneTemplateResourceFoundNoOwner(t *testing.T) {
	g := NewWithT(t)

	namespace := "test"
	testClusterName := "test-cluster"

	templateName := "yellowTemplate"
	templateKind := "YellowTemplate"
	templateAPIVersion := "yellow.io/v1"

	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       templateKind,
			"apiVersion": templateAPIVersion,
			"metadata": map[string]interface{}{
				"name":      templateName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world",
					},
				},
			},
		},
	}

	templateRef := &corev1.ObjectReference{
		Kind:       templateKind,
		APIVersion: templateAPIVersion,
		Name:       templateName,
		Namespace:  namespace,
	}

	expectedKind := "Yellow"
	expectedAPIVersion := templateAPIVersion
	expectedLabels := (map[string]string{clusterv1.ClusterLabelName: testClusterName})

	expectedSpec, ok, err := unstructured.NestedMap(template.UnstructuredContent(), "spec", "template", "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(expectedSpec).NotTo(BeEmpty())

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme(), template.DeepCopy())

	ref, err := CloneTemplate(context.Background(), &CloneTemplateInput{
		Client:      fakeClient,
		TemplateRef: templateRef,
		Namespace:   namespace,
		ClusterName: testClusterName,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ref).NotTo(BeNil())
	g.Expect(ref.Kind).To(Equal(expectedKind))
	g.Expect(ref.APIVersion).To(Equal(expectedAPIVersion))
	g.Expect(ref.Namespace).To(Equal(namespace))
	g.Expect(ref.Name).To(HavePrefix(templateRef.Name))

	clone := &unstructured.Unstructured{}
	clone.SetKind(expectedKind)
	clone.SetAPIVersion(expectedAPIVersion)
	key := client.ObjectKey{Name: ref.Name, Namespace: ref.Namespace}
	g.Expect(fakeClient.Get(context.Background(), key, clone)).To(Succeed())
	g.Expect(clone.GetLabels()).To(Equal(expectedLabels))
	g.Expect(clone.GetOwnerReferences()).To(BeEmpty())
	cloneSpec, ok, err := unstructured.NestedMap(clone.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(Equal(expectedSpec))
}

func TestCloneTemplateMissingSpecTemplate(t *testing.T) {
	g := NewWithT(t)

	namespace := "test"
	testClusterName := "test-cluster"

	templateName := "aquaTemplate"
	templateKind := "AquaTemplate"
	templateAPIVersion := "aqua.io/v1"

	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       templateKind,
			"apiVersion": templateAPIVersion,
			"metadata": map[string]interface{}{
				"name":      templateName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{},
		},
	}

	templateRef := &corev1.ObjectReference{
		Kind:       templateKind,
		APIVersion: templateAPIVersion,
		Name:       templateName,
		Namespace:  namespace,
	}

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme(), template.DeepCopy())

	_, err := CloneTemplate(context.Background(), &CloneTemplateInput{
		Client:      fakeClient,
		TemplateRef: templateRef,
		Namespace:   namespace,
		ClusterName: testClusterName,
	})
	g.Expect(err).To(HaveOccurred())
}
