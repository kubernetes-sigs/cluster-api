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
	"time"

	celgo "github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/cache"
)

// ProgramEntry is a cache.Entry caching a compiled CEL program under its source CEL expression.
type ProgramEntry struct {
	// expression is the source CEL expression
	expression string

	// program is the compiled CEL program.
	program celgo.Program

	// usesNode is true if the expression references the "node" variable.
	usesNode bool

	// usesMachine is true if the expression references the "machine" variable.
	usesMachine bool
}

// Key returns the key of a ProgramEntry.
func (e ProgramEntry) Key() string {
	return e.expression
}

// Compile compiles expression, returning a runnable CEL program.
// Compiled programs are cached, so repeated calls with the same expression are cheap.
func Compile(programCache cache.Cache[ProgramEntry], expression string) (ProgramEntry, error) {
	if entry, ok := programCache.Has(expression); ok {
		// Refresh the cache entry so we cache forever if the expression is still used.
		programCache.Add(entry)
		return entry, nil
	}

	env, err := getEnv()
	if err != nil {
		return ProgramEntry{}, err
	}

	ast, iss := env.Compile(expression)
	if iss.Err() != nil {
		return ProgramEntry{}, iss.Err()
	}
	if ast.OutputType() != celgo.BoolType {
		return ProgramEntry{}, fmt.Errorf("expression must evaluate to a bool, got %s", ast.OutputType())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return ProgramEntry{}, err
	}

	entry := ProgramEntry{
		expression:  expression,
		program:     prg,
		usesNode:    referencesIdent(ast.NativeRep(), nodeVariableName),
		usesMachine: referencesIdent(ast.NativeRep(), machineVariableName),
	}
	programCache.Add(entry)
	return entry, nil
}

// referencesIdent returns true if the given AST references the given identifier.
func referencesIdent(a *celast.AST, identifier string) bool {
	found := false
	celast.PreOrderVisit(a.Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		if e.Kind() == celast.IdentKind && e.AsIdent() == identifier {
			found = true
		}
	}))
	return found
}

// ExpressionResultEntry represents the result of evaluating an MHC's UnhealthyExpressions for a specific Machine.
type ExpressionResultEntry struct {
	MachineHealthCheckKey        client.ObjectKey
	MachineHealthCheckGeneration int64

	MachineKey             client.ObjectKey
	MachineResourceVersion string

	NodeKey             client.ObjectKey
	NodeResourceVersion string

	UnhealthyConditionsMessages []string
	NextCheckTime               *time.Time
}

// Key returns the key of an ExpressionResultEntry.
func (e ExpressionResultEntry) Key() string {
	return e.MachineHealthCheckKey.String() + "," + e.MachineKey.String()
}

// EvaluateExpressions evaluates unhealthyExpressions from the passed mhc.
func EvaluateExpressions(
	programCache cache.Cache[ProgramEntry],
	expressionResultCache cache.Cache[ExpressionResultEntry],
	mhc *clusterv1.MachineHealthCheck,
	node *corev1.Node,
	machine *clusterv1.Machine,
	reconciliationTime time.Time,
) ([]string, time.Duration, error) {
	if len(mhc.Spec.Checks.UnhealthyExpressions) == 0 {
		return nil, 0, nil
	}

	if entry, nextCheck := tryToGetResultsFromCache(expressionResultCache, mhc, node, machine, reconciliationTime); entry != nil {
		// Refresh the cache entry so we can cache forever when NextCheckTime is not set.
		expressionResultCache.Add(*entry)
		return entry.UnhealthyConditionsMessages, nextCheck, nil
	}

	var nodeVal *nodeForCEL
	if node != nil {
		nodeVal = &nodeForCEL{conditions: node.Status.Conditions, now: reconciliationTime}
	}
	machineVal := &machineForCEL{conditions: machine.Status.Conditions, now: reconciliationTime}

	var unhealthyConditionsMessages []string
	var nextChecks []time.Duration
	for _, c := range mhc.Spec.Checks.UnhealthyExpressions {
		unhealthy, nextCheck, err := evaluateExpression(programCache, c.Expression, nodeVal, machineVal)
		if err != nil {
			return nil, 0, err
		}
		if unhealthy {
			unhealthyConditionsMessages = append(unhealthyConditionsMessages, c.Message)
		}
		nextChecks = append(nextChecks, nextCheck)
	}
	nextCheck := minDuration(nextChecks)

	entry := ExpressionResultEntry{
		MachineHealthCheckKey:        client.ObjectKeyFromObject(mhc),
		MachineHealthCheckGeneration: mhc.Generation,
		MachineKey:                   client.ObjectKeyFromObject(machine),
		MachineResourceVersion:       machine.ResourceVersion,
		UnhealthyConditionsMessages:  unhealthyConditionsMessages,
	}
	if nextCheck != 0 {
		entry.NextCheckTime = new(reconciliationTime.Add(nextCheck))
	}
	if node != nil {
		entry.NodeKey = client.ObjectKeyFromObject(node)
		entry.NodeResourceVersion = node.ResourceVersion
	}
	expressionResultCache.Add(entry)

	return unhealthyConditionsMessages, nextCheck, nil
}

func tryToGetResultsFromCache(expressionResultCache cache.Cache[ExpressionResultEntry], mhc *clusterv1.MachineHealthCheck, node *corev1.Node, machine *clusterv1.Machine, reconciliationTime time.Time) (*ExpressionResultEntry, time.Duration) {
	entry, ok := expressionResultCache.Has(ExpressionResultEntry{
		MachineHealthCheckKey: client.ObjectKeyFromObject(mhc),
		MachineKey:            client.ObjectKeyFromObject(machine),
	}.Key())
	// If cache entry does not exist, don't use the cache.
	if !ok {
		return nil, 0
	}

	// If MachineHealthCheck generation changed, don't use the cache.
	if mhc.Generation != entry.MachineHealthCheckGeneration {
		return nil, 0
	}

	// If Node just showed up or Node was deleted, don't use the cache.
	if (node == nil) != (entry.NodeKey == client.ObjectKey{}) {
		return nil, 0
	}

	// If we have a Node and the resourceVersion changed, don't use the cache.
	if node != nil && node.ResourceVersion != entry.NodeResourceVersion {
		return nil, 0
	}

	// If Machine resourceVersion changed, don't use the cache.
	if machine.ResourceVersion != entry.MachineResourceVersion {
		return nil, 0
	}

	// If NextCheckTime is unset, use the cache.
	if entry.NextCheckTime == nil {
		return &entry, 0
	}

	timeUntilNextCheck := entry.NextCheckTime.Sub(reconciliationTime)

	// If NextCheckTime is in the past, don't use the cache.
	if timeUntilNextCheck <= 0 {
		return nil, 0
	}

	// If NextCheckTime is in the future, use the cache.
	return &entry, timeUntilNextCheck
}

// evaluateExpression compiles (or reuses a cached compilation of) expression, evaluates it
// and returns whether the expression matched.
func evaluateExpression(programCache cache.Cache[ProgramEntry], expression string, node *nodeForCEL, machine *machineForCEL) (bool, time.Duration, error) {
	entry, err := Compile(programCache, expression)
	if err != nil {
		return false, 0, err
	}

	// If the expression references Node but we don't have a Node, consider the expression not matched.
	if entry.usesNode && node == nil {
		return false, 0, nil
	}

	// Only provide the variables that are used to reduce memory usage.
	vars := map[string]any{}
	if entry.usesNode {
		vars[nodeVariableName] = node
	}
	if entry.usesMachine {
		vars[machineVariableName] = machine
	}

	out, _, err := entry.program.Eval(vars)
	if err != nil {
		return false, 0, err
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, 0, fmt.Errorf("expression did not evaluate to a bool")
	}

	var nextChecks []time.Duration
	if m, ok := vars[machineVariableName]; ok {
		if m.(*machineForCEL).nextCheck != nil {
			nextChecks = append(nextChecks, *m.(*machineForCEL).nextCheck)
		}
	}
	if m, ok := vars[nodeVariableName]; ok {
		if m.(*nodeForCEL).nextCheck != nil {
			nextChecks = append(nextChecks, *m.(*nodeForCEL).nextCheck)
		}
	}

	return result, minDuration(nextChecks), nil
}

func minDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	minDuration := durations[0]
	// Ignore first element as that is already minDuration
	for _, duration := range durations[1:] {
		if duration < minDuration {
			minDuration = duration
		}
	}
	return minDuration
}
