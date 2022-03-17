package catalog

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	group   = "test.cluster.x-k8s.io"
	version = "v1alpha1"
)

type svc struct{}

type input struct {
	metav1.TypeMeta `json:",inline"`

	Second string
	First  int
}

func (in *input) DeepCopyObject() runtime.Object {
	panic("implement me!")
}

type output struct {
	metav1.TypeMeta `json:",inline"`

	Second string
	First  int
}

func (in *output) DeepCopyObject() runtime.Object {
	panic("implement me!")
}

func TestCatalog_addTypeToScheme(t *testing.T) {
	tests := []struct {
		name string
		gv   schema.GroupVersion
		obj  runtime.Object
		want GroupVersionKind
	}{
		{
			name: "",
			gv:   schema.GroupVersion{Group: group, Version: version},
			obj:  &input{},
			want: GroupVersionKind{Group: group, Version: version, Kind: "request"},
		},
		// TODO: test no pointer, no struct, no version, no group
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := New()
			got := c.addTypeToScheme(tt.gv, tt.obj)

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestCatalog_ObjectKind(t *testing.T) {
	tests := []struct {
		name string
		gv   GroupVersion
		obj  runtime.Object
		want GroupVersionKind
	}{
		{
			name: "",
			gv:   GroupVersion{Group: group, Version: version},
			obj:  &input{},
			want: GroupVersionKind{Group: group, Version: version, Kind: "Input"},
		},
		// TODO: test multiple gvk
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := New()
			c.scheme.AddKnownTypes(tt.gv.toScheme(), tt.obj)

			got, err := c.ObjectKind(tt.obj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestCatalog_RegisterService(t *testing.T) {
	tests := []struct {
		name   string
		gv    GroupVersion
		svc   Hook
		input runtime.Object
		output runtime.Object
	}{
		{
			name:   "",
			gv:     GroupVersion{Group: group, Version: version},
			svc:    &svc{},
			input:  &input{},
			output: &output{},
		},
		// TODO: test no pointer, no struct, double registration, wrong request/response
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// g := NewWithT(t)

			c := New()

			c.RegisterHook(tt.gv, tt.svc, tt.input, tt.output)

			// TODO: assertions
		})
	}
}
