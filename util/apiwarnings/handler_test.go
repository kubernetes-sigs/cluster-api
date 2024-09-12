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
	"regexp"
	"testing"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
)

func TestDiscardMatchingHandler(t *testing.T) {
	tests := []struct {
		name       string
		opts       DiscardMatchingHandlerOptions
		code       int
		message    string
		wantLogged bool
	}{
		{
			name:    "log, if warning does not match any expression",
			code:    299,
			message: "non-matching warning",
			opts: DiscardMatchingHandlerOptions{
				Expressions: []regexp.Regexp{},
			},
			wantLogged: true,
		},
		{
			name:    "do not log, if warning matches at least one expression",
			code:    299,
			message: "matching warning",
			opts: DiscardMatchingHandlerOptions{
				Expressions: []regexp.Regexp{
					*regexp.MustCompile("^matching.*"),
				},
			},
			wantLogged: false,
		},
		{
			name:    "do not log, if code is not 299",
			code:    0,
			message: "",
			opts: DiscardMatchingHandlerOptions{
				Expressions: []regexp.Regexp{},
			},
			wantLogged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			logged := false
			h := NewDiscardMatchingHandler(
				funcr.New(func(_, _ string) {
					logged = true
				},
					funcr.Options{},
				),
				tt.opts,
			)
			h.HandleWarningHeader(tt.code, "", tt.message)
			g.Expect(logged).To(Equal(tt.wantLogged))
		})
	}
}

func TestDiscardMatchingHandler_uninitialized(t *testing.T) {
	g := NewWithT(t)
	h := DiscardMatchingHandler{}
	g.Expect(func() {
		h.HandleWarningHeader(0, "", "")
	}).ToNot(Panic())
}
