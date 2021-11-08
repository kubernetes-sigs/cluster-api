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

package framework_test

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cluster-api/test/framework"
)

func TestObjectToKind(t *testing.T) {
	g := NewWithT(t)

	g.Expect(framework.ObjectToKind(&hello{})).To(Equal("hello"))
}

var _ runtime.Object = &hello{}

type hello struct{}

func (*hello) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (h *hello) DeepCopyObject() runtime.Object {
	return h
}
