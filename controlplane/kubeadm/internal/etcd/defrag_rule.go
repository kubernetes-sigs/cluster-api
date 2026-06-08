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

package etcd

import (
	"github.com/google/cel-go/cel"
	"github.com/pkg/errors"
)

// defragRuleVars maps each CEL variable name to its type.
var defragRuleVars = map[string]*cel.Type{
	"dbSize":       cel.IntType,
	"dbSizeInUse":  cel.IntType,
	"dbSizeFree":   cel.IntType,
	"dbQuota":      cel.IntType,
	"dbQuotaUsage": cel.DoubleType,
}

// EvaluateDefragRule evaluates a CEL boolean expression against the provided etcd database metrics
// and returns true if defragmentation should be performed.
//
// The expression may reference the following variables:
//
//	dbSize       – total size of the etcd database file, in bytes (int)
//	dbSizeInUse  – total size in use in the etcd database, in bytes (int)
//	dbSizeFree   – total unused space (dbSize - dbSizeInUse), in bytes (int)
//	dbQuota      – etcd storage quota in bytes (int)
//	dbQuotaUsage – ratio of database size to quota (dbSize / dbQuota) (double)
func EvaluateDefragRule(rule string, dbSize, dbSizeInUse, dbQuota int64) (bool, error) {
	env, err := newDefragRuleEnv()
	if err != nil {
		return false, err
	}

	ast, issues := env.Compile(rule)
	if issues != nil && issues.Err() != nil {
		return false, errors.Wrapf(issues.Err(), "failed to compile defrag rule %q", rule)
	}

	prg, err := env.Program(ast)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create CEL program for defrag rule %q", rule)
	}

	out, _, err := prg.Eval(map[string]any{
		"dbSize":       dbSize,
		"dbSizeInUse":  dbSizeInUse,
		"dbSizeFree":   dbSize - dbSizeInUse,
		"dbQuota":      dbQuota,
		"dbQuotaUsage": float64(dbSize) / float64(dbQuota),
	})
	if err != nil {
		return false, errors.Wrapf(err, "failed to evaluate defrag rule %q", rule)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, errors.Errorf("defrag rule %q must evaluate to a boolean, got %T", rule, out.Value())
	}
	return result, nil
}

// ValidateDefragRule checks that rule is a syntactically valid CEL expression that references
// only the known defrag variables and evaluates to a boolean. It is intended for use in
// admission webhooks.
func ValidateDefragRule(rule string) error {
	if rule == "" {
		return nil
	}

	env, err := newDefragRuleEnv()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(rule)
	if issues != nil && issues.Err() != nil {
		return errors.Wrap(issues.Err(), "invalid defrag rule")
	}

	if !ast.OutputType().IsEquivalentType(cel.BoolType) {
		return errors.Errorf("defrag rule must evaluate to a boolean, got %s", ast.OutputType())
	}

	return nil
}

// newDefragRuleEnv creates a CEL environment with the five defragmentation variables.
// dbSize, dbSizeInUse, dbSizeFree, and dbQuota are int-typed (bytes); dbQuotaUsage is double-typed.
func newDefragRuleEnv() (*cel.Env, error) {
	opts := make([]cel.EnvOption, 0, len(defragRuleVars))
	for name, typ := range defragRuleVars {
		opts = append(opts, cel.Variable(name, typ))
	}
	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create CEL environment for defrag rule")
	}
	return env, nil
}
