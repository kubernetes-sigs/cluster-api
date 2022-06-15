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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
)

func TestExtensionConfigByInjectCAFromSecretName(t *testing.T) {
	testCases := []struct {
		name     string
		object   client.Object
		expected []string
	}{
		{
			name:     "when extensionConfig has no inject annotation",
			object:   &runtimev1.ExtensionConfig{},
			expected: nil,
		},
		{
			name: "when cluster has a valid Topology",
			object: &runtimev1.ExtensionConfig{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						runtimev1.InjectCAFromSecretAnnotation: "foo/bar",
					},
				},
			},
			expected: []string{"foo/bar"},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			got := extensionConfigByInjectCAFromSecretName(test.object)
			g.Expect(got).To(Equal(test.expected))
		})
	}
}
