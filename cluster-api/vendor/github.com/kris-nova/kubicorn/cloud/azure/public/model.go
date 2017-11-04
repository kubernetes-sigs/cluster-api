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

package public

import (
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cloud/azure/public/resources"
)

type Model struct {
	known           *cluster.Cluster
	cachedResources map[int]cloud.Resource
}

func NewAzurePublicModel(known *cluster.Cluster) cloud.Model {
	return &Model{
		known: known,
	}
}

func (m *Model) Resources() map[int]cloud.Resource {

	if len(m.cachedResources) > 0 {
		return m.cachedResources
	}

	known := m.known

	r := make(map[int]cloud.Resource)
	i := 0

	// ---- [Resource Group] ----
	r[i] = &resources.ResourceGroup{
		Shared: resources.Shared{
			Name: known.Name,
			Tags: make(map[string]string),
		},
	}
	i++

	// ---- [Vnet] ----
	r[i] = &resources.Vnet{
		Shared: resources.Shared{
			Name: known.Name,
			Tags: make(map[string]string),
		},
	}
	i++

	return r
}
