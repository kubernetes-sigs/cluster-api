/*
Copyright 2020 The Kubernetes Authors.

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
package v1alpha3

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetReleaseSeriesForContract(t *testing.T) {
	rsSinglePerContract := []ReleaseSeries{
		{Major: 0, Minor: 4, Contract: "v1alpha4"},
		{Major: 0, Minor: 3, Contract: "v1alpha3"},
	}

	rsMultiplePerContract := []ReleaseSeries{
		{Major: 0, Minor: 4, Contract: "v1alpha4"},
		{Major: 0, Minor: 5, Contract: "v1alpha4"},
		{Major: 0, Minor: 3, Contract: "v1alpha3"},
	}

	tests := []struct {
		name                  string
		contract              string
		releaseSeries         []ReleaseSeries
		expectedReleaseSeries *ReleaseSeries
	}{
		{
			name:                  "Should get the release series with matching contract",
			contract:              "v1alpha4",
			releaseSeries:         rsSinglePerContract,
			expectedReleaseSeries: &rsMultiplePerContract[0],
		},
		{
			name:                  "Should get the newest release series with matching contract",
			contract:              "v1alpha4",
			releaseSeries:         rsMultiplePerContract,
			expectedReleaseSeries: &rsMultiplePerContract[1],
		},
		{
			name:                  "Should return nil if no release series with matching contract is found",
			contract:              "v1alpha5",
			releaseSeries:         rsMultiplePerContract,
			expectedReleaseSeries: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			m := &Metadata{ReleaseSeries: test.releaseSeries}
			g.Expect(m.GetReleaseSeriesForContract(test.contract)).To(Equal(test.expectedReleaseSeries))
		})
	}
}
