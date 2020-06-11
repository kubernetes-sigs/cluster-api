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

package conditions

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestHasSameState(t *testing.T) {
	g := NewWithT(t)

	// same condition
	falseInfo2 := falseInfo1.DeepCopy()
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeTrue())

	// different LastTransitionTime does not impact state
	falseInfo2 = falseInfo1.DeepCopy()
	falseInfo2.LastTransitionTime = metav1.NewTime(time.Date(1900, time.November, 10, 23, 0, 0, 0, time.UTC))
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeTrue())

	// different Type, Status, Reason, Severity and Message determine different state
	falseInfo2 = falseInfo1.DeepCopy()
	falseInfo2.Type = "another type"
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeFalse())

	falseInfo2 = falseInfo1.DeepCopy()
	falseInfo2.Status = corev1.ConditionTrue
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeFalse())

	falseInfo2 = falseInfo1.DeepCopy()
	falseInfo2.Severity = clusterv1.ConditionSeverityWarning
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeFalse())

	falseInfo2 = falseInfo1.DeepCopy()
	falseInfo2.Reason = "another severity"
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeFalse())

	falseInfo2 = falseInfo1.DeepCopy()
	falseInfo2.Message = "another message"
	g.Expect(hasSameState(falseInfo1, falseInfo2)).To(BeFalse())
}

func TestLexicographicLess(t *testing.T) {
	g := NewWithT(t)

	// alphabetical order of Type is respected
	a := TrueCondition("A")
	b := TrueCondition("B")
	g.Expect(lexicographicLess(a, b)).To(BeTrue())

	a = TrueCondition("B")
	b = TrueCondition("A")
	g.Expect(lexicographicLess(a, b)).To(BeFalse())

	// Ready condition is treated as an exception and always goes first
	a = TrueCondition(clusterv1.ReadyCondition)
	b = TrueCondition("A")
	g.Expect(lexicographicLess(a, b)).To(BeTrue())

	a = TrueCondition("A")
	b = TrueCondition(clusterv1.ReadyCondition)
	g.Expect(lexicographicLess(a, b)).To(BeFalse())
}

func TestSet(t *testing.T) {
	a := TrueCondition("a")
	b := TrueCondition("b")
	ready := TrueCondition(clusterv1.ReadyCondition)

	tests := []struct {
		name      string
		to        Setter
		condition *clusterv1.Condition
		want      clusterv1.Conditions
	}{
		{
			name:      "Set adds a condition",
			to:        setterWithConditions(),
			condition: a,
			want:      conditionList(a),
		},
		{
			name:      "Set adds more conditions",
			to:        setterWithConditions(a),
			condition: b,
			want:      conditionList(a, b),
		},
		{
			name:      "Set does not duplicate existing conditions",
			to:        setterWithConditions(a, b),
			condition: a,
			want:      conditionList(a, b),
		},
		{
			name:      "Set sorts conditions in lexicographic order",
			to:        setterWithConditions(b, a),
			condition: ready,
			want:      conditionList(ready, a, b),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			Set(tt.to, tt.condition)

			g.Expect(tt.to.GetConditions()).To(haveSameConditionsOf(tt.want))
		})
	}
}

func TestSetLastTransitionTime(t *testing.T) {
	x := metav1.Date(2012, time.January, 1, 12, 15, 30, 5e8, time.UTC)

	foo := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityInfo, "message foo")
	fooWithLastTransitionTime := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityInfo, "message foo")
	fooWithLastTransitionTime.LastTransitionTime = x
	fooWithAnotherState := TrueCondition("foo")

	tests := []struct {
		name                    string
		to                      Setter
		new                     *clusterv1.Condition
		LastTransitionTimeCheck func(*WithT, metav1.Time)
	}{
		{
			name: "Set a condition that does not exists should set the last transition time if not defined",
			to:   setterWithConditions(),
			new:  foo,
			LastTransitionTimeCheck: func(g *WithT, lastTransitionTime metav1.Time) {
				g.Expect(lastTransitionTime).ToNot(BeZero())
			},
		},
		{
			name: "Set a condition that does not exists should preserve the last transition time if defined",
			to:   setterWithConditions(),
			new:  fooWithLastTransitionTime,
			LastTransitionTimeCheck: func(g *WithT, lastTransitionTime metav1.Time) {
				g.Expect(lastTransitionTime).To(Equal(x))
			},
		},
		{
			name: "Set a condition that already exists with the same state should preserves the last transition time",
			to:   setterWithConditions(fooWithLastTransitionTime),
			new:  foo,
			LastTransitionTimeCheck: func(g *WithT, lastTransitionTime metav1.Time) {
				g.Expect(lastTransitionTime).To(Equal(x))
			},
		},
		{
			name: "Set a condition that already exists but with different state should changes the last transition time",
			to:   setterWithConditions(fooWithLastTransitionTime),
			new:  fooWithAnotherState,
			LastTransitionTimeCheck: func(g *WithT, lastTransitionTime metav1.Time) {
				g.Expect(lastTransitionTime).ToNot(Equal(x))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			Set(tt.to, tt.new)

			tt.LastTransitionTimeCheck(g, Get(tt.to, "foo").LastTransitionTime)
		})
	}
}

func TestMarkMethods(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{}

	// test MarkTrue
	MarkTrue(cluster, "conditionFoo")
	g.Expect(Get(cluster, "conditionFoo")).To(haveSameStateOf(&clusterv1.Condition{
		Type:   "conditionFoo",
		Status: corev1.ConditionTrue,
	}))

	// test MarkFalse
	MarkFalse(cluster, "conditionBar", "reasonBar", clusterv1.ConditionSeverityError, "messageBar")
	g.Expect(Get(cluster, "conditionBar")).To(haveSameStateOf(&clusterv1.Condition{
		Type:     "conditionBar",
		Status:   corev1.ConditionFalse,
		Severity: clusterv1.ConditionSeverityError,
		Reason:   "reasonBar",
		Message:  "messageBar",
	}))

	// test MarkUnknown
	MarkUnknown(cluster, "conditionBaz", "reasonBaz", "messageBaz")
	g.Expect(Get(cluster, "conditionBaz")).To(haveSameStateOf(&clusterv1.Condition{
		Type:    "conditionBaz",
		Status:  corev1.ConditionUnknown,
		Reason:  "reasonBaz",
		Message: "messageBaz",
	}))
}

func TestSetSummary(t *testing.T) {
	g := NewWithT(t)
	target := setterWithConditions(TrueCondition("foo"))

	SetSummary(target)

	g.Expect(Has(target, clusterv1.ReadyCondition)).To(BeTrue())
}

func TestSetMirror(t *testing.T) {
	g := NewWithT(t)
	source := getterWithConditions(TrueCondition(clusterv1.ReadyCondition))
	target := setterWithConditions()

	SetMirror(target, "foo", source)

	g.Expect(Has(target, "foo")).To(BeTrue())
}

func TestSetAggregate(t *testing.T) {
	g := NewWithT(t)
	source1 := getterWithConditions(TrueCondition(clusterv1.ReadyCondition))
	source2 := getterWithConditions(TrueCondition(clusterv1.ReadyCondition))
	target := setterWithConditions()

	SetAggregate(target, "foo", []Getter{source1, source2})

	g.Expect(Has(target, "foo")).To(BeTrue())
}

func setterWithConditions(conditions ...*clusterv1.Condition) Setter {
	obj := &clusterv1.Cluster{}
	obj.SetConditions(conditionList(conditions...))
	return obj
}

func haveSameConditionsOf(expected clusterv1.Conditions) types.GomegaMatcher {
	return &ConditionsMatcher{
		Expected: expected,
	}
}

type ConditionsMatcher struct {
	Expected clusterv1.Conditions
}

func (matcher *ConditionsMatcher) Match(actual interface{}) (success bool, err error) {
	actualConditions, ok := actual.(clusterv1.Conditions)
	if !ok {
		return false, errors.New("Value should be a conditions list")
	}

	if len(actualConditions) != len(matcher.Expected) {
		return false, nil
	}

	for i := range actualConditions {
		if !hasSameState(&actualConditions[i], &matcher.Expected[i]) {
			return false, nil
		}
	}
	return true, nil
}

func (matcher *ConditionsMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to have the same conditions of", matcher.Expected)
}
func (matcher *ConditionsMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to have the same conditions of", matcher.Expected)
}
