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

package version

import (
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
)

func TestWorkerVersionSkewSupported(t *testing.T) {
	tests := []struct {
		name         string
		controlPlane string
		worker       string
		want         bool
	}{
		{name: "same version", controlPlane: "v1.32.0", worker: "v1.32.0", want: true},
		{name: "worker one minor older", controlPlane: "v1.32.0", worker: "v1.31.5", want: true},
		{name: "worker three minors older", controlPlane: "v1.32.0", worker: "v1.29.0", want: true},
		{name: "worker four minors older", controlPlane: "v1.32.0", worker: "v1.28.0", want: false},
		{name: "worker newer minor", controlPlane: "v1.32.0", worker: "v1.33.0", want: false},
		{name: "worker newer patch is allowed", controlPlane: "v1.32.0", worker: "v1.32.1", want: true},
		{name: "worker newer major", controlPlane: "v1.32.0", worker: "v2.0.0", want: false},
		{name: "worker older major", controlPlane: "v2.0.0", worker: "v1.32.0", want: false},
		// Control plane minor lower than the tolerated skew must not underflow.
		{name: "low control plane minor", controlPlane: "v1.2.0", worker: "v1.1.0", want: true},
		{name: "low control plane minor, worker at zero", controlPlane: "v1.2.0", worker: "v1.0.0", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(WorkerVersionSkewSupported(semver.MustParse(tt.controlPlane[1:]), semver.MustParse(tt.worker[1:]))).To(Equal(tt.want))
		})
	}
}

func TestKubeadmJoinVersionSkewSupported(t *testing.T) {
	tests := []struct {
		name         string
		controlPlane string
		worker       string
		want         bool
	}{
		{name: "same minor", controlPlane: "v1.32.0", worker: "v1.32.3", want: true},
		{name: "older minor", controlPlane: "v1.32.0", worker: "v1.31.0", want: false},
		{name: "newer minor", controlPlane: "v1.32.0", worker: "v1.33.0", want: false},
		{name: "different major", controlPlane: "v2.32.0", worker: "v1.32.0", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(KubeadmJoinVersionSkewSupported(semver.MustParse(tt.controlPlane[1:]), semver.MustParse(tt.worker[1:]))).To(Equal(tt.want))
		})
	}
}
