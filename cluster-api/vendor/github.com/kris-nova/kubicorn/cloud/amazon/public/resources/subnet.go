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

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
)

var _ cloud.Resource = &Subnet{}

type Subnet struct {
	Shared
	ClusterSubnet *cluster.Subnet
	ServerPool    *cluster.ServerPool
	CIDR          string
	VpcID         string
	Zone          string
}

func (r *Subnet) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("subnet.Actual")
	newResource := &Subnet{
		Shared: Shared{
			Name: r.Name,
			Tags: make(map[string]string),
		},
	}

	if r.ClusterSubnet.Identifier != "" {
		input := &ec2.DescribeSubnetsInput{
			SubnetIds: []*string{S(r.ClusterSubnet.Identifier)},
		}
		output, err := Sdk.Ec2.DescribeSubnets(input)
		if err != nil {
			return nil, nil, err
		}
		lsn := len(output.Subnets)
		if lsn != 1 {
			return nil, nil, fmt.Errorf("Found [%d] Subnets for ID [%s]", lsn, r.ClusterSubnet.Identifier)
		}
		subnet := output.Subnets[0]
		newResource.CIDR = *subnet.CidrBlock
		newResource.Identifier = *subnet.SubnetId
		newResource.VpcID = *subnet.VpcId
		newResource.Zone = *subnet.AvailabilityZone
		newResource.Tags = map[string]string{
			"Name":              r.Name,
			"KubernetesCluster": immutable.Name,
		}
		for _, tag := range subnet.Tags {
			key := *tag.Key
			val := *tag.Value
			newResource.Tags[key] = val
		}
	} else {
		newResource.CIDR = r.ClusterSubnet.CIDR
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Subnet) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("subnet.Expected")
	newResource := &Subnet{
		Shared: Shared{
			Tags: map[string]string{
				"Name":              r.Name,
				"KubernetesCluster": immutable.Name,
			},
			Identifier: r.ClusterSubnet.Identifier,
			Name:       r.Name,
		},
		CIDR:  r.ClusterSubnet.CIDR,
		VpcID: immutable.Network.Identifier,
		Zone:  r.ClusterSubnet.Zone,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *Subnet) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("subnet.Apply")
	applyResource := expected.(*Subnet)
	isEqual, err := compare.IsEqual(actual.(*Subnet), expected.(*Subnet))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}
	input := &ec2.CreateSubnetInput{
		CidrBlock:        &expected.(*Subnet).CIDR,
		VpcId:            &immutable.Network.Identifier,
		AvailabilityZone: &expected.(*Subnet).Zone,
	}
	output, err := Sdk.Ec2.CreateSubnet(input)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Created Subnet [%s]", *output.Subnet.SubnetId)
	newResource := &Subnet{}
	newResource.CIDR = *output.Subnet.CidrBlock
	newResource.VpcID = *output.Subnet.VpcId
	newResource.Zone = *output.Subnet.AvailabilityZone
	newResource.Name = applyResource.Name
	newResource.Identifier = *output.Subnet.SubnetId

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Subnet) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("subnet.Delete")
	deleteResource := actual.(*Subnet)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete subnet resource without ID [%s]", deleteResource.Name)
	}

	input := &ec2.DeleteSubnetInput{
		SubnetId: &actual.(*Subnet).Identifier,
	}
	_, err := Sdk.Ec2.DeleteSubnet(input)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted subnet [%s]", actual.(*Subnet).Identifier)

	newResource := &Subnet{}
	newResource.Name = actual.(*Subnet).Name
	newResource.Tags = actual.(*Subnet).Tags
	newResource.CIDR = actual.(*Subnet).CIDR
	newResource.Zone = actual.(*Subnet).Zone

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Subnet) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("subnet.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	subnet := &cluster.Subnet{}
	subnet.CIDR = newResource.(*Subnet).CIDR
	subnet.Zone = newResource.(*Subnet).Zone
	subnet.Name = newResource.(*Subnet).Name
	subnet.Identifier = newResource.(*Subnet).Identifier
	found := false

	for i := 0; i < len(newCluster.ServerPools); i++ {
		for j := 0; j < len(newCluster.ServerPools[i].Subnets); j++ {
			if newCluster.ServerPools[i].Subnets[j].Name == newResource.(*Subnet).Name {
				newCluster.ServerPools[i].Subnets[j].CIDR = newResource.(*Subnet).CIDR
				newCluster.ServerPools[i].Subnets[j].Zone = newResource.(*Subnet).Zone
				newCluster.ServerPools[i].Subnets[j].Identifier = newResource.(*Subnet).Identifier
				found = true
			}
		}
	}

	if !found {
		for i := 0; i < len(newCluster.ServerPools); i++ {
			if newCluster.ServerPools[i].Name == newResource.(*Subnet).Name {
				newCluster.ServerPools[i].Subnets = append(newCluster.ServerPools[i].Subnets, subnet)
				found = true
			}
		}
	}

	if !found {
		newCluster.ServerPools = append(newCluster.ServerPools, &cluster.ServerPool{
			Name: newResource.(*Subnet).Name,
			Subnets: []*cluster.Subnet{
				subnet,
			},
		})
	}
	return newCluster
}
