/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

func TestConvertArgs(t *testing.T) {
	g := NewWithT(t)

	argList := []Arg{
		{
			Name:  "foo",
			Value: ptr.To("1"),
		},
		{
			Name:  "bar",
			Value: ptr.To("1"),
		},
		{
			Name:  "foo",
			Value: ptr.To("2"),
		},
	}
	argMap := ConvertFromArgs(argList)

	argList = ConvertToArgs(argMap)
	g.Expect(argList).To(HaveLen(2))
	g.Expect(argList).To(ConsistOf(
		Arg{
			Name:  "bar",
			Value: ptr.To("1"),
		},
		Arg{
			Name:  "foo",
			Value: ptr.To("2"),
		},
	))
}
