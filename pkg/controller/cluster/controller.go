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
	"github.com/kubernetes-incubator/apiserver-builder/pkg/builders"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	listers "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/controller/sharedinformers"
	"sigs.k8s.io/cluster-api/pkg/util"
)

// +controller:group=cluster,version=v1alpha1,kind=Cluster,resource=clusters
type ClusterControllerImpl struct {
	builders.DefaultControllerFns

	actuator Actuator

	// clusterLister holds a lister that knows how to list Cluster from a cache
	clusterLister listers.ClusterLister

	kubernetesClientSet *kubernetes.Clientset
	clientSet           clientset.Interface
}

// Init initializes the controller and is called by the generated code
// Register watches for additional resource types here.
func (c *ClusterControllerImpl) Init(arguments sharedinformers.ControllerInitArguments, actuator Actuator) {
	cInformer := arguments.GetSharedInformers().Factory.Cluster().V1alpha1().Clusters()
	c.clusterLister = cInformer.Lister()
	c.actuator = actuator
	cs, err := clientset.NewForConfig(arguments.GetRestConfig())
	if err != nil {
		glog.Fatalf("error creating cluster client: %v", err)
	}
	c.clientSet = cs
	c.kubernetesClientSet = arguments.GetSharedInformers().KubernetesClientSet

	c.actuator = actuator
}

// Reconcile handles enqueued messages. The delete will be handled by finalizer.
func (c *ClusterControllerImpl) Reconcile(cluster *clusterv1.Cluster) error {
	// Deep-copy otherwise we are mutating our cache.
	clusterCopy := cluster.DeepCopy()
	name := clusterCopy.Name
	glog.Infof("Running reconcile Cluster for %s\n", name)

	if !clusterCopy.ObjectMeta.DeletionTimestamp.IsZero() {
		// no-op if finalizer has been removed.
		if !util.Contains(clusterCopy.ObjectMeta.Finalizers, clusterv1.ClusterFinalizer) {
			glog.Infof("reconciling cluster object %v causes a no-op as there is no finalizer.", name)
			return nil
		}

		glog.Infof("reconciling cluster object %v triggers delete.", name)
		if err := c.actuator.Delete(clusterCopy); err != nil {
			glog.Errorf("Error deleting cluster object %v; %v", name, err)
			return err
		}
		// Remove finalizer on successful deletion.
		glog.Infof("cluster object %v deletion successful, removing finalizer.", name)
		clusterCopy.ObjectMeta.Finalizers = util.Filter(clusterCopy.ObjectMeta.Finalizers, clusterv1.ClusterFinalizer)
		if _, err := c.clientSet.ClusterV1alpha1().Clusters(clusterCopy.Namespace).Update(clusterCopy); err != nil {
			glog.Errorf("Error removing finalizer from cluster object %v; %v", name, err)
			return err
		}
		return nil
	}

	glog.Infof("reconciling cluster object %v triggers idempotent reconcile.", name)
	err := c.actuator.Reconcile(clusterCopy)
	if err != nil {
		glog.Errorf("Error reconciling cluster object %v; %v", name, err)
		return err
	}
	return nil
}

func (c *ClusterControllerImpl) Get(namespace, name string) (*clusterv1.Cluster, error) {
	return c.clusterLister.Clusters(namespace).Get(name)
}
