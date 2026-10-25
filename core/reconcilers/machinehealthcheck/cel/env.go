/*
Copyright 2026 The Kubernetes Authors.

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

package cel

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	celgo "github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apiserver/pkg/cel/environment"
)

var (
	envOnce sync.Once
	env     *celgo.Env
	envErr  error
)

const (
	// nodeVariableName is the name of the CEL variable that the Node is bound to.
	nodeVariableName = "node"

	// machineVariableName is the name of the CEL variable that the Machine is bound to.
	machineVariableName = "machine"
)

type nodeForCEL struct {
	// Invisible to CEL, but accessible in go.
	conditions []corev1.NodeCondition
	now        time.Time
	nextCheck  *time.Duration
}

type machineForCEL struct {
	// Invisible to CEL, but accessible in go.
	conditions []metav1.Condition
	now        time.Time
	nextCheck  *time.Duration
}

// nodeForCELType and machineForCELType are the CEL object types that the native types
// extension (registered in getEnv below) derives for nodeForCEL and machineForCEL: the
// package alias (last segment of the package path) followed by the Go type name.
var (
	nodeForCELType    = celgo.ObjectType("cel.nodeForCEL")
	machineForCELType = celgo.ObjectType("cel.machineForCEL")
)

// getEnv returns the CEL environment used to compile and run expressions.
// The environment is based on the Kubernetes CEL base environment
// (the same function libraries used by e.g. CRD x-kubernetes-validations rules),
// extended with "node", "machine", etc.
func getEnv() (*celgo.Env, error) {
	envOnce.Do(func() {
		// CEL expression: machine.has_condition("Ready", "False")
		// CEL expression: machine.has_condition("Ready", "False", "Reason")
		machineHasConditionMethod := celgo.Function("has_condition",
			celgo.MemberOverload("machine_has_condition",
				[]*celgo.Type{
					machineForCELType, // Receiver: machine
					celgo.StringType,  // Arg 1: Type (string)
					celgo.StringType,  // Arg 2: Status ("False","True","Unknown")
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], nil, nil)
				}),
			),
			celgo.MemberOverload("machine_has_condition_with_reason",
				[]*celgo.Type{
					machineForCELType, // Receiver: machine
					celgo.StringType,  // Arg 1: Type (string)
					celgo.StringType,  // Arg 2: Status ("False","True","Unknown")
					celgo.StringType,  // Arg 3: Reason (string)
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], args[3], nil)
				}),
			),
		)
		// CEL expression: machine.has_condition_since("Ready", "False", "1h")
		// CEL expression: machine.has_condition_since("Ready", "False", "Reason", "1h")
		machineHasConditionSinceMethod := celgo.Function("has_condition_since",
			celgo.MemberOverload("machine_has_condition_since",
				[]*celgo.Type{
					machineForCELType, // Receiver: machine
					celgo.StringType,  // Arg 1: Type (string)
					celgo.StringType,  // Arg 2: Status ("False","True","Unknown")
					celgo.StringType,  // Arg 3: Duration ("1h")
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], nil, args[3])
				}),
			),
			celgo.MemberOverload("machine_has_condition_since_with_reason",
				[]*celgo.Type{
					machineForCELType, // Receiver: machine
					celgo.StringType,  // Arg 1: Type (string)
					celgo.StringType,  // Arg 2: Status ("False","True","Unknown")
					celgo.StringType,  // Arg 3: Reason (string)
					celgo.StringType,  // Arg 4: Duration ("1h")
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], args[3], args[4])
				}),
			),
		)
		// CEL expression: node.has_condition("Ready", "False")
		// CEL expression: node.has_condition("Ready", "False", "Reason")
		nodeHasConditionMethod := celgo.Function("has_condition",
			celgo.MemberOverload("node_has_condition",
				[]*celgo.Type{
					nodeForCELType,   // Receiver: node
					celgo.StringType, // Arg 1: Type (string)
					celgo.StringType, // Arg 2: Status ("False","True","Unknown")
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], nil, nil)
				}),
			),
			celgo.MemberOverload("node_has_condition_with_reason",
				[]*celgo.Type{
					nodeForCELType,   // Receiver: node
					celgo.StringType, // Arg 1: Type (string)
					celgo.StringType, // Arg 2: Status ("False","True","Unknown")
					celgo.StringType, // Arg 3: Reason (string)
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], args[3], nil)
				}),
			),
		)
		// CEL expression: node.has_condition_since("Ready", "False", "1h")
		// CEL expression: node.has_condition_since("Ready", "False", "Reason", "1h")
		nodeHasConditionSinceMethod := celgo.Function("has_condition_since",
			celgo.MemberOverload("node_has_condition_since",
				[]*celgo.Type{
					nodeForCELType,   // Receiver: node
					celgo.StringType, // Arg 1: Type (string)
					celgo.StringType, // Arg 2: Status ("False","True","Unknown")
					celgo.StringType, // Arg 3: Duration ("1h")
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], nil, args[3])
				}),
			),
			celgo.MemberOverload("node_has_condition_since_with_reason",
				[]*celgo.Type{
					nodeForCELType,   // Receiver: node
					celgo.StringType, // Arg 1: Type (string)
					celgo.StringType, // Arg 2: Status ("False","True","Unknown")
					celgo.StringType, // Arg 3: Reason (string)
					celgo.StringType, // Arg 4: Duration ("1h")
				},
				celgo.BoolType,
				celgo.FunctionBinding(func(args ...ref.Val) ref.Val {
					return evaluateCondition(args[0], args[1], args[2], args[3], args[4])
				}),
			),
		)

		envSet, err := environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()).Extend(
			environment.VersionedOptions{
				IntroducedVersion: version.MajorMinor(1, 0),
				EnvOptions: []celgo.EnvOption{
					ext.NativeTypes(
						ext.ParseStructTags(true),
						reflect.TypeFor[nodeForCEL](),
						reflect.TypeFor[machineForCEL](),
					),
					celgo.Variable(nodeVariableName, nodeForCELType),
					nodeHasConditionMethod,
					nodeHasConditionSinceMethod,
					celgo.Variable(machineVariableName, machineForCELType),
					machineHasConditionMethod,
					machineHasConditionSinceMethod,
					celgo.ASTValidators(conditionLiteralsValidator{}),
				},
			},
		)
		if err != nil {
			envErr = fmt.Errorf("failed to build CEL environment: %w", err)
			return
		}
		env, envErr = envSet.Env(environment.StoredExpressions)
	})
	return env, envErr
}

// conditionLiteralsValidator is a CEL AST validator that checks, at compile time, the string
// literal arguments of has_condition and has_condition_since: the status must be one of
// True, False, Unknown and the duration must be parseable by time.ParseDuration.
// Non-literal arguments can't be checked here and are validated when the expression is evaluated.
type conditionLiteralsValidator struct{}

func (conditionLiteralsValidator) Name() string {
	return "mhc.validator.condition_literals"
}

func (conditionLiteralsValidator) Validate(_ *celgo.Env, _ celgo.ValidatorConfig, a *celast.AST, iss *celgo.Issues) {
	celast.PreOrderVisit(a.Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		if e.Kind() != celast.CallKind {
			return
		}
		call := e.AsCall()
		if !call.IsMemberFunction() {
			return
		}
		args := call.Args()

		funcName := call.FunctionName()
		if funcName != "has_condition" && funcName != "has_condition_since" {
			return
		}

		if len(args) > 1 {
			if status, ok := stringLiteral(args[1]); ok && status != "True" && status != "False" && status != "Unknown" {
				iss.ReportErrorAtID(args[1].ID(), "invalid condition status %q, must be one of True, False, Unknown", status)
			}
		}
		if funcName == "has_condition_since" && (len(args) == 3 || len(args) == 4) {
			durationArg := args[len(args)-1]
			if duration, ok := stringLiteral(durationArg); ok {
				if _, err := time.ParseDuration(duration); err != nil {
					iss.ReportErrorAtID(durationArg.ID(), "invalid duration %q: %v", duration, err)
				}
			}
		}
	}))
}

// stringLiteral returns the value of e if it is a string literal.
func stringLiteral(e celast.Expr) (string, bool) {
	if e.Kind() != celast.LiteralKind {
		return "", false
	}
	s, ok := e.AsLiteral().Value().(string)
	return s, ok
}

// evaluateCondition implements the has_condition and has_condition_since methods. This code is executed when the
// CEL expressions are evaluated.
func evaluateCondition(receiverArg, expectedConditionTypeArg, expectedConditionStatusArg, expectedConditionReasonArg, expectedDurationArg ref.Val) ref.Val {
	// Extract arguments.
	expectedConditionType := expectedConditionTypeArg.Value().(string)
	expectedConditionStatus := expectedConditionStatusArg.Value().(string)
	if expectedConditionStatus != "True" && expectedConditionStatus != "False" && expectedConditionStatus != "Unknown" {
		return types.NewErr("invalid condition status: %s, must be one of True, False, Unknown", expectedConditionStatus)
	}
	var expectedConditionReason *string
	if expectedConditionReasonArg != nil {
		expectedConditionReason = new(expectedConditionReasonArg.Value().(string))
	}
	var expectedDuration *time.Duration
	if expectedDurationArg != nil {
		dur, err := time.ParseDuration(expectedDurationArg.Value().(string))
		if err != nil {
			return types.NewErr("invalid duration format: %v", err)
		}
		expectedDuration = &dur
	}

	var conditionStatus, conditionReason string
	var conditionLastTransitionTime metav1.Time
	var now time.Time
	var nextCheck *time.Duration
	var setNextCheck func(duration time.Duration)
	switch v := receiverArg.Value().(type) {
	case *machineForCEL:
		// Get condition.
		condition := meta.FindStatusCondition(v.conditions, expectedConditionType)
		if condition == nil {
			// If condition does not exist, return false.
			return types.Bool(false)
		}
		conditionStatus = string(condition.Status)
		conditionReason = condition.Reason
		conditionLastTransitionTime = condition.LastTransitionTime

		// Get other attributes.
		now = v.now
		nextCheck = v.nextCheck
		setNextCheck = func(nextCheck time.Duration) {
			v.nextCheck = &nextCheck
		}
	case *nodeForCEL:
		// Get condition.
		var condition *corev1.NodeCondition
		for _, c := range v.conditions {
			if string(c.Type) == expectedConditionType {
				condition = &c
				break
			}
		}
		if condition == nil {
			// If condition does not exist, return false.
			return types.Bool(false)
		}
		conditionStatus = string(condition.Status)
		conditionReason = condition.Reason
		conditionLastTransitionTime = condition.LastTransitionTime

		// Get other attributes.
		now = v.now
		nextCheck = v.nextCheck
		setNextCheck = func(nextCheck time.Duration) {
			v.nextCheck = &nextCheck
		}
	default:
		// Only *machineForCEL or *nodeForCEL works, if passed by value, we can't update nextCheck.
		return types.NewErr("expected *machineForCEL or *nodeForCEL")
	}

	// If status is different, return false.
	if conditionStatus != expectedConditionStatus {
		return types.Bool(false)
	}

	// If reason is expected and different, return false.
	if expectedConditionReason != nil && conditionReason != *expectedConditionReason {
		return types.Bool(false)
	}

	// If duration is not expected, return true.
	if expectedDuration == nil {
		return types.Bool(true)
	}

	matchTime := conditionLastTransitionTime.Add(*expectedDuration)
	timeUntilMatch := matchTime.Sub(now)

	// If condition is in the right state for expected duration, return true.
	if timeUntilMatch <= 0 {
		return types.Bool(true)
	}

	// If condition is not yet in the right state for the expected duration, return false
	// and set nextCheck to the time until we have a match.
	// Note: && and || might short circuit parts of an expression, but it's fine if we evaluate the expression again
	// after the part that is executed changes its result.
	if nextCheck == nil || timeUntilMatch < *nextCheck {
		setNextCheck(timeUntilMatch)
	}
	return types.Bool(false)
}
