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

package cmd

import (
	goerrors "errors"

	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type helpError struct {
	err error
}

func (e *helpError) Error() string {
	return e.err.Error()
}

func (e *helpError) Unwrap() error {
	return e.err
}

func wrapHelpError(err error) error {
	if err == nil {
		return nil
	}
	return &helpError{err: err}
}

func isHelpError(err error) bool {
	var target *helpError
	return goerrors.As(err, &target)
}

func helpOnErrorArgs(args cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, arg []string) error {
		return wrapHelpError(args(cmd, arg))
	}
}

func exactArgsWithMessage(expected int, message string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != expected {
			return wrapHelpError(pkgerrors.New(message))
		}
		return nil
	}
}
