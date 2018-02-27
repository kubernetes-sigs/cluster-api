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

package machineset

import (
	"github.com/golang/glog"
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	listers "k8s.io/kube-deploy/ext-apiserver/pkg/client/listers_generated/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/pkg/controller/sharedinformers"
)

// +controller:group=cluster,version=v1alpha1,kind=MachineSet,resource=machinesets
type MachineSetControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about MachineSet
	lister listers.MachineSetLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *MachineSetControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing machinesets labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().MachineSets().Lister()
}

// Reconcile handles enqueued messages
func (c *MachineSetControllerImpl) Reconcile(u *v1alpha1.MachineSet) error {
	// TODO: Implement controller logic here
	glog.Infof("Running reconcile MachineSet for %s\n", u.Name)
	return nil
}

func (c *MachineSetControllerImpl) Get(namespace, name string) (*v1alpha1.MachineSet, error) {
	return c.lister.MachineSets(namespace).Get(name)
}
