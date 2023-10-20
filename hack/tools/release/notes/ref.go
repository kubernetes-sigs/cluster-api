//go:build tools
// +build tools

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

package main

import (
	"strings"

	"github.com/pkg/errors"
)

// ref represents a git reference.
type ref struct {
	// reType is the ref type: tags for a tag, head for a branch, commit for a commit.
	reType string
	value  string
}

func (r ref) String() string {
	return r.reType + "/" + r.value
}

func parseRef(r string) ref {
	split := strings.SplitN(r, "/", 2)
	return ref{
		reType: split[0],
		value:  split[1],
	}
}

func validateRef(r string) error {
	split := strings.SplitN(r, "/", 2)
	if len(split) != 2 {
		return errors.Errorf("invalid ref %s: must follow [type]/[value]", r)
	}

	return nil
}
