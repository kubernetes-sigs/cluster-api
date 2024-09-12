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

package apiwarnings

import (
	"testing"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
)

func TestDefaultHandler(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		message    string
		wantLogged bool
	}{
		{
			name:       "log, if warning does not match any expression",
			code:       299,
			message:    `metadata.finalizers: "foo.example.com": prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers`,
			wantLogged: true,
		},
		{
			name:       "do not log, if warning matches at least one expression",
			code:       299,
			message:    `metadata.finalizers: "dockermachine.infrastructure.cluster.x-k8s.io": prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers`,
			wantLogged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			logged := false
			h := DefaultHandler(
				funcr.New(func(_, _ string) {
					logged = true
				},
					funcr.Options{},
				),
			)
			h.HandleWarningHeader(tt.code, "", tt.message)
			g.Expect(logged).To(Equal(tt.wantLogged))
		})
	}
}
