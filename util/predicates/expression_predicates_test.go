/*
Copyright 2023 The Kubernetes Authors.

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

package predicates

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	logger = logr.New(klogr.New().GetSink())
)

func TestMatchExpression(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name       string
		obj        client.Object
		expression string
		err        string
		matcherErr string
		matched    bool
	}{
		{
			name:       "Object with stub expression should be accepted",
			expression: "true",
			obj:        &corev1.Node{},
			matched:    true,
		},
		{
			name:       "Empty object is rejected",
			expression: "true",
			matched:    false,
		},
		{
			name:       "Object with expression evaluated to false should be rejected",
			expression: "false",
			obj:        &corev1.Node{},
			matched:    false,
		},
		{
			name:       "Object with self expression should be matched",
			expression: "self.metadata.name == 'test'",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			matched: true,
		},
		{
			name:       "Object with matching expression on labels should be matched",
			expression: "'label' in self.metadata.labels",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"label": "something",
					},
				},
			},
			matched: true,
		},
		{
			name:       "Logical expression with correctly handled absent key should be matched",
			expression: "!has(self.metadata.labels) || !('label' in self.metadata.labels)",
			obj:        &corev1.Node{},
			matched:    true,
		},
		{
			name:       "Expression error should prevent object from matching",
			expression: "'label' in self.metadata.labels",
			obj:        &corev1.Node{},
			err:        "no such key: labels",
			matched:    false,
		},
		{
			name:       "Object with non boolean expression should not be matched",
			expression: "self",
			obj:        &corev1.Node{},
			matcherErr: "Expression should evaluate to boolean value",
			matched:    false,
		},
		{
			name:       "Object with expression on unknown variable should not be matched",
			expression: "other",
			obj:        &corev1.Node{},
			matcherErr: "undeclared reference to 'other'",
			matched:    false,
		},
	}

	for _, tt := range testcases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := InitExpressionMatcher(logger, tt.expression)
			if tt.matcherErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(ContainSubstring(tt.matcherErr)))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			result, err := GetExpressionMatcher().matchesExpression(logger, tt.obj)
			if tt.err != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(ContainSubstring(tt.err)))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tt.matched).To(Equal(result))
			g.Expect(tt.matched).To(Equal(GetExpressionMatcher().matches(logger, tt.obj)))
		})
	}
}

func TestEmptyMatcher(t *testing.T) {
	expressionMatcher = nil
	g := NewWithT(t)
	g.Expect(InitExpressionMatcher(logger, "")).ToNot(HaveOccurred())
	g.Expect(GetExpressionMatcher()).To(BeNil())
}

func TestMachineReconciliationWithComplexExpression(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "watch-expression-namespace")
	g.Expect(err).ToNot(HaveOccurred())
	defer func() {
		g.Expect(env.Delete(ctx, ns)).To(Succeed())
	}()

	original := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "cluster",
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: pointer.String("my-data-secret"),
			},
		},
	}

	t.Run("Machine with annotations present should not be reconciled according to expression", func(t *testing.T) {
		g := NewWithT(t)
		annotatedObj := original.DeepCopy()
		annotatedObj.Name = "annotated"

		annotatedObj.SetAnnotations(map[string]string{
			"some": "annotation",
		})

		g.Expect(env.Create(ctx, annotatedObj)).To(Succeed())
		g.Eventually(func() error {
			return env.Get(ctx, client.ObjectKeyFromObject(annotatedObj), annotatedObj)
		}, timeout).Should(Succeed())

		g.Consistently(func() bool {
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(annotatedObj), annotatedObj)).To(Succeed())
			return annotatedObj.Status.BootstrapReady
		}, timeout).Should(BeFalse())
	})

	t.Run("Machine with one label but not another will be reconciled", func(t *testing.T) {
		g := NewWithT(t)
		oneLabel := original.DeepCopy()
		oneLabel.Name = "one-label"
		oneLabel.SetLabels(map[string]string{
			"one": "",
		})

		g.Expect(env.Create(ctx, oneLabel)).To(Succeed())
		g.Eventually(func() error {
			return env.Get(ctx, client.ObjectKeyFromObject(oneLabel), oneLabel)
		}, timeout).Should(Succeed())

		g.Eventually(func() bool {
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(oneLabel), oneLabel)).To(Succeed())
			return oneLabel.Status.BootstrapReady
		}, timeout).Should(BeTrue())
	})

	t.Run("Machine with another label will not be reconciled", func(t *testing.T) {
		g := NewWithT(t)
		anotherLabel := original.DeepCopy()
		anotherLabel.Name = "another-label"
		anotherLabel.SetLabels(map[string]string{
			"one":     "",
			"another": "",
		})

		g.Expect(env.Create(ctx, anotherLabel)).To(Succeed())
		g.Eventually(func() error {
			return env.Get(ctx, client.ObjectKeyFromObject(anotherLabel), anotherLabel)
		}, timeout).Should(Succeed())

		g.Consistently(func() bool {
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(anotherLabel), anotherLabel)).To(Succeed())
			return anotherLabel.Status.BootstrapReady
		}, timeout).Should(BeFalse())
	})
}
