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
	"bytes"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	for _, data := range bytes.Split(rawyaml, yamlSeparator) {
		var m map[string]interface{}
		err := yaml.Unmarshal(data, &m)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal yaml fragment: %q", string(data))
		}

		var u unstructured.Unstructured
		u.SetUnstructuredContent(m)

		ret = append(ret, u)
	}

	return ret, nil
}
