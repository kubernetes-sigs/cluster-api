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

package variables

import (
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestMergeVariables(t *testing.T) {
	t.Run("Merge variables", func(t *testing.T) {
		g := NewWithT(t)

		m, err := MergeVariableMaps(
			map[string]apiextensionsv1.JSON{
				BuiltinsName: {Raw: []byte(`{"cluster":{"name":"cluster-name","namespace":"default","topology":{"class":"clusterClass1","version":"v1.21.1"}}}`)},
				"a":          {Raw: []byte("a-different")},
				"c":          {Raw: []byte("c")},
			},
			map[string]apiextensionsv1.JSON{
				// Verify that builtin variables are merged correctly and
				// the latter variables take precedent ("cluster-name-overwrite").
				BuiltinsName: {Raw: []byte(`{"controlPlane":{"replicas":3},"cluster":{"name":"cluster-name-overwrite"}}`)},
				"a":          {Raw: []byte("a")},
				"b":          {Raw: []byte("b")},
			},
		)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(m).To(HaveKeyWithValue(BuiltinsName, apiextensionsv1.JSON{Raw: []byte(`{"cluster":{"name":"cluster-name-overwrite","namespace":"default","topology":{"version":"v1.21.1","class":"clusterClass1"}},"controlPlane":{"replicas":3}}`)}))
		g.Expect(m).To(HaveKeyWithValue("a", apiextensionsv1.JSON{Raw: []byte("a")}))
		g.Expect(m).To(HaveKeyWithValue("b", apiextensionsv1.JSON{Raw: []byte("b")}))
		g.Expect(m).To(HaveKeyWithValue("c", apiextensionsv1.JSON{Raw: []byte("c")}))
	})
}
