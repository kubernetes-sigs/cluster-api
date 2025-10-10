/*
Copyright 2025 The Kubernetes Authors.

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

package migrate

import (
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// Engine coordinates YAML parsing, resource conversion, and output serialization.
type Engine struct {
	parser    *YAMLParser
	converter *Converter

	errs     []error
	warnings []string
}

// MigrationOptions contains input/output configuration for migration.
type MigrationOptions struct {
	Input  io.Reader
	Output io.Writer
	Errors io.Writer
}

// MigrationResult contains the outcome of a migration operation.
type MigrationResult struct {
	TotalResources int
	ConvertedCount int
	SkippedCount   int
	ErrorCount     int
	Warnings       []string
	Errors         []error
}

// ResourceInfo contains metadata about a resource being processed.
type ResourceInfo struct {
	GroupVersionKind schema.GroupVersionKind
	Name             string
	Namespace        string
	// Index in the YAML stream
	Index int
}

// NewEngine creates a new migration engine with parser and converter.
func NewEngine(parser *YAMLParser, converter *Converter) (*Engine, error) {
	if parser == nil || converter == nil {
		return nil, fmt.Errorf("parser and converter must be provided")
	}
	return &Engine{
		parser:    parser,
		converter: converter,
		errs:      make([]error, 0),
		warnings:  make([]string, 0),
	}, nil
}

// appendError records an error for later aggregation.
func (e *Engine) appendError(err error) {
	if err == nil {
		return
	}
	e.errs = append(e.errs, err)
}

// appendWarning records a warning message.
func (e *Engine) appendWarning(msg string) {
	if msg == "" {
		return
	}
	e.warnings = append(e.warnings, msg)
}

// aggregateErrors returns an aggregated error from collected errors.
func (e *Engine) aggregateErrors() error {
	return utilerrors.NewAggregate(e.errs)
}

// Migrate handles multi-document YAML streams, error collection, and reporting.
// TODO
func (e *Engine) Migrate(opts MigrationOptions) (*MigrationResult, error) {
	return nil, fmt.Errorf("migration engine not yet implemented")
}
