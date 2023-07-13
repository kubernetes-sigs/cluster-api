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
	"sigs.k8s.io/controller-runtime/pkg/event"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	logger = logr.New(klogr.New().GetSink())
)

func TestComposeExpression(t *testing.T) {
	g := NewWithT(t)

	g.Expect(ComposeFilterExpression("!key", "")).To(Equal("!key"))
	g.Expect(ComposeFilterExpression("", "")).To(Equal(""))
	g.Expect(ComposeFilterExpression("", "value")).To(Equal("cluster.x-k8s.io/watch-filter = value"))
}

func TestMatchExpression(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name       string
		obj        client.Object
		expression string
		err        string
		matched    bool
	}{
		{
			name:       "Set based expression should be accepted",
			expression: "key notin (value),key in (otherValue)",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"key": "otherValue",
					},
				},
			},
			matched: true,
		},
		{
			name:       "Matching expression on label keys",
			expression: "label",
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
			name:       "Label key not present should be matched",
			expression: "!label",
			obj:        &corev1.Node{},
			matched:    true,
		},
		{
			name:       "Empty expression should always match object",
			expression: "",
			obj:        &corev1.Node{},
			matched:    true,
		},
		{
			name:       "Equality based requirement is supported",
			expression: "key = value",
			obj:        &corev1.Node{},
			matched:    false,
		},
		{
			name:       "Compose an expression from watchFilter value matches object",
			expression: ComposeFilterExpression("", "true"),
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"cluster.x-k8s.io/watch-filter": "true",
					},
				},
			},
			matched: true,
		},
		{
			name:       "Incorrect expression will error out",
			expression: "what is that?",
			err:        "couldn't parse the selector string",
		},
	}

	for _, tt := range testcases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := InitLabelMatcher(logger, tt.expression)
			if tt.err != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(ContainSubstring(tt.err)))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			// Ensure all predicate events are consistent with each other
			g.Expect(matcher.Matches(logger).Create(event.CreateEvent{Object: tt.obj})).To(Equal(tt.matched))
			g.Expect(matcher.Matches(logger).Update(event.UpdateEvent{ObjectNew: tt.obj, ObjectOld: tt.obj})).To(Equal(tt.matched))
			g.Expect(matcher.Matches(logger).Delete(event.DeleteEvent{Object: tt.obj})).To(Equal(tt.matched))
			g.Expect(matcher.Matches(logger).Generic(event.GenericEvent{Object: tt.obj})).To(Equal(tt.matched))
		})
	}
}

func TestMachineReconciliationWithComplexLabelSelector(t *testing.T) {
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "watch-label-namespace")
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

	t.Run("Machine with prohibited labels found should not be reconciled according to expression", func(t *testing.T) {
		g := NewWithT(t)
		incorrectlyLabeledObj := original.DeepCopy()
		incorrectlyLabeledObj.Name = "labeled-incorrectly"

		incorrectlyLabeledObj.SetLabels(map[string]string{
			"some": "label",
		})

		g.Expect(env.Create(ctx, incorrectlyLabeledObj)).To(Succeed())
		g.Consistently(func() error {
			return env.Get(ctx, client.ObjectKeyFromObject(incorrectlyLabeledObj), incorrectlyLabeledObj)
		}, timeout).ShouldNot(Succeed())
	})

	t.Run("Machine with only one label will not be reconciled", func(t *testing.T) {
		g := NewWithT(t)
		oneLabel := original.DeepCopy()
		oneLabel.Name = "one-label"
		oneLabel.SetLabels(map[string]string{
			"one": "",
		})

		g.Expect(env.Create(ctx, oneLabel)).To(Succeed())
		g.Consistently(func() error {
			return env.Get(ctx, client.ObjectKeyFromObject(oneLabel), oneLabel)
		}, timeout).ShouldNot(Succeed())
	})

	t.Run("Machine with both labels will be reconciled", func(t *testing.T) {
		g := NewWithT(t)
		anotherLabel := original.DeepCopy()
		anotherLabel.Name = "another-label"
		anotherLabel.SetLabels(map[string]string{
			"one":                           "",
			"cluster.x-k8s.io/watch-filter": "value",
		})

		g.Expect(env.Create(ctx, anotherLabel)).To(Succeed())
		g.Eventually(func() error {
			return env.Get(ctx, client.ObjectKeyFromObject(anotherLabel), anotherLabel)
		}, timeout).Should(Succeed())

		g.Eventually(func() bool {
			g.Expect(env.Get(ctx, client.ObjectKeyFromObject(anotherLabel), anotherLabel)).To(Succeed())
			return anotherLabel.Status.BootstrapReady
		}, timeout).Should(BeTrue())
	})
}
