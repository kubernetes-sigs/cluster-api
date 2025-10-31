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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
)

// Engine orchestrates the migration process by combining a parser and converter.
type Engine struct {
	parser    *YAMLParser
	converter *Converter

	errs     []error
	warnings []string
}

// MigrationOptions contains configuration for a migration run.
type MigrationOptions struct {
	Input     io.Reader
	Output    io.Writer
	Errors    io.Writer
	ToVersion string
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
		return nil, errors.New("parser and converter must be provided")
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
	return kerrors.NewAggregate(e.errs)
}

// Migrate handles multi-document YAML streams, error collection, and reporting.
func (e *Engine) Migrate(opts MigrationOptions) (*MigrationResult, error) {
	e.errs = make([]error, 0)
	e.warnings = make([]string, 0)

	// Parse input YAML stream
	documents, err := e.parser.ParseYAMLStream(opts.Input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse YAML stream")
	}

	result := &MigrationResult{
		TotalResources: len(documents),
		ConvertedCount: 0,
		SkippedCount:   0,
		ErrorCount:     0,
	}

	for i := range documents {
		doc := &documents[i]

		switch doc.Type {
		case ResourceTypeCoreV1Beta1:
			info := ResourceInfo{
				GroupVersionKind: doc.GVK,
				Index:            doc.Index,
			}

			conversionResult := e.converter.ConvertResource(info, doc.Object)

			if conversionResult.Error != nil {
				e.appendError(errors.Wrapf(conversionResult.Error, "failed to convert document at index %d (%s)", doc.Index, doc.GVK.String()))
				result.ErrorCount++
			} else if conversionResult.Converted {
				doc.Object = conversionResult.Object
				result.ConvertedCount++
			} else {
				result.SkippedCount++
				for _, warning := range conversionResult.Warnings {
					e.appendWarning(warning)
					if opts.Errors != nil {
						fmt.Fprintf(opts.Errors, "INFO: %s\n", warning)
					}
				}
			}

		case ResourceTypeOtherCAPI:
			// ResourceTypeOtherCAPI means cluster.x-k8s.io resources at versions other than v1beta1
			// (e.g., already at v1beta2) - pass through unchanged
			result.SkippedCount++
			if opts.Errors != nil {
				fmt.Fprintf(opts.Errors, "INFO: Resource %s is already at version %s, no conversion needed\n", doc.GVK.Kind, doc.GVK.Version)
			}

		case ResourceTypeNonCAPI:
			// Pass through non-CAPI resources unchanged
			result.SkippedCount++
			if opts.Errors != nil {
				fmt.Fprintf(opts.Errors, "INFO: Passing through non-CAPI resource: %s\n", doc.GVK.String())
			}

		case ResourceTypeUnsupported:
			// Pass through unsupported resources with warning
			result.SkippedCount++
			warning := fmt.Sprintf("Unable to parse document at index %d, passing through unchanged", doc.Index)
			e.appendWarning(warning)
			if opts.Errors != nil {
				fmt.Fprintf(opts.Errors, "WARNING: %s\n", warning)
			}
		}
	}

	if err := e.parser.SerializeYAMLStream(documents, opts.Output); err != nil {
		return nil, errors.Wrap(err, "failed to serialize output")
	}

	result.Warnings = e.warnings
	result.Errors = e.errs

	if len(e.errs) > 0 {
		return result, e.aggregateErrors()
	}

	return result, nil
}
