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

package catalog

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Hook defines the behaviour a hook should provide.
// TODO: verify if we can drop this, given that it used only to make func signature easier to ready (use Hook instead of interface{})
//   but at this stage we are not requiring any method to be implemented.
type Hook interface{}

// hookDescriptor hold all the information about a hook
type hookDescriptor struct {
	// TODO: add some metadata like description, deprecated (all we need for the OpenAPISpec)

	// request gvk for the Hook.
	request schema.GroupVersionKind

	// response gvk for the Hook.
	response schema.GroupVersionKind
}

// GroupVersionHook unambiguously identifies a Hook.
type GroupVersionHook struct {
	Group   string
	Version string
	Hook    string
}

var emptyGroupVersionService = GroupVersionHook{}

// Empty returns true if group, version, and hook are empty
func (gvs GroupVersionHook) Empty() bool {
	return len(gvs.Group) == 0 && len(gvs.Version) == 0 && len(gvs.Hook) == 0
}

func (gvs GroupVersionHook) GroupVersion() schema.GroupVersion {
	return schema.GroupVersion{Group: gvs.Group, Version: gvs.Version}
}

func (gvs GroupVersionHook) String() string {
	return strings.Join([]string{gvs.Group, "/", gvs.Version, ", Hook=", gvs.Hook}, "")
}
