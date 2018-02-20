
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


package cluster

import (
	"testing"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/test"

	"k8s.io/kube-deploy/ext-apiserver/pkg/apis"
	"k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset"
	"k8s.io/kube-deploy/ext-apiserver/pkg/openapi"
	"k8s.io/kube-deploy/ext-apiserver/pkg/controller/sharedinformers"
)

func TestCluster(t *testing.T) {
	testenv := test.NewTestEnvironment()
	config := testenv.Start(apis.GetAllApiBuilders(), openapi.GetOpenAPIDefinitions)
	cs := clientset.NewForConfigOrDie(config)

	shutdown := make(chan struct{})
	si := sharedinformers.NewSharedInformers(config, shutdown)
	controller := NewClusterController(config, si)
	controller.Run(shutdown)

	t.Run("clusterControllerReconcile", func(t *testing.T) {
		clusterControllerReconcile(t, cs, controller)
	})

	close(shutdown)
	testenv.Stop()
}
