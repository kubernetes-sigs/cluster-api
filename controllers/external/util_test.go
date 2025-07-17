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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

const (
	testClusterName = "test-cluster"
)

func TestGetResourceFound(t *testing.T) {
	g := NewWithT(t)

	testResourceName := "greenTemplate"
	testResourceKind := "GreenTemplate"
	testResourceAPIVersion := "green.io/v1"
	testResourceVersion := "999"

	testResource := &unstructured.Unstructured{}
	testResource.SetKind(testResourceKind)
	testResource.SetAPIVersion(testResourceAPIVersion)
	testResource.SetName(testResourceName)
	testResource.SetNamespace(metav1.NamespaceDefault)
	testResource.SetResourceVersion(testResourceVersion)

	testResourceReference := &corev1.ObjectReference{
		Kind:       testResourceKind,
		APIVersion: testResourceAPIVersion,
		Name:       testResourceName,
		Namespace:  metav1.NamespaceDefault,
	}

	fakeClient := fake.NewClientBuilder().WithObjects(testResource.DeepCopy()).Build()
	got, err := Get(ctx, fakeClient, testResourceReference)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(BeComparableTo(testResource))
}

func TestGetResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	testResourceReference := &corev1.ObjectReference{
		Kind:       "BlueTemplate",
		APIVersion: "blue.io/v1",
		Name:       "blueTemplate",
		Namespace:  metav1.NamespaceDefault,
	}

	fakeClient := fake.NewClientBuilder().Build()
	_, err := Get(ctx, fakeClient, testResourceReference)
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(errors.Cause(err))).To(BeTrue())
}

func TestGetObjectFromContractVersionedRef(t *testing.T) {
	testCases := []struct {
		name                string
		ref                 *clusterv1.ContractVersionedObjectReference
		objs                []client.Object
		expectError         bool
		expectNotFoundError bool
	}{
		{
			name: "object found",
			ref: &clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.BootstrapGroupVersion.Group,
				Kind:     builder.TestBootstrapConfigKind,
				Name:     "bootstrap-config",
			},
			objs: []client.Object{
				builder.TestBootstrapConfig(metav1.NamespaceDefault, "bootstrap-config").Build(),
				builder.TestBootstrapConfigCRD,
			},
			expectError:         false,
			expectNotFoundError: false,
		},
		{
			name: "object not found, because CRD is missing",
			ref: &clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.BootstrapGroupVersion.Group,
				Kind:     builder.TestBootstrapConfigKind,
				Name:     "bootstrap-config",
			},
			objs: []client.Object{
				builder.TestBootstrapConfig(metav1.NamespaceDefault, "bootstrap-config").Build(),
				// corresponding CRD is missing
			},
			expectError:         true,
			expectNotFoundError: false,
		},
		{
			name: "object not found, because object is missing",
			ref: &clusterv1.ContractVersionedObjectReference{
				APIGroup: builder.BootstrapGroupVersion.Group,
				Kind:     builder.TestBootstrapConfigKind,
				Name:     "bootstrap-config",
			},
			objs: []client.Object{
				// object is missing
				builder.TestBootstrapConfigCRD,
			},
			expectError:         true,
			expectNotFoundError: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objs...).Build()

			object, err := GetObjectFromContractVersionedRef(t.Context(), c, tt.ref, metav1.NamespaceDefault)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(Equal(tt.expectNotFoundError))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(object.GetAPIVersion()).To(Equal(builder.BootstrapGroupVersion.String()))
			}
		})
	}
}

func TestCloneTemplateResourceNotFound(t *testing.T) {
	g := NewWithT(t)

	testClusterName := "bar"

	testResourceReference := &corev1.ObjectReference{
		Kind:       "OrangeTemplate",
		APIVersion: "orange.io/v1",
		Name:       "orangeTemplate",
		Namespace:  metav1.NamespaceDefault,
	}

	fakeClient := fake.NewClientBuilder().Build()
	_, _, err := CreateFromTemplate(ctx, &CreateFromTemplateInput{
		Client:      fakeClient,
		TemplateRef: testResourceReference,
		Namespace:   metav1.NamespaceDefault,
		ClusterName: testClusterName,
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(apierrors.IsNotFound(errors.Cause(err))).To(BeTrue())
}

func TestCloneTemplateResourceFound(t *testing.T) {
	g := NewWithT(t)

	templateName := "purpleTemplate"
	templateKind := "PurpleTemplate"
	templateAPIVersion := "purple.io/v1"

	template := unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       templateKind,
			"apiVersion": templateAPIVersion,
			"metadata": map[string]interface{}{
				"name":      templateName,
				"namespace": metav1.NamespaceDefault,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"test-template": "annotations",
							"precedence":    "template",
						},
						"labels": map[string]interface{}{
							"test-template": "label",
							"precedence":    "template",
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
		Namespace:  metav1.NamespaceDefault,
	}

	owner := metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       testClusterName,
	}

	expectedKind := "Purple"
	expectedAPIGroup := "purple.io" // group from templateAPIVersion
	expectedMetadata, ok, err := unstructured.NestedMap(template.UnstructuredContent(), "spec", "template", "metadata")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(expectedMetadata).NotTo(BeEmpty())

	expectedSpec, ok, err := unstructured.NestedMap(template.UnstructuredContent(), "spec", "template", "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(expectedSpec).NotTo(BeEmpty())

	fakeClient := fake.NewClientBuilder().WithObjects(template.DeepCopy()).Build()

	_, ref, err := CreateFromTemplate(ctx, &CreateFromTemplateInput{
		Client:      fakeClient,
		TemplateRef: templateRef.DeepCopy(),
		Namespace:   metav1.NamespaceDefault,
		ClusterName: testClusterName,
		OwnerRef:    owner.DeepCopy(),
		Labels: map[string]string{
			"precedence":               "input",
			clusterv1.ClusterNameLabel: "should-be-overwritten",
		},
		Annotations: map[string]string{
			"precedence": "input",
			clusterv1.TemplateClonedFromNameAnnotation: "should-be-overwritten",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ref).NotTo(BeNil())
	g.Expect(ref.Kind).To(Equal(expectedKind))
	g.Expect(ref.APIGroup).To(Equal(expectedAPIGroup))
	g.Expect(ref.Name).To(HavePrefix(templateRef.Name))

	clone := &unstructured.Unstructured{}
	clone.SetKind(expectedKind)
	clone.SetAPIVersion(templateAPIVersion)

	key := client.ObjectKey{Name: ref.Name, Namespace: metav1.NamespaceDefault}
	g.Expect(fakeClient.Get(ctx, key, clone)).To(Succeed())
	g.Expect(clone.GetOwnerReferences()).To(HaveLen(1))
	g.Expect(clone.GetOwnerReferences()).To(ContainElement(owner))

	cloneSpec, ok, err := unstructured.NestedMap(clone.UnstructuredContent(), "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(BeComparableTo(expectedSpec))

	cloneLabels := clone.GetLabels()
	g.Expect(cloneLabels).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, testClusterName))
	g.Expect(cloneLabels).To(HaveKeyWithValue("test-template", "label"))
	g.Expect(cloneLabels).To(HaveKeyWithValue("precedence", "input"))

	cloneAnnotations := clone.GetAnnotations()
	g.Expect(cloneAnnotations).To(HaveKeyWithValue("test-template", "annotations"))
	g.Expect(cloneAnnotations).To(HaveKeyWithValue("precedence", "input"))

	g.Expect(cloneAnnotations).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, templateRef.Name))
	g.Expect(cloneAnnotations).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, templateRef.GroupVersionKind().GroupKind().String()))
}

func TestCloneTemplateResourceFoundNoOwner(t *testing.T) {
	g := NewWithT(t)

	templateName := "yellowTemplate"
	templateKind := "YellowTemplate"
	templateAPIVersion := "yellow.io/v1"

	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       templateKind,
			"apiVersion": templateAPIVersion,
			"metadata": map[string]interface{}{
				"name":      templateName,
				"namespace": metav1.NamespaceDefault,
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
		Namespace:  metav1.NamespaceDefault,
	}

	expectedKind := "Yellow"
	expectedAPIGroup := "yellow.io" // group from templateAPIVersion
	expectedLabels := map[string]string{clusterv1.ClusterNameLabel: testClusterName}

	expectedSpec, ok, err := unstructured.NestedMap(template.UnstructuredContent(), "spec", "template", "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(expectedSpec).NotTo(BeEmpty())

	fakeClient := fake.NewClientBuilder().WithObjects(template.DeepCopy()).Build()

	_, ref, err := CreateFromTemplate(ctx, &CreateFromTemplateInput{
		Client:      fakeClient,
		TemplateRef: templateRef,
		Namespace:   metav1.NamespaceDefault,
		Name:        "object-name",
		ClusterName: testClusterName,
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ref).NotTo(BeNil())
	g.Expect(ref.Kind).To(Equal(expectedKind))
	g.Expect(ref.APIGroup).To(Equal(expectedAPIGroup))
	g.Expect(ref.Name).To(Equal("object-name"))

	clone := &unstructured.Unstructured{}
	clone.SetKind(expectedKind)
	clone.SetAPIVersion(templateAPIVersion)
	key := client.ObjectKey{Name: ref.Name, Namespace: metav1.NamespaceDefault}
	g.Expect(fakeClient.Get(ctx, key, clone)).To(Succeed())
	g.Expect(clone.GetLabels()).To(Equal(expectedLabels))
	g.Expect(clone.GetOwnerReferences()).To(BeEmpty())
	cloneSpec, ok, err := unstructured.NestedMap(clone.UnstructuredContent(), "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(BeComparableTo(expectedSpec))
}

func TestCloneTemplateMissingSpec(t *testing.T) {
	g := NewWithT(t)

	templateName := "aquaTemplate"
	templateKind := "AquaTemplate"
	templateAPIVersion := "aqua.io/v1"

	template := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       templateKind,
			"apiVersion": templateAPIVersion,
			"metadata": map[string]interface{}{
				"name":      templateName,
				"namespace": metav1.NamespaceDefault,
			},
		},
	}

	templateRef := &corev1.ObjectReference{
		Kind:       templateKind,
		APIVersion: templateAPIVersion,
		Name:       templateName,
		Namespace:  metav1.NamespaceDefault,
	}

	owner := metav1.OwnerReference{
		Kind:       "Cluster",
		APIVersion: clusterv1.GroupVersion.String(),
		Name:       testClusterName,
	}

	expectedKind := "Aqua"
	expectedAPIGroup := "aqua.io" // group from templateAPIVersion

	fakeClient := fake.NewClientBuilder().WithObjects(template.DeepCopy()).Build()

	_, ref, err := CreateFromTemplate(ctx, &CreateFromTemplateInput{
		Client:      fakeClient,
		TemplateRef: templateRef.DeepCopy(),
		Namespace:   metav1.NamespaceDefault,
		ClusterName: testClusterName,
		OwnerRef:    owner.DeepCopy(),
		Labels: map[string]string{
			"precedence":               "input",
			clusterv1.ClusterNameLabel: "should-be-overwritten",
		},
		Annotations: map[string]string{
			"precedence": "input",
			clusterv1.TemplateClonedFromNameAnnotation: "should-be-overwritten",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ref).NotTo(BeNil())
	g.Expect(ref.Kind).To(Equal(expectedKind))
	g.Expect(ref.APIGroup).To(Equal(expectedAPIGroup))
	g.Expect(ref.Name).To(HavePrefix(templateRef.Name))

	clone := &unstructured.Unstructured{}
	clone.SetKind(expectedKind)
	clone.SetAPIVersion(templateAPIVersion)

	key := client.ObjectKey{Name: ref.Name, Namespace: metav1.NamespaceDefault}
	g.Expect(fakeClient.Get(ctx, key, clone)).To(Succeed())
	g.Expect(clone.GetOwnerReferences()).To(HaveLen(1))
	g.Expect(clone.GetOwnerReferences()).To(ContainElement(owner))

	cloneSpec, ok, err := unstructured.NestedMap(clone.UnstructuredContent(), "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeFalse())
	g.Expect(cloneSpec).To(BeNil())

	cloneLabels := clone.GetLabels()
	g.Expect(cloneLabels).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, testClusterName))
	g.Expect(cloneLabels).To(HaveKeyWithValue("precedence", "input"))

	cloneAnnotations := clone.GetAnnotations()
	g.Expect(cloneAnnotations).To(HaveKeyWithValue("precedence", "input"))

	g.Expect(cloneAnnotations).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, templateRef.Name))
	g.Expect(cloneAnnotations).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, templateRef.GroupVersionKind().GroupKind().String()))
}
