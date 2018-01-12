
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
	"log"

	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	"k8s.io/kube-deploy/ext-apiserver/pkg/controller/sharedinformers"
	listers "k8s.io/kube-deploy/ext-apiserver/pkg/client/listers_generated/cluster/v1alpha1"
)

// +controller:group=cluster,version=v1alpha1,kind=Cluster,resource=clusters
type ClusterControllerImpl struct {
	builders.DefaultControllerFns

	// lister indexes properties about Cluster
	lister listers.ClusterLister
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *ClusterControllerImpl) Init(arguments sharedinformers.ControllerInitArguments) {
	// Use the lister for indexing clusters labels
	c.lister = arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Clusters().Lister()
}

// Reconcile handles enqueued messages
func (c *ClusterControllerImpl) Reconcile(u *v1alpha1.Cluster) error {
	// Implement controller logic here
	log.Printf("Running reconcile Cluster for %s\n", u.Name)
	return nil
}

func (c *ClusterControllerImpl) Get(namespace, name string) (*v1alpha1.Cluster, error) {
	return c.lister.Clusters(namespace).Get(name)
}
