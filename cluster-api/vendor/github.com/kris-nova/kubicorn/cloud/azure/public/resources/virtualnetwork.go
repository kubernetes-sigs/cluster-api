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

package resources

import (
	"fmt"

	"github.com/kris-nova/kubicorn/cutil/retry"

	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
)

var _ cloud.Resource = &Vnet{}

type Vnet struct {
	Shared
	CIDR      string
	SubnetMap map[string]string
}

func (r *Vnet) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vnet.Actual")
	newResource := &Vnet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: immutable.Network.Identifier,
		},
	}
	if immutable.Network.Identifier != "" {
		vnet, err := Sdk.Vnet.Get(immutable.Name, immutable.Name, "")
		if err == nil {
			newResource.Identifier = *vnet.ID
			if len(*vnet.AddressSpace.AddressPrefixes) == 1 {
				spaces := *vnet.AddressSpace.AddressPrefixes
				newResource.CIDR = spaces[0]
			}
		}
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Vnet) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vnet.Expected")
	newResource := &Vnet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: immutable.Network.Identifier,
		},
		SubnetMap: make(map[string]string),
		CIDR:      immutable.Network.CIDR,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Vnet) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vnet.Apply")
	applyResource := expected.(*Vnet)
	isEqual, err := compare.IsEqual(actual.(*Vnet), expected.(*Vnet))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}

	parameters := network.VirtualNetwork{
		Location: &immutable.Location,
		VirtualNetworkPropertiesFormat: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{
				AddressPrefixes: &[]string{immutable.Network.CIDR},
			},
		},
	}

	// ------------------------ Subnets ------------------------
	var subnetsToAdd []network.Subnet
	for _, serverPool := range immutable.ServerPools {
		for _, subnet := range serverPool.Subnets {
			newParamsSubnet := network.Subnet{
				SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
					AddressPrefix: s(subnet.CIDR),
				},
				Name: s(serverPool.Name),
			}
			subnetsToAdd = append(subnetsToAdd, newParamsSubnet)
		}
	}
	parameters.Subnets = &subnetsToAdd

	_, errch := Sdk.Vnet.CreateOrUpdate(immutable.Name, immutable.Name, parameters, make(chan struct{}))
	err = <-errch
	if err != nil {
		return nil, nil, err
	}

	l := &lookUpVnetRetrier{
		name: immutable.Name,
	}
	retrier := retry.NewRetrier(10, 1, l)
	err = retrier.RunRetry()
	if err != nil {
		return nil, nil, err
	}
	parsedVnet := retrier.Retryable().(*lookUpVnetRetrier).vnet
	logger.Info("Created or found vnet [%s]", *parsedVnet.ID)

	newResource := &Vnet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: *parsedVnet.ID,
		},
		SubnetMap: make(map[string]string),
		CIDR:      immutable.Network.CIDR,
	}

	subnets := *parsedVnet.Subnets
	// Set subnet identifiers
	for _, subnet := range subnets {
		lookedUpName := *subnet.Name
		for _, serverPool := range immutable.ServerPools {
			if lookedUpName == serverPool.Name {
				newResource.SubnetMap[lookedUpName] = *subnet.ID
				logger.Info("Created subnet: %s", *subnet.ID)
			}
		}
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

type lookUpVnetRetrier struct {
	name string
	vnet network.VirtualNetwork
}

func (l *lookUpVnetRetrier) Try() error {
	vnet, err := Sdk.Vnet.Get(l.name, l.name, "")
	if err != nil {
		// fmt.Println(err)
		return err
	}
	if vnet.ID == nil || *vnet.ID == "" {
		// fmt.Println("empty id")
		return fmt.Errorf("Empty id")
	}
	l.vnet = vnet
	return nil
}

func (r *Vnet) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vnet.Delete")
	deleteResource := actual.(*Vnet)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete VPC resource without ID [%s]", deleteResource.Name)
	}

	respch, errch := Sdk.Vnet.Delete(immutable.Name, immutable.Name, make(chan struct{}))
	select {
	case <-respch:
		logger.Info("Deleted Vnet: %s", immutable.Network.Identifier)
	case err := <-errch:
		return nil, nil, err
	}
	logger.Info("Deleted Vnet [%s]", immutable.Network.Identifier)

	newResource := &Vnet{
		Shared: Shared{
			Name:       immutable.Network.Name,
			Tags:       r.Tags,
			Identifier: "",
		},
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Vnet) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("vnet.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	newCluster.Network.Identifier = newResource.(*Vnet).Identifier

	for _, serverPool := range newCluster.ServerPools {
		for _, subnet := range serverPool.Subnets {
			for subnetName, subnetId := range newResource.(*Vnet).SubnetMap {
				if subnetName == serverPool.Name {
					subnet.Identifier = subnetId
				}
			}
		}
	}

	return newCluster
}
