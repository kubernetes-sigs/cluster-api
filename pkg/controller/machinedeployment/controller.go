
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


package machinedeployment

import (
	"log"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
)

// +controller:group=cluster,version=v1alpha1,kind=MachineDeployment,resource=machinedeployments
type MachineDeploymentControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about MachineDeployment
	lister listers.MachineDeploymentLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineDeploymentControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing machinedeployments labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineDeployments().Lister()
}

// Reconcile handles enqueued messages
func (c *MachineDeploymentControllerImpl) Reconcile(u *v1alpha1.MachineDeployment) error {
	// Implement controller logic here
	log.Printf("Running reconcile MachineDeployment for %s\n", u.Name)
	return nil
}

func (c *MachineDeploymentControllerImpl) Get(namespace, name string) (*v1alpha1.MachineDeployment, error) {
	return c.lister.MachineDeployments(namespace).Get(name)
}
