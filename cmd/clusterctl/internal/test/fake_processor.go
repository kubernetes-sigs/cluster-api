/*
Copyright 2019 The Kubernetes Authors.

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

package test

import (
	"fmt"
)

type FakeProcessor struct {
	errGetVariables   error
	errGetVariableMap error
	errProcess        error
	artifactName      string
}

func NewFakeProcessor() *FakeProcessor {
	return &FakeProcessor{}
}

func (fp *FakeProcessor) WithTemplateName(n string) *FakeProcessor {
	fp.artifactName = n
	return fp
}

func (fp *FakeProcessor) WithGetVariablesErr(e error) *FakeProcessor {
	fp.errGetVariables = e
	return fp
}

func (fp *FakeProcessor) WithProcessErr(e error) *FakeProcessor {
	fp.errProcess = e
	return fp
}

func (fp *FakeProcessor) GetTemplateName(_, _ string) string {
	return fp.artifactName
}

func (fp *FakeProcessor) GetClusterClassTemplateName(_, name string) string {
	return fmt.Sprintf("clusterclass-%s.yaml", name)
}

func (fp *FakeProcessor) GetVariables(_ []byte) ([]string, error) {
	return nil, fp.errGetVariables
}

func (fp *FakeProcessor) GetVariableMap(_ []byte) (map[string]*string, error) {
	return nil, fp.errGetVariableMap
}

func (fp *FakeProcessor) Process(_ []byte, _ func(string) (string, error)) ([]byte, error) {
	return nil, fp.errProcess
}
