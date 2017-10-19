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

var _ cloud.Resource = &RouteTable{}

type RouteTable struct {
	Shared
	ClusterSubnet *cluster.Subnet
	ServerPool    *cluster.ServerPool
}

func (r *RouteTable) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("routetable.Actual")
	newResource := &RouteTable{
		Shared: Shared{
			Name: r.Name,
			Tags: make(map[string]string),
		},
	}

	if r.ClusterSubnet.Identifier != "" {
		input := &ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{
				{
					Name:   S("tag:kubicorn-route-table-subnet-pair"),
					Values: []*string{S(r.ClusterSubnet.Name)},
				},
			},
		}
		output, err := Sdk.Ec2.DescribeRouteTables(input)
		if err != nil {
			return nil, nil, err
		}
		llc := len(output.RouteTables)
		if llc > 0 {
			rt := output.RouteTables[0]
			for _, tag := range rt.Tags {
				key := *tag.Key
				val := *tag.Value
				newResource.Tags[key] = val
			}
			newResource.Name = r.ClusterSubnet.Name
			newResource.Identifier = r.ClusterSubnet.Name
		}
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *RouteTable) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("routetable.Expected")
	newResource := &RouteTable{
		Shared: Shared{
			Tags: map[string]string{
				"Name":                             r.Name,
				"KubernetesCluster":                immutable.Name,
				"kubicorn-route-table-subnet-pair": r.ClusterSubnet.Name,
			},
			Name:       r.ServerPool.Name,
			Identifier: r.ServerPool.Name,
		},
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *RouteTable) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("routetable.Apply")
	applyResource := expected.(*RouteTable)
	isEqual, err := compare.IsEqual(actual.(*RouteTable), expected.(*RouteTable))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}

	// --- Create Route Table
	rtInput := &ec2.CreateRouteTableInput{
		VpcId: &immutable.Network.Identifier,
	}
	rtOutput, err := Sdk.Ec2.CreateRouteTable(rtInput)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Created Route Table [%s]", *rtOutput.RouteTable.RouteTableId)

	//  --- Lookup Internet Gateway
	input := &ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   S("tag:kubicorn-internet-gateway-name"),
				Values: []*string{S(immutable.Name)},
			},
		},
	}
	output, err := Sdk.Ec2.DescribeInternetGateways(input)
	if err != nil {
		return nil, nil, err
	}
	lsn := len(output.InternetGateways)
	if lsn != 1 {
		return nil, nil, fmt.Errorf("Found [%d] Internet Gateways for ID [%s]", lsn, r.ServerPool.Identifier)
	}
	ig := output.InternetGateways[0]
	logger.Info("Mapping route table [%s] to internet gateway [%s]", *rtOutput.RouteTable.RouteTableId, *ig.InternetGatewayId)

	// --- Map Route Table to Internet Gateway
	riInput := &ec2.CreateRouteInput{
		DestinationCidrBlock: S("0.0.0.0/0"),
		GatewayId:            ig.InternetGatewayId,
		RouteTableId:         rtOutput.RouteTable.RouteTableId,
	}
	_, err = Sdk.Ec2.CreateRoute(riInput)
	if err != nil {
		return nil, nil, err
	}

	subnetID := ""
	for _, sp := range immutable.ServerPools {
		if sp.Name == r.Name {
			for _, sn := range sp.Subnets {
				if sn.Name == r.Name {
					subnetID = sn.Identifier
				}
			}
		}
	}
	if subnetID == "" {
		return nil, nil, fmt.Errorf("Unable to find subnet id")
	}

	// --- Associate Route table to this particular subnet
	asInput := &ec2.AssociateRouteTableInput{
		SubnetId:     &subnetID,
		RouteTableId: rtOutput.RouteTable.RouteTableId,
	}
	_, err = Sdk.Ec2.AssociateRouteTable(asInput)
	if err != nil {
		return nil, nil, err
	}

	expected.(*RouteTable).Identifier = *rtOutput.RouteTable.RouteTableId
	err = expected.(*RouteTable).tag(expected.(*RouteTable).Tags)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Associated route table [%s] to subnet [%s]", *rtOutput.RouteTable.RouteTableId, subnetID)
	newResource := &RouteTable{}
	newResource.Identifier = expected.(*RouteTable).Identifier
	newResource.Name = expected.(*RouteTable).Name

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *RouteTable) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("routetable.Delete")
	deleteResource := actual.(*RouteTable)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete routetable resource without ID [%s]", deleteResource.Name)
	}
	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{
			{
				Name:   S("tag:kubicorn-route-table-subnet-pair"),
				Values: []*string{S(r.ClusterSubnet.Name)},
			},
		},
	}
	output, err := Sdk.Ec2.DescribeRouteTables(input)
	if err != nil {
		return nil, nil, err
	}
	llc := len(output.RouteTables)
	if llc != 1 {
		return nil, nil, fmt.Errorf("Found [%d] Route Tables for VPC ID [%s]", llc, r.ClusterSubnet.Identifier)
	}
	rt := output.RouteTables[0]

	dainput := &ec2.DisassociateRouteTableInput{
		AssociationId: rt.Associations[0].RouteTableAssociationId,
	}
	_, err = Sdk.Ec2.DisassociateRouteTable(dainput)
	if err != nil {
		return nil, nil, err
	}

	dinput := &ec2.DeleteRouteTableInput{
		RouteTableId: rt.RouteTableId,
	}
	_, err = Sdk.Ec2.DeleteRouteTable(dinput)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted routetable [%s]", actual.(*RouteTable).Identifier)

	newResource := &RouteTable{}
	newResource.Name = actual.(*RouteTable).Name
	newResource.Tags = actual.(*RouteTable).Tags

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *RouteTable) tag(tags map[string]string) error {
	logger.Debug("routetable.Tag")
	tagInput := &ec2.CreateTagsInput{
		Resources: []*string{&r.Identifier},
	}
	for key, val := range tags {
		logger.Debug("Registering RouteTable tag [%s] %s", key, val)
		tagInput.Tags = append(tagInput.Tags, &ec2.Tag{
			Key:   S("%s", key),
			Value: S("%s", val),
		})
	}
	_, err := Sdk.Ec2.CreateTags(tagInput)
	if err != nil {
		return err
	}
	return nil
}

func (r *RouteTable) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("subnet.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	return newCluster
}
