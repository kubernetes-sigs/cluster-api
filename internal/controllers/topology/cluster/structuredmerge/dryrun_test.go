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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conversion"
)

func Test_cleanupManagedFieldsAndAnnotation(t *testing.T) {
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
			want:    newObjectBuilder().Build(),
		},
		{
			name: "filter out dry-run annotation",
			obj: newObjectBuilder().
				WithAnnotation(clusterv1.TopologyDryRunAnnotation, "").
				Build(),
			wantErr: false,
			want:    newObjectBuilder().Build(),
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
			name: "remove managed field",
			obj: newObjectBuilder().
				WithManagedFieldsEntry(TopologyManagerName, "", metav1.ManagedFieldsOperationApply, []byte(`{}`), nil).
				Build(),
			wantErr: false,
			want:    newObjectBuilder().Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cleanupManagedFieldsAndAnnotations(tt.obj)
			g.Expect(tt.obj).To(BeEquivalentTo(tt.want))
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
