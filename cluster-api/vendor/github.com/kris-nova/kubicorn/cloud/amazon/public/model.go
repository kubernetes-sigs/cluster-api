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
	"github.com/kris-nova/kubicorn/cloud/amazon/public/resources"
)

type Model struct {
	known           *cluster.Cluster
	cachedResources map[int]cloud.Resource
}

func NewAmazonPublicModel(known *cluster.Cluster) cloud.Model {
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

	// ---- [Key Pair] ----
	r[i] = &resources.KeyPair{
		Shared: resources.Shared{
			Name: known.Name,
			Tags: make(map[string]string),
		},
	}
	i++

	// ---- [VPC] ----
	r[i] = &resources.Vpc{
		Shared: resources.Shared{
			Name: known.Name,
			Tags: make(map[string]string),
		},
	}
	//vpcIndex := i
	i++

	// ---- [Internet Gateway] ----
	r[i] = &resources.InternetGateway{
		Shared: resources.Shared{
			Name: known.Name,
			Tags: make(map[string]string),
		},
	}
	i++
	//
	for _, serverPool := range known.ServerPools {
		name := serverPool.Name

		// ---- [Security Groups] ----
		for _, firewall := range serverPool.Firewalls {
			r[i] = &resources.SecurityGroup{
				Shared: resources.Shared{
					Name: firewall.Name,
					Tags: make(map[string]string),
				},
				Firewall:   firewall,
				ServerPool: serverPool,
			}
			i++
		}

		// ---- [Subnets] ----
		for _, subnet := range serverPool.Subnets {
			r[i] = &resources.Subnet{
				Shared: resources.Shared{
					Name: subnet.Name,
					Tags: make(map[string]string),
				},
				ServerPool:    serverPool,
				ClusterSubnet: subnet,
			}
			i++

			// ---- [Route Table] ----
			r[i] = &resources.RouteTable{
				Shared: resources.Shared{
					Name: subnet.Name,
					Tags: make(map[string]string),
				},
				ClusterSubnet: subnet,
				ServerPool:    serverPool,
			}
			i++
		}

		// ---- [Launch Configuration] ----
		r[i] = &resources.Lc{
			Shared: resources.Shared{
				Name: name,
				Tags: make(map[string]string),
			},
			ServerPool: serverPool,
		}
		i++

		// ---- [Autoscale Group] ----
		r[i] = &resources.Asg{
			Shared: resources.Shared{
				Name: name,
				Tags: make(map[string]string),
			},
			ServerPool: serverPool,
		}
		i++
	}

	m.cachedResources = r
	return m.cachedResources
}
