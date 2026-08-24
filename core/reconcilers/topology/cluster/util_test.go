/*
Copyright 2021 The Kubernetes Authors.

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

package cluster

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestGetReference(t *testing.T) {
	fakeControlPlaneTemplateCRDv99 := builder.GenericControlPlaneTemplateCRD.DeepCopy()
	fakeControlPlaneTemplateCRDv99.Labels = map[string]string{
		clusterv1.GroupVersion.String(): "v99",
	}
	crds := []client.Object{
		fakeControlPlaneTemplateCRDv99,
		builder.GenericBootstrapConfigTemplateCRD,
	}

	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetemplate1").Build()
	controlPlaneTemplatev99 := controlPlaneTemplate.DeepCopy()
	controlPlaneTemplatev99.SetAPIVersion(builder.ControlPlaneGroupVersion.Group + "/v99")

	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "workerbootstraptemplate1").Build()

	tests := []struct {
		name    string
		ref     *corev1.ObjectReference
		objects []client.Object
		want    *unstructured.Unstructured
		wantRef *corev1.ObjectReference
		wantErr bool
	}{
		{
			name:    "Get object fails: ref is nil",
			ref:     nil,
			wantErr: true,
		},
		{
			name: "Get object",
			ref:  contract.ObjToRef(workerBootstrapTemplate),
			objects: []client.Object{
				workerBootstrapTemplate,
			},
			want:    workerBootstrapTemplate,
			wantRef: contract.ObjToRef(workerBootstrapTemplate),
		},
		{
			name:    "Get object fails: object does not exist",
			ref:     contract.ObjToRef(workerBootstrapTemplate),
			objects: []client.Object{},
			wantErr: true,
		},
		{
			name: "Get object fails: object does not exist with this apiVersion",
			ref:  contract.ObjToRef(controlPlaneTemplate),
			objects: []client.Object{
				controlPlaneTemplatev99,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			r := &Reconciler{
				Client: fakeClient,
			}
			got, err := r.getReference(ctx, tt.ref)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(EqualObject(tt.want))
			g.Expect(tt.ref).To(EqualObject(tt.wantRef))
		})
	}
}

func TestGetTemplate(t *testing.T) {
	fakeControlPlaneTemplateCRDv99 := builder.GenericControlPlaneTemplateCRD.DeepCopy()
	fakeControlPlaneTemplateCRDv99.Labels = map[string]string{
		clusterv1.GroupVersion.String(): "v99",
	}
	crds := []client.Object{
		fakeControlPlaneTemplateCRDv99,
		builder.GenericBootstrapConfigTemplateCRD,
	}

	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetemplate1").Build()
	controlPlaneTemplateRef := clusterv1.ClusterClassTemplateReference{
		APIVersion: controlPlaneTemplate.GroupVersionKind().GroupVersion().String(),
		Kind:       controlPlaneTemplate.GetKind(),
		Name:       controlPlaneTemplate.GetName(),
	}
	controlPlaneTemplateNamespace := controlPlaneTemplate.GetNamespace()
	controlPlaneTemplatev99 := controlPlaneTemplate.DeepCopy()
	controlPlaneTemplatev99.SetAPIVersion(builder.ControlPlaneGroupVersion.Group + "/v99")

	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "workerbootstraptemplate1").Build()
	workerBootstrapTemplateRef := clusterv1.ClusterClassTemplateReference{
		APIVersion: workerBootstrapTemplate.GroupVersionKind().GroupVersion().String(),
		Kind:       workerBootstrapTemplate.GetKind(),
		Name:       workerBootstrapTemplate.GetName(),
	}
	workerBootstrapTemplateNamespace := workerBootstrapTemplate.GetNamespace()

	tests := []struct {
		name        string
		templateRef clusterv1.ClusterClassTemplateReference
		namespace   string
		template    clusterv1.ClusterClassTemplate
		objects     []client.Object
		want        *unstructured.Unstructured
		wantErr     bool
	}{
		{
			name:        "Get object fails: neither templateRef nor template is set",
			templateRef: clusterv1.ClusterClassTemplateReference{},
			template:    clusterv1.ClusterClassTemplate{},
			wantErr:     true,
		},
		{
			name:        "Get object from templateRef",
			templateRef: workerBootstrapTemplateRef,
			namespace:   workerBootstrapTemplateNamespace,
			objects: []client.Object{
				workerBootstrapTemplate,
			},
			want: workerBootstrapTemplate,
		},
		{
			name:        "Get object from templateRef fails: object does not exist",
			templateRef: workerBootstrapTemplateRef,
			namespace:   workerBootstrapTemplateNamespace,
			objects:     []client.Object{},
			wantErr:     true,
		},
		{
			name:        "Get object from templateRef fails: object does not exist with this apiVersion",
			templateRef: controlPlaneTemplateRef,
			namespace:   controlPlaneTemplateNamespace,
			objects: []client.Object{
				controlPlaneTemplatev99,
			},
			wantErr: true,
		},
		{
			name: "Get object from inline template",
			template: clusterv1.ClusterClassTemplate{
				APIVersion: controlPlaneTemplate.GroupVersionKind().GroupVersion().String(),
				Kind:       controlPlaneTemplate.GetKind(),
			},
			want: func() *unstructured.Unstructured {
				u := &unstructured.Unstructured{}
				u.SetAPIVersion(controlPlaneTemplate.GroupVersionKind().GroupVersion().String())
				u.SetKind(controlPlaneTemplate.GetKind())
				return u
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			r := &Reconciler{
				Client: fakeClient,
			}
			got, err := r.getTemplate(ctx, tt.templateRef, tt.namespace, tt.template)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(EqualObject(tt.want))
		})
	}
}
