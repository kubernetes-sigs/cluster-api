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
	"strings"
)

var _ cloud.Resource = &LoadBalancer{}

type LoadBalancer struct {
	Shared
	ServerPool      *cluster.ServerPool
	Subnet          *cluster.Subnet
	BackendPoolIDs  []string
	NatPoolIDs      []string
	InboundNatRules []*InboundNatRule
}

type InboundNatRule struct {
	ListenPort int
	TargetPort int
	Protocol   string
}

func (r *LoadBalancer) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("loadbalancer.Actual")
	newResource := &LoadBalancer{
		Shared: Shared{
			Tags:       r.Tags,
			Identifier: r.Subnet.Identifier,
		},
	}

	if r.Subnet.LoadBalancer.Identifier != "" {
		lb, err := Sdk.LoadBalancer.Get(immutable.Name, r.Subnet.Name, "")
		if err != nil {
			logger.Debug("Error looking up load balancer [%s]: %v", r.ServerPool.Name, err)
		} else if lb.Name != nil {
			newResource.Name = *lb.Name
			for _, b := range *lb.BackendAddressPools {
				newResource.BackendPoolIDs = append(newResource.BackendPoolIDs, *b.ID)
			}
			for _, b := range *lb.InboundNatPools {
				newResource.NatPoolIDs = append(newResource.NatPoolIDs, *b.ID)
			}
			for _, r := range *lb.InboundNatRules {
				newResource.InboundNatRules = append(newResource.InboundNatRules, &InboundNatRule{
					ListenPort: int(*r.FrontendPort),
					TargetPort: int(*r.BackendPort),
					Protocol:   string(r.Protocol),
				})
			}
		}
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *LoadBalancer) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("loadbalancer.Expected")
	newResource := &LoadBalancer{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: r.Subnet.Identifier,
		},
		NatPoolIDs:     r.Subnet.LoadBalancer.NATIDs,
		BackendPoolIDs: r.Subnet.LoadBalancer.BackendIDs,
	}
	for _, r := range r.Subnet.LoadBalancer.InboundRules {
		newResource.InboundNatRules = append(newResource.InboundNatRules, &InboundNatRule{
			ListenPort: r.ListenPort,
			TargetPort: r.TargetPort,
			Protocol:   r.Protocol,
		})
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *LoadBalancer) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("loadbalancer.Apply")
	applyResource := expected.(*LoadBalancer)
	isEqual, err := compare.IsEqual(actual.(*LoadBalancer), expected.(*LoadBalancer))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}
	pipid := ""
	for _, serverPools := range immutable.ServerPools {
		for _, subnet := range serverPools.Subnets {
			if subnet.Name == r.Subnet.Name {
				pipid = subnet.LoadBalancer.PublicIPIdentifier
			}
		}
	}
	if pipid == "" {
		return nil, nil, fmt.Errorf("Unable to look up IP ID for associated public IP")
	}
	fid := strings.Replace(pipid, fmt.Sprintf("publicIPAddresses/%s", r.Subnet.Name), fmt.Sprintf("loadBalancers/%s/frontendIPConfigurations/LoadBalancerFrontEnd", r.Subnet.Name), 1)
	var inboundPools []network.InboundNatPool
	i := 0
	for _, rule := range r.Subnet.LoadBalancer.InboundRules {

		logger.Debug("Frontend port: [%d]", rule.ListenPort)
		iPool := network.InboundNatPool{
			Name: s(fmt.Sprintf("InboundNatPool.%d", i)),
			InboundNatPoolPropertiesFormat: &network.InboundNatPoolPropertiesFormat{
				FrontendPortRangeStart: i32(int32(rule.ListenPort)),
				FrontendPortRangeEnd:   i32(int32(rule.ListenPort + 31)),
				BackendPort:            i32(int32(rule.TargetPort)),
				Protocol:               network.TransportProtocolTCP,
				FrontendIPConfiguration: &network.SubResource{
					ID: s(fid),
				},
			},
		}
		inboundPools = append(inboundPools, iPool)
		i++
	}

	parameters := network.LoadBalancer{
		Name:     s(r.Subnet.Name),
		Location: &immutable.Location,
		LoadBalancerPropertiesFormat: &network.LoadBalancerPropertiesFormat{
			InboundNatPools: &inboundPools,
			FrontendIPConfigurations: &[]network.FrontendIPConfiguration{
				{
					Name: s("LoadBalancerFrontEnd"),
					FrontendIPConfigurationPropertiesFormat: &network.FrontendIPConfigurationPropertiesFormat{
						PublicIPAddress: &network.PublicIPAddress{
							ID: &pipid,
						},
						PrivateIPAllocationMethod: network.Dynamic,
					},
				},
			},
			BackendAddressPools: &[]network.BackendAddressPool{
				{
					BackendAddressPoolPropertiesFormat: &network.BackendAddressPoolPropertiesFormat{},
					Name: s(fmt.Sprintf("backend-%s", r.Subnet.Name)),
				},
			},
		},
	}

	// ----- Debug request
	//byteslice, _ := json.Marshal(parameters)
	//var out bytes.Buffer
	//json.Indent(&out, byteslice, "", "  ")
	//fmt.Println(string(out.Bytes()))

	lbch, errch := Sdk.LoadBalancer.CreateOrUpdate(immutable.Name, r.Subnet.Name, parameters, make(chan struct{}))
	lb := <-lbch
	err = <-errch
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Created or updated load balancer [%s]", *lb.ID)

	var backEndPools []string
	for _, b := range *lb.BackendAddressPools {
		backEndPools = append(backEndPools, *b.ID)
	}
	var inboundNatPools []string
	for _, b := range *lb.InboundNatPools {
		logger.Debug(*b.ID)
		inboundNatPools = append(inboundNatPools, *b.ID)
	}

	newResource := &LoadBalancer{
		Shared: Shared{
			Name:       *lb.Name,
			Tags:       r.Tags,
			Identifier: *lb.ID,
		},
		NatPoolIDs:     inboundNatPools,
		BackendPoolIDs: backEndPools,
	}
	for _, r := range r.Subnet.LoadBalancer.InboundRules {
		newResource.InboundNatRules = append(newResource.InboundNatRules, &InboundNatRule{
			ListenPort: r.ListenPort,
			TargetPort: r.TargetPort,
			Protocol:   r.Protocol,
		})
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *LoadBalancer) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("loadbalancer.Delete")
	deleteResource := actual.(*LoadBalancer)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete VPC resource without ID [%s]", deleteResource.Name)
	}

	respch, errch := Sdk.LoadBalancer.Delete(immutable.Name, r.Subnet.Name, make(chan struct{}))
	<-respch
	err := <-errch
	if err != nil {
		return nil, nil, nil
	}
	logger.Info("Deleted load balancer [%s]", deleteResource.Identifier)
	newResource := &LoadBalancer{
		Shared: Shared{
			Name:       r.ServerPool.Name,
			Tags:       r.Tags,
			Identifier: immutable.Network.Identifier,
		},
	}
	for _, r := range r.Subnet.LoadBalancer.InboundRules {
		newResource.InboundNatRules = append(newResource.InboundNatRules, &InboundNatRule{
			ListenPort: r.ListenPort,
			TargetPort: r.TargetPort,
			Protocol:   r.Protocol,
		})
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *LoadBalancer) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("loadbalancer.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	for i := 0; i < len(newCluster.ServerPools); i++ {
		serverPool := newCluster.ServerPools[i]
		for j := 0; j < len(serverPool.Subnets); j++ {
			subnet := serverPool.Subnets[j]
			if subnet.Name == newResource.(*LoadBalancer).Name {
				subnet.LoadBalancer.BackendIDs = newResource.(*LoadBalancer).BackendPoolIDs
				subnet.LoadBalancer.NATIDs = newResource.(*LoadBalancer).NatPoolIDs
				subnet.LoadBalancer.Identifier = newResource.(*LoadBalancer).Identifier
				newCluster.ServerPools[i].Subnets[j] = subnet
			}
		}
	}
	return newCluster
}
