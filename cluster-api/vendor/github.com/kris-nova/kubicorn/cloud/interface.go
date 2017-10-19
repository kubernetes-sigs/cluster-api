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

package cloud

import (
	"github.com/kris-nova/kubicorn/apis/cluster"
)

// Reconciler will create and destroy infrastructure based on an intended state. A Reconciler will
// also audit the expected and actual state.
type Reconciler interface {

	// Actual will audit a cloud and return the API representation of the current resources in the cloud.
	Actual(known *cluster.Cluster) (actual *cluster.Cluster, err error)

	// Expected will audit a state store and return the API representation of the intended resources in the cloud.
	Expected(known *cluster.Cluster) (expected *cluster.Cluster, err error)

	// Reconcile will take an actual and expected API representation and attempt to ensure the intended state.
	Reconcile(actual, expected *cluster.Cluster) (reconciled *cluster.Cluster, err error)

	// Destroy will take an actual API representation and destroy the resources in the cloud.
	Destroy() (destroyed *cluster.Cluster, err error)
}

// Model is what maps an API to a set of cloud Resources.
type Model interface {

	// Resources returns the mapped resources for the specific cloud implementation.
	Resources() map[int]Resource
}

// Resource represents a single cloud level resource that can be mutated. Resources are mapped via a model.
type Resource interface {

	// Actual will return the current existing resource in the cloud if it exists.
	Actual(known *cluster.Cluster) (actual *cluster.Cluster, resource Resource, err error)

	// Expected will return the anticipated cloud resource.
	Expected(known *cluster.Cluster) (expected *cluster.Cluster, resource Resource, err error)

	// Apply will create a cloud resource if needed.
	Apply(actual, expected Resource, expectedCluster *cluster.Cluster) (updatedCluster *cluster.Cluster, resource Resource, err error)

	// Delete will delete a cloud resource if needed.
	Delete(actual Resource, known *cluster.Cluster) (updatedCluster *cluster.Cluster, resource Resource, err error)
}
