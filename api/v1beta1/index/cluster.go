/*
Copyright 2021 The Kubernetes Authors.

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

package index

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// ClusterClassNameField is used by the Cluster controller to index Clusters by ClusterClass name.
	ClusterClassNameField = "spec.topology.class"
)

// ByClusterClassName adds the cluster class name  index to the
// managers cache.
func ByClusterClassName(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetCache().IndexField(ctx, &clusterv1.Cluster{},
		ClusterClassNameField,
		ClusterByClusterClassClassName,
	); err != nil {
		return errors.Wrap(err, "error setting index field")
	}
	return nil
}

// ClusterByClusterClassClassName contains the logic to index Clusters by ClusterClass name.
func ClusterByClusterClassClassName(o client.Object) []string {
	cluster, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected Cluster but got a %T", o))
	}
	if cluster.Spec.Topology != nil {
		return []string{cluster.GetClassKey().Name}
	}
	return nil
}
