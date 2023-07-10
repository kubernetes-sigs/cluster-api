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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var expressionMatcher *ExpressionMatcher

// ExpressionMatcher holds initialized CEL program for evaluation of incoming objects on events
// and filtering which objects to reconcile.
type ExpressionMatcher struct {
	program cel.Program
}

// InitExpressionMatcher initializes expression which will apply on all processed objects in events in every controller.
func InitExpressionMatcher(log logr.Logger, expression string) error {
	if expression == "" {
		return nil
	}
	program, err := getProgram(log, expression)
	if err != nil {
		log.Error(err, fmt.Sprintf("Unable to compile CEL program for %s", expression))
		return err
	}
	log.Info("Initialized expression predicate for the given rule: ", expression)

	expressionMatcher = &ExpressionMatcher{program}
	return nil
}

// GetExpressionMatcher returns existing ExpressionMatcher.
func GetExpressionMatcher() *ExpressionMatcher {
	return expressionMatcher
}

func (m *ExpressionMatcher) matches(log logr.Logger, o client.Object) bool {
	matches, err := m.matchesExpression(log, o)
	if err != nil {
		log.V(6).Info(fmt.Sprintf("Rejecting object %+v due to errror: %s", o, err))
		return false
	}
	if !matches {
		log.V(6).Info(fmt.Sprintf("Rejecting object - does not match expression %+v", o))
	}

	return matches
}

func getProgram(log logr.Logger, expression string) (cel.Program, error) {
	declarations := cel.Declarations(
		decls.NewVar("self", decls.NewMapType(decls.String, decls.Dyn)),
	)

	env, err := cel.NewEnv(declarations)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to create CEL environment")
	}

	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		log.Error(issues.Err(), "Unable to compile provided expression %s", expression)
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, errors.New("Expression should evaluate to boolean value")
	}

	prg, err := env.Program(ast)
	if err != nil {
		log.Error(err, "Unable to create CEL instance")
		return nil, err
	}

	return prg, nil
}

func (m *ExpressionMatcher) matchesExpression(log logr.Logger, o client.Object) (bool, error) {
	if o == nil {
		return false, nil
	}

	unstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
	if err != nil {
		log.Error(err, "Unable to convert object to unstructured")
		return false, err
	}

	result, details, err := m.program.Eval(map[string]interface{}{
		"self": unstructured,
	})
	if err != nil {
		log.Error(err, fmt.Sprintf("Can't eval object with provided expression: %v", details))
		return false, err
	}

	return result == types.True, nil
}

// PassThrough returns a predicate accepting anything.
func PassThrough() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return true },
		UpdateFunc:  func(e event.UpdateEvent) bool { return true },
		DeleteFunc:  func(e event.DeleteEvent) bool { return true },
		GenericFunc: func(e event.GenericEvent) bool { return true },
	}
}

// Matches returns a predicate that returns true only if the provided
// CEL expression matches on filtered resource.
func (m *ExpressionMatcher) Matches(log logr.Logger) predicate.Funcs {
	if m == nil {
		log.Info("Skipping filter apply, no expression was set")
		return PassThrough()
	}

	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return m.matches(log.WithValues("predicate", "ResourceMatchesExpression", "eventType", "update"), e.ObjectNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return m.matches(log.WithValues("predicate", "ResourceMatchesExpression", "eventType", "create"), e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return m.matches(log.WithValues("predicate", "ResourceMatchesExpression", "eventType", "delete"), e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return m.matches(log.WithValues("predicate", "ResourceMatchesExpression", "eventType", "generic"), e.Object)
		},
	}
}
