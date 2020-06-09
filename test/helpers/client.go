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

package helpers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// NewFakeClientWithScheme creates a new fake client with the given scheme for testing.
// You can choose to initialize it with a slice of runtime.Object; all the objects with be given
// a fake ResourceVersion="1" so it will be possible to use optimistic lock.
func NewFakeClientWithScheme(clientScheme *runtime.Scheme, initObjs ...runtime.Object) client.Client {
	// NOTE: for consistency with the NewFakeClientWithScheme func in controller runtime, this func
	// should not have side effects on initObjs. So it creates a copy of each object and
	// set the resourceVersion on the copy only.
	initObjsWithResourceVersion := make([]runtime.Object, len(initObjs))
	for i := range initObjs {
		objsWithResourceVersion := initObjs[i].DeepCopyObject()
		accessor, err := meta.Accessor(objsWithResourceVersion)
		if err != nil {
			panic(fmt.Errorf("failed to get accessor for object: %v", err))
		}

		if accessor.GetResourceVersion() == "" {
			accessor.SetResourceVersion("1")
		}
		initObjsWithResourceVersion[i] = objsWithResourceVersion
	}
	return fake.NewFakeClientWithScheme(clientScheme, initObjsWithResourceVersion...)
}
