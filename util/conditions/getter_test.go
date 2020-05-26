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

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

var (
	nil1          *clusterv1.Condition
	true1         = TrueCondition("true1")
	unknown1      = UnknownCondition("unknown1", "reason unknown1", "message unknown1")
	falseInfo1    = FalseCondition("falseInfo1", "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	falseWarning1 = FalseCondition("falseWarning1", "reason falseWarning1", clusterv1.ConditionSeverityWarning, "message falseWarning1")
	falseError1   = FalseCondition("falseError1", "reason falseError1", clusterv1.ConditionSeverityError, "message falseError1")
)

func TestGetAndHas(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{}

	g.Expect(Has(cluster, "conditionBaz")).To(BeFalse())
	g.Expect(Get(cluster, "conditionBaz")).To(BeNil())

	cluster.SetConditions(conditionList(TrueCondition("conditionBaz")))

	g.Expect(Has(cluster, "conditionBaz")).To(BeTrue())
	g.Expect(Get(cluster, "conditionBaz")).To(haveSameStateOf(TrueCondition("conditionBaz")))
}

func TestIsMethods(t *testing.T) {
	g := NewWithT(t)

	obj := getterWithConditions(nil1, true1, unknown1, falseInfo1, falseWarning1, falseError1)

	// test isTrue
	g.Expect(IsTrue(obj, "nil1")).To(BeFalse())
	g.Expect(IsTrue(obj, "true1")).To(BeTrue())
	g.Expect(IsTrue(obj, "falseInfo1")).To(BeFalse())
	g.Expect(IsTrue(obj, "unknown1")).To(BeFalse())

	// test isFalse
	g.Expect(IsFalse(obj, "nil1")).To(BeFalse())
	g.Expect(IsFalse(obj, "true1")).To(BeFalse())
	g.Expect(IsFalse(obj, "falseInfo1")).To(BeTrue())
	g.Expect(IsFalse(obj, "unknown1")).To(BeFalse())

	// test isUnknown
	g.Expect(IsUnknown(obj, "nil1")).To(BeTrue())
	g.Expect(IsUnknown(obj, "true1")).To(BeFalse())
	g.Expect(IsUnknown(obj, "falseInfo1")).To(BeFalse())
	g.Expect(IsUnknown(obj, "unknown1")).To(BeTrue())

	// test GetReason
	g.Expect(GetReason(obj, "nil1")).To(Equal(""))
	g.Expect(GetReason(obj, "falseInfo1")).To(Equal("reason falseInfo1"))

	// test GetMessage
	g.Expect(GetMessage(obj, "nil1")).To(Equal(""))
	g.Expect(GetMessage(obj, "falseInfo1")).To(Equal("message falseInfo1"))

	// test GetSeverity
	g.Expect(GetSeverity(obj, "nil1")).To(BeNil())
	severity := GetSeverity(obj, "falseInfo1")
	expectedSeverity := clusterv1.ConditionSeverityInfo
	g.Expect(severity).To(Equal(&expectedSeverity))

	// test GetMessage
	g.Expect(GetLastTransitionTime(obj, "nil1")).To(BeNil())
	g.Expect(GetLastTransitionTime(obj, "falseInfo1")).ToNot(BeNil())
}

func getterWithConditions(conditions ...*clusterv1.Condition) Getter {
	obj := &clusterv1.Cluster{}
	obj.SetConditions(conditionList(conditions...))
	return obj
}

func conditionList(conditions ...*clusterv1.Condition) clusterv1.Conditions {
	cs := clusterv1.Conditions{}
	for _, x := range conditions {
		if x != nil {
			cs = append(cs, *x)
		}
	}
	return cs
}

func haveSameStateOf(expected *clusterv1.Condition) types.GomegaMatcher {
	return &ConditionMatcher{
		Expected: expected,
	}
}

type ConditionMatcher struct {
	Expected *clusterv1.Condition
}

func (matcher *ConditionMatcher) Match(actual interface{}) (success bool, err error) {
	actualCondition, ok := actual.(*clusterv1.Condition)
	if !ok {
		return false, errors.New("Value should be a condition")
	}

	return hasSameState(actualCondition, matcher.Expected), nil
}

func (matcher *ConditionMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "to have the same state of", matcher.Expected)
}
func (matcher *ConditionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, "not to have the same state of", matcher.Expected)
}
