// Copyright © 2017 The Kubicorn Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil"
	"github.com/kris-nova/kubicorn/cutil/initapi"
)

// Create will create a new test cluster
func Create(testCluster *cluster.Cluster) (*cluster.Cluster, error) {
	testCluster, err := initapi.InitCluster(testCluster)
	if err != nil {
		return nil, err
	}
	reconciler, err := cutil.GetReconciler(testCluster, nil)
	if err != nil {
		return nil, err
	}

	expected, err := reconciler.Expected(testCluster)
	if err != nil {
		return nil, err
	}
	actual, err := reconciler.Actual(testCluster)
	if err != nil {
		return nil, err
	}
	created, err := reconciler.Reconcile(actual, expected)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Read will read a test cluster
func Read(testCluster *cluster.Cluster) (*cluster.Cluster, error) {
	reconciler, err := cutil.GetReconciler(testCluster, nil)
	if err != nil {
		return nil, err
	}

	actual, err := reconciler.Actual(testCluster)
	if err != nil {
		return nil, err
	}
	return actual, nil
}

// Update will update a test cluster
func Update(testCluster *cluster.Cluster) (*cluster.Cluster, error) {
	testCluster, err := initapi.InitCluster(testCluster)
	if err != nil {
		return nil, err
	}
	reconciler, err := cutil.GetReconciler(testCluster, nil)
	if err != nil {
		return nil, err
	}

	expected, err := reconciler.Expected(testCluster)
	if err != nil {
		return nil, err
	}
	actual, err := reconciler.Actual(testCluster)
	if err != nil {
		return nil, err
	}
	updated, err := reconciler.Reconcile(actual, expected)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Delete will delete a test cluster
func Delete(testCluster *cluster.Cluster) (*cluster.Cluster, error) {
	reconciler, err := cutil.GetReconciler(testCluster, nil)
	if err != nil {
		return nil, err
	}

	destroyCluster, err := reconciler.Destroy()
	if err != nil {
		return nil, err
	}
	return destroyCluster, nil
}
