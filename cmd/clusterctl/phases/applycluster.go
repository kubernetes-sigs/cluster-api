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

package phases

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
)

func ApplyCluster(client clusterclient.Client, cluster *clusterv1.Cluster, extra ...*unstructured.Unstructured) error {
	if cluster.Namespace == "" {
		cluster.Namespace = client.GetContextNamespace()
	}

	err := client.EnsureNamespace(cluster.Namespace)
	if err != nil {
		return errors.Wrapf(err, "unable to ensure namespace %q", cluster.Namespace)
	}

	for _, e := range extra {
		if e.GetNamespace() == "" {
			e.SetNamespace(client.GetContextNamespace())
		}

		klog.Infof("Creating Cluster referenced object %q with name %q in namespace %q", e.GroupVersionKind(), e.GetName(), e.GetNamespace())
		if err := client.CreateUnstructuredObject(e); err != nil {
			return err
		}
	}

	klog.Infof("Creating cluster object %v in namespace %q", cluster.Name, cluster.Namespace)
	if err := client.CreateClusterObject(cluster); err != nil {
		return err
	}

	return nil
}
