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

	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/cutil/retry"
)

var _ cloud.Resource = &PublicIP{}

type PublicIP struct {
	Shared
	ServerPool *cluster.ServerPool
	Subnet     *cluster.Subnet
	IpAddress  string
}

func (r *PublicIP) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("publicip.Actual")

	newResource := &PublicIP{
		Shared: Shared{
			Tags:       r.Tags,
			Identifier: r.Subnet.LoadBalancer.PublicIPIdentifier,
		},
	}
	if r.Subnet.LoadBalancer.Identifier != "" {
		ip, err := Sdk.PublicIP.Get(immutable.Name, r.Subnet.Name, "")
		if err != nil {
			logger.Debug("Error looking up public ip: %v", err)
		} else if ip.ID != nil {
			newResource.IpAddress = *ip.IPAddress
			newResource.Identifier = *ip.ID
			newResource.Name = *ip.Name
		}
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *PublicIP) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("publicip.Expected")
	newResource := &PublicIP{
		Shared: Shared{
			Name:       r.Subnet.Name,
			Tags:       r.Tags,
			Identifier: r.Subnet.LoadBalancer.PublicIPIdentifier,
		},
		IpAddress: r.Subnet.LoadBalancer.PublicIPAddress,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *PublicIP) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("publicip.Apply")
	applyResource := expected.(*PublicIP)
	isEqual, err := compare.IsEqual(actual.(*PublicIP), expected.(*PublicIP))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}

	parameters := network.PublicIPAddress{
		Name:     &applyResource.Name,
		Location: &immutable.Location,
		PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
			DNSSettings: &network.PublicIPAddressDNSSettings{
				DomainNameLabel: s(r.Subnet.Name),
			},
			PublicIPAddressVersion:   network.IPv4,
			IdleTimeoutInMinutes:     i32(4),
			PublicIPAllocationMethod: network.Static,
		},
	}
	ipch, errch := Sdk.PublicIP.CreateOrUpdate(immutable.Name, r.Subnet.Name, parameters, make(chan struct{}))

	select {
	case <-ipch:
		break
	case err = <-errch:
		if err != nil {
			return nil, nil, err
		}
	}

	l := &lookUpIpRetrier{
		clusterName:  immutable.Name,
		resourceName: r.Subnet.Name,
	}
	retrier := retry.NewRetrier(10, 1, l)
	err = retrier.RunRetry()
	if err != nil {
		return nil, nil, err
	}
	parsedIp := retrier.Retryable().(*lookUpIpRetrier).publicIP
	logger.Info("Created or found IP [%s]", *parsedIp.ID)
	newResource := &PublicIP{
		Shared: Shared{
			Name:       *parsedIp.Name,
			Tags:       r.Tags,
			Identifier: *parsedIp.ID,
		},
		//IpAddress: *parsedIp.IPAddress,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

type lookUpIpRetrier struct {
	clusterName  string
	resourceName string
	publicIP     network.PublicIPAddress
}

func (l *lookUpIpRetrier) Try() error {
	ip, err := Sdk.PublicIP.Get(l.clusterName, l.resourceName, "")
	if err != nil {
		// fmt.Println(err)
		return err
	}
	if ip.ID == nil || *ip.ID == "" {
		// fmt.Println("empty id")
		return fmt.Errorf("Empty id")
	}
	l.publicIP = ip
	return nil
}

func (r *PublicIP) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("publicip.Delete")
	deleteResource := actual.(*PublicIP)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete VPC resource without ID [%s]", deleteResource.Name)
	}

	respch, errch := Sdk.PublicIP.Delete(immutable.Name, r.Subnet.Name, make(chan struct{}))
	<-respch
	err := <-errch
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted public ip [%s]", deleteResource.Identifier)

	newResource := &PublicIP{
		Shared: Shared{},
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *PublicIP) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("publicip.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	for i := 0; i < len(newCluster.ServerPools); i++ {
		serverPool := newCluster.ServerPools[i]
		for j := 0; j < len(serverPool.Subnets); j++ {
			subnet := serverPool.Subnets[j]
			if subnet.Name == newResource.(*PublicIP).Name {
				subnet.LoadBalancer.PublicIPAddress = newResource.(*PublicIP).IpAddress
				subnet.LoadBalancer.PublicIPIdentifier = newResource.(*PublicIP).Identifier
			}
		}
	}
	return newCluster
}
