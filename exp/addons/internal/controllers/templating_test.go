/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	testClusterName = "test-cluster"
	testNamespace   = "testing"
)

func Test_renderTemplates(t *testing.T) {
	testCluster := makeTestCluster(
		func(cl *clusterv1.Cluster) {
			cl.ObjectMeta.Annotations = map[string]string{"cluster.x-k8s.io/my-test": "just-a-value"}
			cl.ObjectMeta.Labels = map[string]string{"cluster.x-k8s.io/my-label": "label-value"}
		},
	)
	renderTests := []struct {
		name string
		obj  *unstructured.Unstructured
		want *unstructured.Unstructured
	}{
		{"no template values", makeTestConfigMap("testing"), makeTestConfigMap("testing")},
		{"annotation values", makeTestConfigMap(`{{ annotation "cluster.x-k8s.io/my-test" }}`), makeTestConfigMap("just-a-value")},
		{"label values", makeTestConfigMap(`{{ label "cluster.x-k8s.io/my-label" }}`), makeTestConfigMap("label-value")},
		{"data value", makeTestConfigMap(`{{ .ClusterName }}`), makeTestConfigMap(testCluster.ObjectMeta.Name)},
		{"missing element", makeTestConfigMap(`{{ label "unknown" }}`), makeTestConfigMap("")},
	}

	for _, tt := range renderTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			rendered, err := renderTemplates(testCluster, tt.obj)

			g.Expect(err).To(Succeed())
			g.Expect(rendered).To(Equal(tt.want))
		})
	}
}

func Test_renderTemplates_with_errors(t *testing.T) {
	testCluster := makeTestCluster(
		func(cl *clusterv1.Cluster) {
			cl.ObjectMeta.Annotations = map[string]string{"cluster.x-k8s.io/my-test": "!!int"}
		},
	)
	renderTests := []struct {
		name    string
		obj     *unstructured.Unstructured
		wantErr string
	}{
		{"invalid syntax", makeTestConfigMap("{{ abels}"), "failed to parse template: template: job"},
		{"invalid template", makeTestConfigMap("{{ annotation }}"), "failed to execute template.*wrong number of args for annotation"},
	}

	for _, tt := range renderTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := renderTemplates(testCluster, tt.obj)

			g.Expect(err).To(MatchErrorRegex(tt.wantErr))
		})
	}
}

func makeTestConfigMap(s string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": testNamespace,
			},
			"data": map[string]interface{}{
				"key": s,
			},
		},
	}
}

func makeTestCluster(opts ...func(*clusterv1.Cluster)) *clusterv1.Cluster {
	c := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
		},
		Spec: clusterv1.ClusterSpec{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func MatchErrorRegex(expected string) types.GomegaMatcher {
	return &matchErrorRegex{
		expected: expected,
	}
}

type matchErrorRegex struct {
	expected string
}

func (matcher *matchErrorRegex) Match(actual interface{}) (bool, error) {
	matchErr, ok := actual.(error)
	if !ok {
		return false, fmt.Errorf("MatchErrorRegex matcher expects an error")
	}

	match, err := regexp.MatchString(matcher.expected, matchErr.Error())
	if err != nil {
		return false, err
	}
	return match, nil
}

func (matcher *matchErrorRegex) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %v to match %s", actual, matcher.expected)
}

func (matcher *matchErrorRegex) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected %v to NOT match %s", actual, matcher.expected)
}
