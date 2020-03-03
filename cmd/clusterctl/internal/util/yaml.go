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

package util

import (
	"bufio"
	"bytes"
	"io"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

var yamlSeparator = []byte("---")

// JoinYaml takes a list of YAML files and join them ensuring
// each YAML that the yaml separator goes on a new line by adding \n where necessary
func JoinYaml(yamls ...[]byte) []byte {
	var cr = []byte("\n")
	var b [][]byte //nolint
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

// ToUnstructured takes a YAML and converts it to a list of Unstructured objects
func ToUnstructured(rawyaml []byte) ([]unstructured.Unstructured, error) {
	var ret []unstructured.Unstructured //nolint

	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(rawyaml)))
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
			return nil, errors.Wrapf(err, "failed to unmarshal yaml fragment: %q", string(b))
		}

		var u unstructured.Unstructured
		u.SetUnstructuredContent(m)

		// Ignore empty objects.
		// Empty objects are generated if there are weird things in manifest files like e.g. two --- in a row without a yaml doc in the middle
		if u.Object == nil {
			continue
		}

		ret = append(ret, u)
	}

	return ret, nil
}

// FromUnstructured takes a list of Unstructured objects and converts it into a YAML
func FromUnstructured(objs []unstructured.Unstructured) ([]byte, error) {
	var ret [][]byte //nolint
	for _, o := range objs {
		content, err := yaml.Marshal(o.UnstructuredContent())
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal yaml for %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		}
		ret = append(ret, content)
	}

	return JoinYaml(ret...), nil
}
