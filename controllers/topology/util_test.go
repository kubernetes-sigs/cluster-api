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

package topology

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	"sigs.k8s.io/cluster-api/internal/builder"
	. "sigs.k8s.io/cluster-api/internal/matchers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetReference(t *testing.T) {
	fakeControlPlaneTemplateCRDv99 := builder.GenericControlPlaneTemplateCRD.DeepCopy()
	fakeControlPlaneTemplateCRDv99.Labels = map[string]string{
		"cluster.x-k8s.io/v1beta1": "v1beta1_v99",
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
			name: "Get object and update the ref",
			ref:  contract.ObjToRef(controlPlaneTemplate),
			objects: []client.Object{
				controlPlaneTemplatev99,
			},
			want:    controlPlaneTemplatev99,
			wantRef: contract.ObjToRef(controlPlaneTemplatev99),
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

			r := &ClusterReconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
			}
			got, err := r.getReference(ctx, tt.ref)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(EqualObject(tt.want))
			g.Expect(tt.ref).To(EqualObject(tt.wantRef))
		})
	}
}

func TestCalculateTemplatesInUse(t *testing.T) {
	t.Run("Calculate templates in use with regular MachineDeployment and MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		md := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithBootstrapTemplate(builder.BootstrapTemplate(metav1.NamespaceDefault, "mdBT").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()).
			Build()
		ms := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithBootstrapTemplate(builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()

		actual, err := calculateTemplatesInUse(md, []*clusterv1.MachineSet{ms})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(4))

		g.Expect(actual).To(HaveKey(mustTemplateRefID(md.Spec.Template.Spec.Bootstrap.ConfigRef)))
		g.Expect(actual).To(HaveKey(mustTemplateRefID(&md.Spec.Template.Spec.InfrastructureRef)))

		g.Expect(actual).To(HaveKey(mustTemplateRefID(ms.Spec.Template.Spec.Bootstrap.ConfigRef)))
		g.Expect(actual).To(HaveKey(mustTemplateRefID(&ms.Spec.Template.Spec.InfrastructureRef)))
	})

	t.Run("Calculate templates in use with MachineDeployment and MachineSet without BootstrapTemplate", func(t *testing.T) {
		g := NewWithT(t)

		mdWithoutBootstrapTemplate := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()).
			Build()
		msWithoutBootstrapTemplate := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()

		actual, err := calculateTemplatesInUse(mdWithoutBootstrapTemplate, []*clusterv1.MachineSet{msWithoutBootstrapTemplate})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(2))

		g.Expect(actual).To(HaveKey(mustTemplateRefID(&mdWithoutBootstrapTemplate.Spec.Template.Spec.InfrastructureRef)))

		g.Expect(actual).To(HaveKey(mustTemplateRefID(&msWithoutBootstrapTemplate.Spec.Template.Spec.InfrastructureRef)))
	})

	t.Run("Calculate templates in use with MachineDeployment and MachineSet ignore templates when resources in deleting", func(t *testing.T) {
		g := NewWithT(t)

		deletionTimeStamp := metav1.Now()

		mdInDeleting := builder.MachineDeployment(metav1.NamespaceDefault, "md").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "mdIMT").Build()).
			Build()
		mdInDeleting.SetDeletionTimestamp(&deletionTimeStamp)

		msInDeleting := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()
		msInDeleting.SetDeletionTimestamp(&deletionTimeStamp)

		actual, err := calculateTemplatesInUse(mdInDeleting, []*clusterv1.MachineSet{msInDeleting})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(0))

		g.Expect(actual).ToNot(HaveKey(mustTemplateRefID(&mdInDeleting.Spec.Template.Spec.InfrastructureRef)))

		g.Expect(actual).ToNot(HaveKey(mustTemplateRefID(&msInDeleting.Spec.Template.Spec.InfrastructureRef)))
	})

	t.Run("Calculate templates in use without MachineDeployment and with MachineSet", func(t *testing.T) {
		g := NewWithT(t)

		ms := builder.MachineSet(metav1.NamespaceDefault, "ms").
			WithBootstrapTemplate(builder.BootstrapTemplate(metav1.NamespaceDefault, "msBT").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "msIMT").Build()).
			Build()

		actual, err := calculateTemplatesInUse(nil, []*clusterv1.MachineSet{ms})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actual).To(HaveLen(2))

		g.Expect(actual).To(HaveKey(mustTemplateRefID(ms.Spec.Template.Spec.Bootstrap.ConfigRef)))
		g.Expect(actual).To(HaveKey(mustTemplateRefID(&ms.Spec.Template.Spec.InfrastructureRef)))
	})
}

// mustTemplateRefID returns the templateRefID as calculated by templateRefID, but panics
// if templateRefID returns an error.
func mustTemplateRefID(ref *corev1.ObjectReference) string {
	refID, err := templateRefID(ref)
	if err != nil {
		panic(errors.Wrap(err, "failed to calculate templateRefID"))
	}
	return refID
}
