/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"math"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestConvertSeconds(t *testing.T) {
	g := NewWithT(t)

	seconds := ptr.To[int32](100)
	duration := ConvertFromSeconds(seconds)
	g.Expect(ConvertToSeconds(duration)).To(Equal(seconds))

	seconds = nil
	duration = ConvertFromSeconds(seconds)
	g.Expect(ConvertToSeconds(duration)).To(Equal(seconds))

	// Durations longer than MaxInt32 are capped.
	duration = ptr.To(metav1.Duration{Duration: (math.MaxInt32 + 1) * time.Second})
	g.Expect(ConvertToSeconds(duration)).To(Equal(ptr.To[int32](math.MaxInt32)))
}

func TestConvert_bool_To_Pointer_bool(t *testing.T) {
	testCases := []struct {
		name        string
		in          bool
		hasRestored bool
		restored    *bool
		wantOut     *bool
	}{
		{
			name:    "when applying v1beta1, false should be converted to nil",
			in:      false,
			wantOut: nil,
		},
		{
			name:    "when applying v1beta1, true should be converted to *true",
			in:      true,
			wantOut: ptr.To(true),
		},
		{
			name:        "when doing round trip, false should be converted to nil if not previously explicitly set to false (previously set to nil)",
			in:          false,
			hasRestored: true,
			restored:    nil,
			wantOut:     nil,
		},
		{
			name:        "when doing round trip, false should be converted to nil if not previously explicitly set to false (previously set to true)",
			in:          false,
			hasRestored: true,
			restored:    ptr.To(true),
			wantOut:     nil,
		},
		{
			name:        "when doing round trip, false should be converted to false if previously explicitly set to false",
			in:          false,
			hasRestored: true,
			restored:    ptr.To(false),
			wantOut:     ptr.To(false),
		},
		{
			name:        "when doing round trip, true should be converted to *true (no matter of restored value is nil)",
			in:          true,
			hasRestored: true,
			restored:    nil,
			wantOut:     ptr.To(true),
		},
		{
			name:        "when doing round trip, true should be converted to *true (no matter of restored value is true)",
			in:          true,
			hasRestored: true,
			restored:    ptr.To(true),
			wantOut:     ptr.To(true),
		},
		{
			name:        "when doing round trip, true should be converted to *true (no matter of restored value is false)",
			in:          true,
			hasRestored: true,
			restored:    ptr.To(false),
			wantOut:     ptr.To(true),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var out *bool
			Convert_bool_To_Pointer_bool(tt.in, tt.hasRestored, tt.restored, &out)
			g.Expect(out).To(Equal(tt.wantOut))
		})
	}
}

func TestConvert_int32_To_Pointer_int32(t *testing.T) {
	testCases := []struct {
		name        string
		in          int32
		hasRestored bool
		restored    *int32
		wantOut     *int32
	}{
		{
			name:    "when applying v1beta1, 0 should be converted to nil",
			in:      0,
			wantOut: nil,
		},
		{
			name:    "when applying v1beta1, value!=0 should be converted to *value",
			in:      1,
			wantOut: ptr.To[int32](1),
		},
		{
			name:        "when doing round trip, 0 should be converted to nil if not previously explicitly set to 0 (previously set to nil)",
			in:          0,
			hasRestored: true,
			restored:    nil,
			wantOut:     nil,
		},
		{
			name:        "when doing round trip, 0 should be converted to nil if not previously explicitly set to 0 (previously set to another value)",
			in:          0,
			hasRestored: true,
			restored:    ptr.To[int32](1),
			wantOut:     nil,
		},
		{
			name:        "when doing round trip, 0 should be converted to 0 if previously explicitly set to 0",
			in:          0,
			hasRestored: true,
			restored:    ptr.To[int32](0),
			wantOut:     ptr.To[int32](0),
		},
		{
			name:        "when doing round trip, value should be converted to *value (no matter of restored value is nil)",
			in:          1,
			hasRestored: true,
			restored:    nil,
			wantOut:     ptr.To[int32](1),
		},
		{
			name:        "when doing round trip, value should be converted to *value (no matter of restored value is not 0)",
			in:          1,
			hasRestored: true,
			restored:    ptr.To[int32](2),
			wantOut:     ptr.To[int32](1),
		},
		{
			name:        "when doing round trip, value should be converted to *value (no matter of restored value is 0)",
			in:          1,
			hasRestored: true,
			restored:    ptr.To[int32](0),
			wantOut:     ptr.To[int32](1),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var out *int32
			Convert_int32_To_Pointer_int32(tt.in, tt.hasRestored, tt.restored, &out)
			g.Expect(out).To(Equal(tt.wantOut))
		})
	}
}

func TestConvert_Duration_To_Pointer_int32(t *testing.T) {
	testCases := []struct {
		name        string
		in          metav1.Duration
		hasRestored bool
		restored    *int32
		wantOut     *int32
	}{
		{
			name:    "when applying v1beta1, 0s should be converted to nil",
			in:      metav1.Duration{},
			wantOut: nil,
		},
		{
			name:    "when applying v1beta1, value!=0s should be converted to *value",
			in:      metav1.Duration{Duration: 1 * time.Second},
			wantOut: ptr.To[int32](1),
		},
		{
			name:        "when doing round trip, 0s should be converted to nil if not previously explicitly set to 0s (previously set to nil)",
			in:          metav1.Duration{},
			hasRestored: true,
			restored:    nil,
			wantOut:     nil,
		},
		{
			name:        "when doing round trip, 0s should be converted to nil if not previously explicitly set to 0s (previously set to another value)",
			in:          metav1.Duration{},
			hasRestored: true,
			restored:    ptr.To[int32](1),
			wantOut:     nil,
		},
		{
			name:        "when doing round trip, 0s should be converted to 0s if previously explicitly set to 0s",
			in:          metav1.Duration{},
			hasRestored: true,
			restored:    ptr.To[int32](0),
			wantOut:     ptr.To[int32](0),
		},
		{
			name:        "when doing round trip, value should be converted to *value (no matter of restored value is nil)",
			in:          metav1.Duration{Duration: 1 * time.Second},
			hasRestored: true,
			restored:    nil,
			wantOut:     ptr.To[int32](1),
		},
		{
			name:        "when doing round trip, value should be converted to *value (no matter of restored value is not 0s)",
			in:          metav1.Duration{Duration: 1 * time.Second},
			hasRestored: true,
			restored:    ptr.To[int32](2),
			wantOut:     ptr.To[int32](1),
		},
		{
			name:        "when doing round trip, value should be converted to *value (no matter of restored value is 0s)",
			in:          metav1.Duration{Duration: 1 * time.Second},
			hasRestored: true,
			restored:    ptr.To[int32](0),
			wantOut:     ptr.To[int32](1),
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var out *int32
			Convert_Duration_To_Pointer_int32(tt.in, tt.hasRestored, tt.restored, &out)
			g.Expect(out).To(Equal(tt.wantOut))
		})
	}
}
