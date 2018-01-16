
/*
Copyright 2018 The Kubernetes Authors.

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


package machine

import (
	"log"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/pkg/controller/sharedinformers"
	listers "k8s.io/kube-deploy/ext-apiserver/pkg/client/listers_generated/cluster/v1alpha1"
)

// +controller:group=cluster,version=v1alpha1,kind=Machine,resource=machines
type MachineControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about Machine
	lister listers.MachineLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing machines labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Machines().Lister()
}

// Reconcile handles enqueued messages
func (c *MachineControllerImpl) Reconcile(u *v1alpha1.Machine) error {
	// Implement controller logic here
	log.Printf("Running reconcile Machine for %s\n", u.Name)
	return nil
}

func (c *MachineControllerImpl) Get(namespace, name string) (*v1alpha1.Machine, error) {
	return c.lister.Machines(namespace).Get(name)
}
