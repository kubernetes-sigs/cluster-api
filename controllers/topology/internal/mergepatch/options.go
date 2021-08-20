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

package mergepatch

// HelperOption is some configuration that modifies options for Helper.
type HelperOption interface {
	// ApplyToHelper applies this configuration to the given helper options.
	ApplyToHelper(*HelperOptions)
}

// HelperOptions contains options for Helper.
type HelperOptions struct {
	ignorePaths [][]string
}

// ApplyOptions applies the given patch options on these options,
// and then returns itself (for convenient chaining).
func (o *HelperOptions) ApplyOptions(opts []HelperOption) *HelperOptions {
	for _, opt := range opts {
		opt.ApplyToHelper(o)
	}
	return o
}

// IgnorePath instruct the Helper to ignore a given path when computing a patch.
type IgnorePath []string

// ApplyToHelper applies this configuration to the given helper options.
func (i IgnorePath) ApplyToHelper(opts *HelperOptions) {
	opts.ignorePaths = append(opts.ignorePaths, i)
}
