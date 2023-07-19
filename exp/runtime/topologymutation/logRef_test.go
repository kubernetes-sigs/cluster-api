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

package topologymutation

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_Log(t *testing.T) {
	g := NewWithT(t)

	l := &logRef{
		Group:     "group",
		Version:   "version",
		Kind:      "kind",
		Namespace: "namespace",
		Name:      "name",
	}

	g.Expect(l.String()).To(Equal("group/version/kind/namespace/name"))
	g.Expect(l.MarshalLog()).To(BeComparableTo(logRefWithoutStringFunc(*l)))
}
