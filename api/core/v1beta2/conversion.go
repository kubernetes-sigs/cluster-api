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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func (*Cluster) Hub()            {}
func (*ClusterClass) Hub()       {}
func (*Machine) Hub()            {}
func (*MachineSet) Hub()         {}
func (*MachineDeployment) Hub()  {}
func (*MachineHealthCheck) Hub() {}
func (*MachinePool) Hub()        {}
func (*MachineDrainRule) Hub()   {}

// ConvertToSeconds takes *metav1.Duration and returns a *int32.
// Durations longer than MaxInt32 are capped.
// NOTE: this is a util function intended only for usage in API conversions.
func ConvertToSeconds(in *metav1.Duration) *int32 {
	if in == nil {
		return nil
	}
	seconds := math.Trunc(in.Seconds())
	if seconds > math.MaxInt32 {
		return ptr.To[int32](math.MaxInt32)
	}
	return ptr.To(int32(seconds))
}

// ConvertFromSeconds takes *int32 and returns a *metav1.Duration.
// Durations longer than MaxInt32 are capped.
// NOTE: this is a util function intended only for usage in API conversions.
func ConvertFromSeconds(in *int32) *metav1.Duration {
	if in == nil {
		return nil
	}
	return ptr.To(metav1.Duration{Duration: time.Duration(*in) * time.Second})
}
