/*
Copyright 2017 The Kubernetes Authors.

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

package university

import (
	"fmt"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"github.com/kubernetes-incubator/apiserver-builder/example/pkg/apis/miskatonic/v1beta1"
	listers "github.com/kubernetes-incubator/apiserver-builder/example/pkg/client/listers_generated/miskatonic/v1beta1"
	"github.com/kubernetes-incubator/apiserver-builder/example/pkg/controller/sharedinformers"
)

// +controller:group=miskatonic,version=v1beta1,kind=University,resource=universities
type UniversityControllerImpl struct {
	builders.DefaultControllerFns

	// universitylister indexes properties about Universities
	universitylister listers.UniversityLister
}

// Init initializes the controller and is called by the generated code
func (c *UniversityControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	c.universitylister = arguments.GetSharedInformers().Factory.Miskatonic().V1beta1().Universities().Lister()
}

// Reconcile handles enqueued messages
func (c *UniversityControllerImpl) Reconcile(u *v1beta1.University) error {
	fmt.Printf("Running reconcile University for %s\n", u.Name)
	return nil
}

func (c *UniversityControllerImpl) Get(namespace, name string) (*v1beta1.University, error) {
	return c.universitylister.Universities(namespace).Get(name)
}
