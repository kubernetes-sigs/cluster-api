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

package names

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

func Test_templateGenerator_GenerateName(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     map[string]interface{}
		want     []types.GomegaMatcher
		wantErr  bool
	}{
		{
			name:     "simple template",
			template: "some-simple-{{ .test }}",
			data: map[string]interface{}{
				"test": "testdata",
			},
			want: []types.GomegaMatcher{
				Equal("some-simple-testdata"),
			},
		},
		{
			name:     "name which gets trimmed and added a random suffix with 5 characters",
			template: fmt.Sprintf("%064d", 0),
			want: []types.GomegaMatcher{
				HavePrefix(fmt.Sprintf("%058d", 0)),
				Not(HaveSuffix("00000")),
			},
		},
		{
			name:     "name which does not get trimmed",
			template: fmt.Sprintf("%063d", 0),
			want: []types.GomegaMatcher{
				Equal(fmt.Sprintf("%063d", 0)),
			},
		},
		{
			name:     "error on parsing template",
			template: "some-hardcoded-name-{{ .doesnotexistindata",
			wantErr:  true,
		},
		{
			name:     "error on due to missing key in data",
			template: "some-hardcoded-name-{{ .doesnotexistindata }}",
			data:     nil,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			generator := &templateGenerator{
				template: tt.template,
				data:     tt.data,
			}
			got, err := generator.GenerateName()
			if (err != nil) != tt.wantErr {
				t.Errorf("templateGenerator.GenerateName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) > maxNameLength {
				t.Errorf("generated name should never be longer than %d, got %d", maxNameLength, len(got))
			}
			for _, matcher := range tt.want {
				g.Expect(got).To(matcher)
			}
		})
	}
}
