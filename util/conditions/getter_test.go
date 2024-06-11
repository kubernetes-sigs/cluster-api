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
	"k8s.io/apimachinery/pkg/util/sets"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	nil1          *clusterv1.Condition
	true1         = TrueCondition("true1")
	unknown1      = UnknownCondition("unknown1", "reason unknown1", "message unknown1")
	falseInfo1    = FalseCondition("falseInfo1", "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	falseWarning1 = FalseCondition("falseWarning1", "reason falseWarning1", clusterv1.ConditionSeverityWarning, "message falseWarning1")
	falseError1   = FalseCondition("falseError1", "reason falseError1", clusterv1.ConditionSeverityError, "message falseError1")

	negativePolarityConditions       = sets.New("false1-negative-polarity", "unknown1-negative-polarity", "trueInfo1-negative-polarity", "trueWarning1-negative-polarity", "trueError1-negative-polarity")
	false1WithNegativePolarity       = FalseConditionWithNegativePolarity("false1-negative-polarity")
	unknown1WithNegativePolarity     = UnknownCondition("unknown1-negative-polarity", "reason unknown1-negative-polarity", "message unknown1-negative-polarity")
	trueInfo1WithNegativePolarity    = TrueConditionWithNegativePolarity("trueInfo1-negative-polarity", "reason trueInfo1-negative-polarity", clusterv1.ConditionSeverityInfo, "message trueInfo1-negative-polarity")
	trueWarning1WithNegativePolarity = TrueConditionWithNegativePolarity("trueWarning1-negative-polarity", "reason trueWarning1-negative-polarity", clusterv1.ConditionSeverityWarning, "message trueWarning1-negative-polarity")
	trueError1WithNegativePolarity   = TrueConditionWithNegativePolarity("trueError1-negative-polarity", "reason trueError1-negative-polarity", clusterv1.ConditionSeverityError, "message trueError1-negative-polarity")
)

func TestGetAndHas(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{}

	g.Expect(Has(cluster, "conditionBaz")).To(BeFalse())
	g.Expect(Get(cluster, "conditionBaz")).To(BeNil())

	cluster.SetConditions(conditionList(TrueCondition("conditionBaz")))

	g.Expect(Has(cluster, "conditionBaz")).To(BeTrue())
	g.Expect(Get(cluster, "conditionBaz")).To(HaveSameStateOf(TrueCondition("conditionBaz")))
}

func TestIsMethods(t *testing.T) {
	g := NewWithT(t)

	obj := getterWithConditions(nil1, true1, unknown1, falseInfo1, falseWarning1, falseError1, false1WithNegativePolarity, unknown1WithNegativePolarity, trueInfo1WithNegativePolarity, trueWarning1WithNegativePolarity, trueError1WithNegativePolarity)

	// test isTrue
	g.Expect(IsTrue(obj, "nil1")).To(BeFalse())
	g.Expect(IsTrue(obj, "true1")).To(BeTrue())
	g.Expect(IsTrue(obj, "falseInfo1")).To(BeFalse())
	g.Expect(IsTrue(obj, "unknown1")).To(BeFalse())
	g.Expect(IsTrue(obj, "false1-negative-polarity")).To(BeFalse())
	g.Expect(IsTrue(obj, "trueInfo1-negative-polarity")).To(BeTrue())
	g.Expect(IsTrue(obj, "unknown1-negative-polarity")).To(BeFalse())

	// test isFalse
	g.Expect(IsFalse(obj, "nil1")).To(BeFalse())
	g.Expect(IsFalse(obj, "true1")).To(BeFalse())
	g.Expect(IsFalse(obj, "falseInfo1")).To(BeTrue())
	g.Expect(IsFalse(obj, "unknown1")).To(BeFalse())
	g.Expect(IsFalse(obj, "false1-negative-polarity")).To(BeTrue())
	g.Expect(IsFalse(obj, "trueInfo1-negative-polarity")).To(BeFalse())
	g.Expect(IsFalse(obj, "unknown1-negative-polarity")).To(BeFalse())

	// test isUnknown
	g.Expect(IsUnknown(obj, "nil1")).To(BeTrue())
	g.Expect(IsUnknown(obj, "true1")).To(BeFalse())
	g.Expect(IsUnknown(obj, "falseInfo1")).To(BeFalse())
	g.Expect(IsUnknown(obj, "unknown1")).To(BeTrue())
	g.Expect(IsUnknown(obj, "false1-negative-polarity")).To(BeFalse())
	g.Expect(IsUnknown(obj, "trueInfo1-negative-polarity")).To(BeFalse())
	g.Expect(IsUnknown(obj, "unknown1-negative-polarity")).To(BeTrue())

	// test GetReason
	g.Expect(GetReason(obj, "nil1")).To(Equal(""))
	g.Expect(GetReason(obj, "falseInfo1")).To(Equal("reason falseInfo1"))
	g.Expect(GetReason(obj, "trueInfo1-negative-polarity")).To(Equal("reason trueInfo1-negative-polarity"))

	// test GetMessage
	g.Expect(GetMessage(obj, "nil1")).To(Equal(""))
	g.Expect(GetMessage(obj, "falseInfo1")).To(Equal("message falseInfo1"))
	g.Expect(GetMessage(obj, "trueInfo1-negative-polarity")).To(Equal("message trueInfo1-negative-polarity"))

	// test GetSeverity
	expectedSeverity := clusterv1.ConditionSeverityInfo
	g.Expect(GetSeverity(obj, "nil1")).To(BeNil())
	severity := GetSeverity(obj, "falseInfo1")
	g.Expect(severity).To(Equal(&expectedSeverity))
	severity = GetSeverity(obj, "trueInfo1-negative-polarity")
	g.Expect(severity).To(Equal(&expectedSeverity))

	// test GetLastTransitionTime
	g.Expect(GetLastTransitionTime(obj, "nil1")).To(BeNil())
	g.Expect(GetLastTransitionTime(obj, "falseInfo1")).ToNot(BeNil())
}

func TestMirror(t *testing.T) {
	foo := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityInfo, "message foo")
	ready := TrueCondition(clusterv1.ReadyCondition)
	readyBar := ready.DeepCopy()
	readyBar.Type = "bar"

	tests := []struct {
		name string
		from Getter
		t    clusterv1.ConditionType
		want *clusterv1.Condition
	}{
		{
			name: "Returns nil when the ready condition does not exists",
			from: getterWithConditions(foo),
			want: nil,
		},
		{
			name: "Returns ready condition from source",
			from: getterWithConditions(ready, foo),
			t:    "bar",
			want: readyBar,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := mirror(tt.from, tt.t)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).To(HaveSameStateOf(tt.want))
		})
	}
}

func TestSummary(t *testing.T) {
	foo := TrueCondition("foo")
	bar := FalseCondition("bar", "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	baz := FalseCondition("baz", "reason falseInfo2", clusterv1.ConditionSeverityInfo, "message falseInfo2")
	fooWithNegativePolarity := FalseConditionWithNegativePolarity("foo-negative-polarity")
	barWithNegativePolarity := TrueConditionWithNegativePolarity("bar-negative-polarity", "reason trueInfo1-negative-polarity", clusterv1.ConditionSeverityInfo, "message trueInfo1-negative-polarity")
	existingReady := FalseCondition(clusterv1.ReadyCondition, "reason falseError1", clusterv1.ConditionSeverityError, "message falseError1") // NB. existing ready has higher priority than other conditions

	tests := []struct {
		name    string
		from    Getter
		options []MergeOption
		want    *clusterv1.Condition
	}{
		{
			name: "Returns nil when there are no conditions to summarize",
			from: getterWithConditions(),
			want: nil,
		},
		{
			name: "Returns ready condition with the summary of existing conditions (with default options)",
			from: getterWithConditions(foo, bar),
			want: FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions with negative polarity (with default options)",
			from:    getterWithConditions(fooWithNegativePolarity, barWithNegativePolarity),
			options: []MergeOption{WithNegativePolarityConditions("foo-negative-polarity", "bar-negative-polarity")},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason trueInfo1-negative-polarity", clusterv1.ConditionSeverityInfo, "message trueInfo1-negative-polarity"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions with mixed polarity (with default options)",
			from:    getterWithConditions(foo, bar, fooWithNegativePolarity, barWithNegativePolarity),
			options: []MergeOption{WithNegativePolarityConditions("foo-negative-polarity", "bar-negative-polarity")},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1"), // bar take precedence on barWithNegativePolarity because it is listed first
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounter options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounter()},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounterIf(false)},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounterIf(true)},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithStepCounter(), WithStepCounterIfOnly("bar")},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "0 of 1 completed"),
		},
		{
			name:    "Returns ready condition with the summary of existing conditions (using WithStepCounterIf and WithStepCounterIfOnly options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithStepCounter(), WithStepCounterIfOnly("foo")},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions options)",
			from:    getterWithConditions(foo, bar),
			options: []MergeOption{WithConditions("foo")}, // bar should be ignored
			want:    TrueCondition(clusterv1.ReadyCondition),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions with negative polarity (using WithConditions options)",
			from:    getterWithConditions(fooWithNegativePolarity, barWithNegativePolarity),
			options: []MergeOption{WithConditions("foo-negative-polarity"), WithNegativePolarityConditions("foo-negative-polarity", "bar-negative-polarity")}, // bar-negative-polarity should be ignored because it is not listed in WithConditions
			want:    TrueCondition(clusterv1.ReadyCondition),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions with mixed polarity (using WithConditions options)",
			from:    getterWithConditions(foo, bar, fooWithNegativePolarity, barWithNegativePolarity),
			options: []MergeOption{WithConditions("foo", "foo-negative-polarity", "bar-negative-polarity"), WithNegativePolarityConditions("foo-negative-polarity", "bar-negative-polarity")},
			want:    FalseCondition(clusterv1.ReadyCondition, "reason trueInfo1-negative-polarity", clusterv1.ConditionSeverityInfo, "message trueInfo1-negative-polarity"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounter options)",
			from:    getterWithConditions(foo, bar, baz),
			options: []MergeOption{WithConditions("foo", "bar"), WithStepCounter()}, // baz should be ignored, total steps should be 2
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "1 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithConditions("bar", "baz"), WithStepCounter(), WithStepCounterIfOnly("bar")}, // there is only bar, the step counter should be set and counts only a subset of conditions
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "0 of 1 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options - with inconsistent order between the two)",
			from:    getterWithConditions(bar),
			options: []MergeOption{WithConditions("baz", "bar"), WithStepCounter(), WithStepCounterIfOnly("bar", "baz")}, // conditions in WithStepCounterIfOnly could be in different order than in WithConditions
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "0 of 2 completed"),
		},
		{
			name:    "Returns ready condition with the summary of selected conditions (using WithConditions and WithStepCounterIfOnly options)",
			from:    getterWithConditions(bar, baz),
			options: []MergeOption{WithConditions("bar", "baz"), WithStepCounter(), WithStepCounterIfOnly("bar")}, // there is also baz, so the step counter should not be set
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1"),
		},
		{
			name:    "Ready condition respects merge order",
			from:    getterWithConditions(bar, baz),
			options: []MergeOption{WithConditions("baz", "bar")}, // baz should take precedence on bar
			want:    FalseCondition(clusterv1.ReadyCondition, "reason falseInfo2", clusterv1.ConditionSeverityInfo, "message falseInfo2"),
		},
		{
			name: "Ignores existing Ready condition when computing the summary",
			from: getterWithConditions(existingReady, foo, bar),
			want: FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := summary(tt.from, tt.options...)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).To(HaveSameStateOf(tt.want))
		})
	}
}

func TestAggregate(t *testing.T) {
	ready1 := TrueCondition(clusterv1.ReadyCondition)
	ready2 := FalseCondition(clusterv1.ReadyCondition, "reason falseInfo1", clusterv1.ConditionSeverityInfo, "message falseInfo1")
	bar := FalseCondition("bar", "reason falseError1", clusterv1.ConditionSeverityError, "message falseError1") // NB. bar has higher priority than other conditions

	tests := []struct {
		name string
		from []Getter
		t    clusterv1.ConditionType
		want *clusterv1.Condition
	}{
		{
			name: "Returns nil when there are no conditions to aggregate",
			from: []Getter{},
			want: nil,
		},
		{
			name: "Returns foo condition with the aggregation of object's ready conditions",
			from: []Getter{
				getterWithConditions(ready1),
				getterWithConditions(ready1),
				getterWithConditions(ready2, bar),
				getterWithConditions(),
				getterWithConditions(bar),
			},
			t:    "foo",
			want: FalseCondition("foo", "reason falseInfo1", clusterv1.ConditionSeverityInfo, "2 of 5 completed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := aggregate(tt.from, tt.t)
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).To(HaveSameStateOf(tt.want))
		})
	}
}

func getterWithConditions(conditions ...*clusterv1.Condition) Getter {
	obj := &clusterv1.Cluster{}
	obj.SetConditions(conditionList(conditions...))
	return obj
}

func nilGetter() Getter {
	var obj *clusterv1.Cluster
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
