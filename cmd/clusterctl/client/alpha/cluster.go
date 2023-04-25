/*
Copyright 2023 The Kubernetes Authors.

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

package alpha

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// getCluster retrieves the Cluster object corresponding to the name and namespace specified.
func getCluster(proxy cluster.Proxy, name, namespace string) (*clusterv1.Cluster, error) {
	cluster := &clusterv1.Cluster{}
	c, err := proxy.NewClient()
	if err != nil {
		return nil, err
	}
	objKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	if err := c.Get(ctx, objKey, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}
