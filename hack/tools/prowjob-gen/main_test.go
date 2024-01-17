/*
Copyright 2024 The Kubernetes Authors.

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

// main is the main package for prowjob-gen.
package main

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_Generator(t *testing.T) {
	g, err := newGenerator("test/test-configuration.yaml", "test", "test")
	if err != nil {
		t.Errorf("newGenerator() error = %v", err)
		return
	}

	if err := g.generate(); err != nil {
		t.Errorf("g.generate() error = %v", err)
		return
	}

	goldenData, err := os.ReadFile("test/test-main.yaml.golden")
	if err != nil {
		t.Errorf("reading golden file: %v", err)
		return
	}

	testData, err := os.ReadFile("test/test-main.yaml.tmp")
	if err != nil {
		t.Errorf("reading file generated from test: %v", err)
		return
	}

	if diff := cmp.Diff(string(goldenData), string(testData)); diff != "" {
		t.Errorf("generated and golden test file differ, diff:\n%s", diff)
	}
}
