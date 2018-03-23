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
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/test"

	"k8s.io/kube-deploy/cluster-api/pkg/apis"
	clusterv1 "k8s.io/kube-deploy/cluster-api/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/pkg/client/clientset_generated/clientset"
	"k8s.io/kube-deploy/cluster-api/pkg/controller/sharedinformers"
	"k8s.io/kube-deploy/cluster-api/pkg/openapi"
)

type fakeActuator struct{}

func (fakeActuator) Create(*clusterv1.Cluster, *clusterv1.Machine) error           { return nil }
func (fakeActuator) Delete(*clusterv1.Machine) error                               { return nil }
func (fakeActuator) Update(c *clusterv1.Cluster, machine *clusterv1.Machine) error { return nil }
func (fakeActuator) Exists(*clusterv1.Machine) (bool, error)                       { return false, nil }

func TestMachine(t *testing.T) {
	testenv := test.NewTestEnvironment()
	config := testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs := clientset.NewForConfigOrDie(config)

	shutdown := make(chan struct{})
	si := sharedinformers.NewSharedInformers(config, shutdown)
	controller := NewMachineController(config, si, fakeActuator{})
	controller.Run(shutdown)

	t.Run("machineControllerReconcile", func(t *testing.T) {
		machineControllerReconcile(t, cs, controller)
	})

	close(shutdown)
	testenv.Stop()
}
