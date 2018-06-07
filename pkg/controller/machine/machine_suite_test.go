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
	"k8s.io/client-go/rest"

	"sigs.k8s.io/cluster-api/pkg/apis"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
	"sigs.k8s.io/cluster-api/pkg/openapi"
)

func TestMachine(t *testing.T) {
	testenv := test.NewTestEnvironment()
	config := testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs := clientset.NewForConfigOrDie(config)

	// TODO: When cluster-api support per namepsace object, we need to make each subtest to run
	// in different namespace. Everything lives inside default namespace so that the test needs to run
	// sequentially, which is the default behavior for "go test".
	t.Run("machineControllerReconcile", func(t *testing.T) {
		controller, shutdown := getController(config)
		defer close(shutdown)
		machineControllerReconcile(t, cs, controller, "default")
	})
	t.Run("machineControllerReconcileNonDefaultNameSpace", func(t *testing.T) {
		controller, shutdown := getController(config)
		defer close(shutdown)
		machineControllerReconcile(t, cs, controller, "nondefault")
	})
	t.Run("machineControllerConcurrentReconcile", func(t *testing.T) {
		controller, shutdown := getController(config)
		defer close(shutdown)
		machineControllerConcurrentReconcile(t, cs, controller)
	})
	testenv.Stop()
}

func getController(config *rest.Config) (*MachineController, chan struct{}) {
	shutdown := make(chan struct{})
	si := sharedinformers.NewSharedInformers(config, shutdown)
	actuator := NewTestActuator()
	controller := NewMachineController(config, si, actuator)
	controller.RunAsync(shutdown)

	return controller, shutdown
}
