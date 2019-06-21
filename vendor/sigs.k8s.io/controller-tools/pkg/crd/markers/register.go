/*
Copyright 2019 The Kubernetes Authors.

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

package markers

import (
	"reflect"

	"sigs.k8s.io/controller-tools/pkg/markers"
)

// AllDefinitions contains all marker definitions for this package.
var AllDefinitions []*markers.Definition

// mustMakeAllWithPrefix converts each object into a marker definition using
// the object's type's with the prefix to form the marker name.
func mustMakeAllWithPrefix(prefix string, target markers.TargetType, objs ...interface{}) []*markers.Definition {
	defs := make([]*markers.Definition, len(objs))
	for i, obj := range objs {
		name := prefix + ":" + reflect.TypeOf(obj).Name()
		def, err := markers.MakeDefinition(name, target, obj)
		if err != nil {
			panic(err)
		}
		defs[i] = def
	}

	return defs
}

// Register registers all definitions for CRD generation to the given registry.
func Register(reg *markers.Registry) error {
	for _, def := range AllDefinitions {
		if err := reg.Register(def); err != nil {
			return err
		}
	}

	return nil
}
