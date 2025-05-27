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
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func (*KubeadmConfig) Hub()         {}
func (*KubeadmConfigTemplate) Hub() {}

func (*ClusterConfiguration) Hub() {}
func (*ClusterStatus) Hub()        {}
func (*InitConfiguration) Hub()    {}
func (*JoinConfiguration) Hub()    {}

// ConvertToArgs takes a argument map and converts it to a slice of arguments.
// The resulting argument slice is sorted alpha-numerically.
// NOTE: this is a util function intended only for usage in API conversions.
func ConvertToArgs(in map[string]string) []Arg {
	if in == nil {
		return nil
	}
	args := make([]Arg, 0, len(in))
	for k, v := range in {
		args = append(args, Arg{Name: k, Value: v})
	}
	sort.Slice(args, func(i, j int) bool {
		if args[i].Name == args[j].Name {
			return args[i].Value < args[j].Value
		}
		return args[i].Name < args[j].Name
	})
	return args
}

// ConvertFromArgs takes a slice of arguments and returns an argument map.
// Duplicate argument keys will be de-duped, where later keys will take precedence.
// NOTE: this is a util function intended only for usage in API conversions.
func ConvertFromArgs(in []Arg) map[string]string {
	if in == nil {
		return nil
	}
	args := make(map[string]string, len(in))
	for _, arg := range in {
		args[arg.Name] = arg.Value
	}
	return args
}

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
