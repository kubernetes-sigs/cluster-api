/*
Copyright YEAR The Kubernetes Authors.

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

package festival

import (
	"log"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"github.com/kubernetes-incubator/apiserver-builder/example/pkg/apis/kingsport/v1"
	listers "github.com/kubernetes-incubator/apiserver-builder/example/pkg/client/listers_generated/kingsport/v1"
	"github.com/kubernetes-incubator/apiserver-builder/example/pkg/controller/sharedinformers"
)

// +controller:group=kingsport,version=v1,kind=Festival,resource=festivals
type FestivalControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about Festival
	lister listers.FestivalLister
}

// Init initializes the controller and is called by the generated code
func (c *FestivalControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	c.lister = arguments.GetSharedInformers().Factory.Kingsport().V1().Festivals().Lister()
}

// Reconcile handles enqueued messages
func (c *FestivalControllerImpl) Reconcile(u *v1.Festival) error {
	// Implement controller logic here
	log.Printf("Running reconcile Festival for %s\n", u.Name)
	return nil
}

func (c *FestivalControllerImpl) Get(namespace, name string) (*v1.Festival, error) {
	return c.lister.Get(name)
}
