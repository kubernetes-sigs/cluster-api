/*
Copyright 2022 The Kubernetes Authors.

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

package structuredmerge

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conversion"
)

func Test_cleanupManagedFieldsAndAnnotation(t *testing.T) {
	rawManagedFieldWithAnnotation := `{"f:metadata":{"f:annotations":{"f:topology.cluster.x-k8s.io/dry-run":{},"f:cluster.x-k8s.io/conversion-data":{}}}}`
	rawManagedFieldWithAnnotationSpecLabels := `{"f:metadata":{"f:annotations":{"f:topology.cluster.x-k8s.io/dry-run":{},"f:cluster.x-k8s.io/conversion-data":{}},"f:labels":{}},"f:spec":{"f:foo":{}}}`
	rawManagedFieldWithSpecLabels := `{"f:metadata":{"f:labels":{}},"f:spec":{"f:foo":{}}}`

	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		wantErr bool
		want    *unstructured.Unstructured
	}{
		{
			name:    "no-op",
			obj:     newObjectBuilder().Build(),
			wantErr: false,
		},
		{
			name: "filter out dry-run annotation",
			obj: newObjectBuilder().
				WithAnnotation(clusterv1.TopologyDryRunAnnotation, "").
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				Build(),
		},
		{
			name: "filter out conversion annotation",
			obj: newObjectBuilder().
				WithAnnotation(conversion.DataAnnotation, "").
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				Build(),
		},
		{
			name: "managedFields: should drop managed fields of other manager",
			obj: newObjectBuilder().
				WithManagedFieldsEntry("other", "", metav1.ManagedFieldsOperationApply, []byte(`{}`), nil).
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				Build(),
		},
		{
			name: "managedFields: should drop managed fields of a subresource",
			obj: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "status", metav1.ManagedFieldsOperationApply, []byte(`{}`), nil).
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				Build(),
		},
		{
			name: "managedFields: should drop managed fields of another operation",
			obj: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationUpdate, []byte(`{}`), nil).
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				Build(),
		},
		{
			name: "managedFields: cleanup up the managed field entry",
			obj: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(rawManagedFieldWithAnnotation), nil).
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(`{}`), nil).
				Build(),
		},
		{
			name: "managedFields: cleanup the managed field entry and preserve other ownership",
			obj: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(rawManagedFieldWithAnnotationSpecLabels), nil).
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(rawManagedFieldWithSpecLabels), nil).
				Build(),
		},
		{
			name: "managedFields: remove time",
			obj: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(`{}`), &metav1.Time{Time: time.Time{}}).
				Build(),
			wantErr: false,
			want: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(`{}`), nil).
				Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if err := cleanupManagedFieldsAndAnnotation(tt.obj); (err != nil) != tt.wantErr {
				t.Errorf("cleanupManagedFieldsAndAnnotation() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want != nil {
				g.Expect(tt.obj).To(BeEquivalentTo(tt.want))
			}
		})
	}
}

type objectBuilder struct {
	u *unstructured.Unstructured
}

func newObjectBuilder() objectBuilder {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetName("test")
	u.SetNamespace("default")

	return objectBuilder{
		u: u,
	}
}

func (b objectBuilder) DeepCopy() objectBuilder {
	return objectBuilder{b.u.DeepCopy()}
}

func (b objectBuilder) Build() *unstructured.Unstructured {
	// Setting an empty managed field array if no managed field is set so there is
	// no difference between an object which never had a managed field and one from
	// which all managed field entries were removed.
	if b.u.GetManagedFields() == nil {
		b.u.SetManagedFields([]metav1.ManagedFieldsEntry{})
	}
	return b.u.DeepCopy()
}

func (b objectBuilder) WithAnnotation(k, v string) objectBuilder {
	annotations := b.u.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[k] = v
	b.u.SetAnnotations(annotations)
	return b
}

func (b objectBuilder) WithManagedFieldsEntry(manager, subresource string, operation metav1.ManagedFieldsOperationType, fieldsV1 []byte, time *metav1.Time) objectBuilder {
	managedFields := append(b.u.GetManagedFields(), metav1.ManagedFieldsEntry{
		Manager:     manager,
		Operation:   operation,
		Subresource: subresource,
		FieldsV1:    &metav1.FieldsV1{Raw: fieldsV1},
		Time:        time,
	})
	b.u.SetManagedFields(managedFields)
	return b
}
