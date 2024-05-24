/*
Copyright 2021 The Kubernetes Authors.

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

package kubeadm

import (
	"fmt"
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
)

func TestGetDefaultRegistry(t *testing.T) {
	tests := []struct {
		version          string
		expectedRegistry string
	}{
		// < v1.22
		{version: "1.19.1", expectedRegistry: OldDefaultImageRepository},
		{version: "1.20.5", expectedRegistry: OldDefaultImageRepository},
		{version: "1.21.85", expectedRegistry: OldDefaultImageRepository},

		// v1.22
		{version: "1.22.0", expectedRegistry: OldDefaultImageRepository},
		{version: "1.22.16", expectedRegistry: OldDefaultImageRepository},
		{version: "1.22.17", expectedRegistry: DefaultImageRepository},
		{version: "1.22.99", expectedRegistry: DefaultImageRepository},

		// v1.23
		{version: "1.23.0", expectedRegistry: OldDefaultImageRepository},
		{version: "1.23.14", expectedRegistry: OldDefaultImageRepository},
		{version: "1.23.15", expectedRegistry: DefaultImageRepository},
		{version: "1.23.99", expectedRegistry: DefaultImageRepository},

		// v1.24
		{version: "1.24.0", expectedRegistry: OldDefaultImageRepository},
		{version: "1.24.8", expectedRegistry: OldDefaultImageRepository},
		{version: "1.24.9", expectedRegistry: DefaultImageRepository},
		{version: "1.24.99", expectedRegistry: DefaultImageRepository},

		// > v1.24
		{version: "1.25.0", expectedRegistry: DefaultImageRepository},
		{version: "1.26.1", expectedRegistry: DefaultImageRepository},
		{version: "1.27.2", expectedRegistry: DefaultImageRepository},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s => %s", tt.version, tt.expectedRegistry), func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(GetDefaultRegistry(semver.MustParse(tt.version))).To(Equal(tt.expectedRegistry))
		})
	}
}
