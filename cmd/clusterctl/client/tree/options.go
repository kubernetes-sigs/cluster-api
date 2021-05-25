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

package tree

// AddObjectOption define an option for the ObjectTree Add operation.
type AddObjectOption interface {
	ApplyToAdd(*addObjectOptions)
}

type addObjectOptions struct {
	MetaName       string
	GroupingObject bool
	NoEcho         bool
}

func (o *addObjectOptions) ApplyOptions(opts []AddObjectOption) *addObjectOptions {
	for _, opt := range opts {
		opt.ApplyToAdd(o)
	}
	return o
}

// The ObjectMetaName option defines the meta name that should be used for the object in the presentation layer,
// e.g. control plane for KCP.
type ObjectMetaName string

// ApplyToAdd applies the given options.
func (n ObjectMetaName) ApplyToAdd(options *addObjectOptions) {
	options.MetaName = string(n)
}

// The GroupingObject option makes this node responsible of triggering the grouping action
// when adding the node's children.
type GroupingObject bool

// ApplyToAdd applies the given options.
func (n GroupingObject) ApplyToAdd(options *addObjectOptions) {
	options.GroupingObject = bool(n)
}

// The NoEcho options defines if the object should be hidden if the object's ready condition has the
// same Status, Severity and Reason of the parent's object ready condition (it is an echo).
type NoEcho bool

// ApplyToAdd applies the given options.
func (n NoEcho) ApplyToAdd(options *addObjectOptions) {
	options.NoEcho = bool(n)
}
