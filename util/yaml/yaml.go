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

// Package yaml implements yaml utility functions.
package yaml

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
	apiyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
)

// ExtractClusterReferences returns the references in a Cluster object.
func ExtractClusterReferences(out *ParseOutput, c *clusterv1.Cluster) (res []*unstructured.Unstructured) {
	if c.Spec.InfrastructureRef == nil {
		return nil
	}
	if obj := out.FindUnstructuredReference(c.Spec.InfrastructureRef); obj != nil {
		res = append(res, obj)
	}
	return
}

// ExtractMachineReferences returns the references in a Machine object.
func ExtractMachineReferences(out *ParseOutput, m *clusterv1.Machine) (res []*unstructured.Unstructured) {
	if obj := out.FindUnstructuredReference(&m.Spec.InfrastructureRef); obj != nil {
		res = append(res, obj)
	}
	if m.Spec.Bootstrap.ConfigRef != nil {
		if obj := out.FindUnstructuredReference(m.Spec.Bootstrap.ConfigRef); obj != nil {
			res = append(res, obj)
		}
	}
	return
}

// ParseOutput is the output given from the Parse function.
type ParseOutput struct {
	Clusters            []*clusterv1.Cluster
	Machines            []*clusterv1.Machine
	MachineSets         []*clusterv1.MachineSet
	MachineDeployments  []*clusterv1.MachineDeployment
	UnstructuredObjects []*unstructured.Unstructured
}

// Add adds the other ParseOutput slices to this instance.
func (p *ParseOutput) Add(o *ParseOutput) *ParseOutput {
	p.Clusters = append(p.Clusters, o.Clusters...)
	p.Machines = append(p.Machines, o.Machines...)
	p.MachineSets = append(p.MachineSets, o.MachineSets...)
	p.MachineDeployments = append(p.MachineDeployments, o.MachineDeployments...)
	p.UnstructuredObjects = append(p.UnstructuredObjects, o.UnstructuredObjects...)
	return p
}

// FindUnstructuredReference takes in an ObjectReference and tries to find an Unstructured object.
func (p *ParseOutput) FindUnstructuredReference(ref *corev1.ObjectReference) *unstructured.Unstructured {
	for _, obj := range p.UnstructuredObjects {
		if obj.GroupVersionKind() == ref.GroupVersionKind() &&
			ref.Namespace == obj.GetNamespace() &&
			ref.Name == obj.GetName() {
			return obj
		}
	}
	return nil
}

// ParseInput is an input struct for the Parse function.
type ParseInput struct {
	File string
}

// Parse extracts runtime objects from a file.
func Parse(input ParseInput) (*ParseOutput, error) {
	output := &ParseOutput{}

	// Open the input file.
	reader, err := os.Open(input.File)
	if err != nil {
		return nil, err
	}

	// Create a new decoder.
	decoder := NewYAMLDecoder(reader)
	defer decoder.Close()

	for {
		u := &unstructured.Unstructured{}
		_, gvk, err := decoder.Decode(nil, u)
		if err == io.EOF {
			break
		}
		if runtime.IsNotRegisteredError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		switch gvk.Kind {
		case "Cluster":
			obj := &clusterv1.Cluster{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				return nil, errors.Wrapf(err, "cannot convert object to %s", gvk.Kind)
			}
			output.Clusters = append(output.Clusters, obj)
		case "Machine":
			obj := &clusterv1.Machine{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				return nil, errors.Wrapf(err, "cannot convert object to %s", gvk.Kind)
			}
			output.Machines = append(output.Machines, obj)
		case "MachineSet":
			obj := &clusterv1.MachineSet{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				return nil, errors.Wrapf(err, "cannot convert object to %s", gvk.Kind)
			}
			output.MachineSets = append(output.MachineSets, obj)
		case "MachineDeployment":
			obj := &clusterv1.MachineDeployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				return nil, errors.Wrapf(err, "cannot convert object to %s", gvk.Kind)
			}
			output.MachineDeployments = append(output.MachineDeployments, obj)
		default:
			output.UnstructuredObjects = append(output.UnstructuredObjects, u)
		}
	}

	return output, nil
}

type yamlDecoder struct {
	reader  *apiyaml.YAMLReader
	decoder runtime.Decoder
	close   func() error
}

func (d *yamlDecoder) Decode(defaults *schema.GroupVersionKind, into runtime.Object) (runtime.Object, *schema.GroupVersionKind, error) {
	for {
		doc, err := d.reader.Read()
		if err != nil {
			return nil, nil, err
		}

		//  Skip over empty documents, i.e. a leading `---`
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		return d.decoder.Decode(doc, defaults, into)
	}
}

func (d *yamlDecoder) Close() error {
	return d.close()
}

// NewYAMLDecoder returns a new streaming Decoded that supports YAML.
func NewYAMLDecoder(r io.ReadCloser) streaming.Decoder {
	return &yamlDecoder{
		reader:  apiyaml.NewYAMLReader(bufio.NewReader(r)),
		decoder: scheme.Codecs.UniversalDeserializer(),
		close:   r.Close,
	}
}

// ToUnstructured takes a YAML and converts it to a list of Unstructured objects.
func ToUnstructured(rawyaml []byte) ([]unstructured.Unstructured, error) {
	var ret []unstructured.Unstructured

	reader := apiyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(rawyaml)))
	count := 1
	for {
		// Read one YAML document at a time, until io.EOF is returned
		b, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrapf(err, "failed to read yaml")
		}
		if len(b) == 0 {
			break
		}

		var m map[string]interface{}
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal the %s yaml document: %q", util.Ordinalize(count), string(b))
		}

		var u unstructured.Unstructured
		u.SetUnstructuredContent(m)

		// Ignore empty objects.
		// Empty objects are generated if there are weird things in manifest files like e.g. two --- in a row without a yaml doc in the middle
		if u.Object == nil {
			continue
		}

		ret = append(ret, u)
		count++
	}

	return ret, nil
}

// JoinYaml takes a list of YAML files and join them ensuring
// each YAML that the yaml separator goes on a new line by adding \n where necessary.
func JoinYaml(yamls ...[]byte) []byte {
	var yamlSeparator = []byte("---")

	var cr = []byte("\n")
	var b [][]byte //nolint:prealloc
	for _, y := range yamls {
		if !bytes.HasPrefix(y, cr) {
			y = append(cr, y...)
		}
		if !bytes.HasSuffix(y, cr) {
			y = append(y, cr...)
		}
		b = append(b, y)
	}

	r := bytes.Join(b, yamlSeparator)
	r = bytes.TrimPrefix(r, cr)
	r = bytes.TrimSuffix(r, cr)

	return r
}

// FromUnstructured takes a list of Unstructured objects and converts it into a YAML.
func FromUnstructured(objs []unstructured.Unstructured) ([]byte, error) {
	var ret [][]byte //nolint:prealloc
	for _, o := range objs {
		content, err := yaml.Marshal(o.UnstructuredContent())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal yaml for %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		}
		ret = append(ret, content)
	}

	return JoinYaml(ret...), nil
}

// Raw returns un-indented yaml string; it also remove the first empty line, if any.
// While writing yaml, always use space instead of tabs for indentation.
func Raw(raw string) string {
	return strings.TrimPrefix(heredoc.Doc(raw), "\n")
}
