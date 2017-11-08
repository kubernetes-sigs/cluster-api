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

package main

import (
	"context"
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
)

type controller struct {
	gce  *GCEClient
	kube *rest.RESTClient
}

func (c *controller) onAdd(obj interface{}) {
	machine := obj.(*clusterv1.Machine)
	fmt.Printf("object created: %s\n", machine.ObjectMeta.Name)

	if err := c.gce.CreateVM(machine); err != nil {
		fmt.Printf("error creating VM: %v\n", err)
	} else {
		fmt.Printf("created VM\n")
	}
}

func (c *controller) onUpdate(oldObj, newObj interface{}) {
	oldMachine := oldObj.(*clusterv1.Machine)
	newMachine := newObj.(*clusterv1.Machine)
	fmt.Printf("object updated: %s\n", oldMachine.ObjectMeta.Name)
	fmt.Printf("  old k8s version: %s, new: %s\n", oldMachine.Spec.Versions.Kubelet, newMachine.Spec.Versions.Kubelet)
}

func (c *controller) onDelete(obj interface{}) {
	machine := obj.(*clusterv1.Machine)
	fmt.Printf("object deleted: %s\n", machine.ObjectMeta.Name)

	if err := c.gce.DeleteVM(machine); err != nil {
		fmt.Printf("error deleting VM: %v\n", err)
	} else {
		fmt.Printf("deleted VM\n")
	}
}

func (c *controller) run(ctx context.Context) error {
	source := cache.NewListWatchFromClient(c.kube, "machines", apiv1.NamespaceAll, fields.Everything())

	_, controller := cache.NewInformer(
		source,
		&clusterv1.Machine{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		},
	)

	controller.Run(ctx.Done())
	// unreachable; run forever
	return nil
}
