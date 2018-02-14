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
package fake

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	cluster "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster"
)

// FakeClusters implements ClusterInterface
type FakeClusters struct {
	Fake *FakeCluster
	ns   string
}

var clustersResource = schema.GroupVersionResource{Group: "cluster", Version: "", Resource: "clusters"}

var clustersKind = schema.GroupVersionKind{Group: "cluster", Version: "", Kind: "Cluster"}

// Get takes name of the cluster, and returns the corresponding cluster object, and an error if there is any.
func (c *FakeClusters) Get(name string, options v1.GetOptions) (result *cluster.Cluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(clustersResource, c.ns, name), &cluster.Cluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*cluster.Cluster), err
}

// List takes label and field selectors, and returns the list of Clusters that match those selectors.
func (c *FakeClusters) List(opts v1.ListOptions) (result *cluster.ClusterList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(clustersResource, clustersKind, c.ns, opts), &cluster.ClusterList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &cluster.ClusterList{}
	for _, item := range obj.(*cluster.ClusterList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested clusters.
func (c *FakeClusters) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(clustersResource, c.ns, opts))

}

// Create takes the representation of a cluster and creates it.  Returns the server's representation of the cluster, and an error, if there is any.
func (c *FakeClusters) Create(cluster *cluster.Cluster) (result *cluster.Cluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(clustersResource, c.ns, cluster), &cluster.Cluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*cluster.Cluster), err
}

// Update takes the representation of a cluster and updates it. Returns the server's representation of the cluster, and an error, if there is any.
func (c *FakeClusters) Update(cluster *cluster.Cluster) (result *cluster.Cluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(clustersResource, c.ns, cluster), &cluster.Cluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*cluster.Cluster), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeClusters) UpdateStatus(cluster *cluster.Cluster) (*cluster.Cluster, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(clustersResource, "status", c.ns, cluster), &cluster.Cluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*cluster.Cluster), err
}

// Delete takes name of the cluster and deletes it. Returns an error if one occurs.
func (c *FakeClusters) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(clustersResource, c.ns, name), &cluster.Cluster{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeClusters) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(clustersResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &cluster.ClusterList{})
	return err
}

// Patch applies the patch and returns the patched cluster.
func (c *FakeClusters) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *cluster.Cluster, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(clustersResource, c.ns, name, data, subresources...), &cluster.Cluster{})

	if obj == nil {
		return nil, err
	}
	return obj.(*cluster.Cluster), err
}
