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

var _ cloud.Resource = &Vpc{}

type Vpc struct {
	Shared
	CIDR string
}

func (r *Vpc) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vpc.Actual")

	// New Resource
	newResource := &Vpc{
		Shared: Shared{
			Name: r.Name,
			Tags: make(map[string]string),
		},
	}

	// Query for resource if we have an Identifier
	if immutable.Network.Identifier != "" {
		input := &ec2.DescribeVpcsInput{
			VpcIds: []*string{&immutable.Network.Identifier},
		}
		output, err := Sdk.Ec2.DescribeVpcs(input)
		if err != nil {
			return nil, nil, err
		}
		lvpc := len(output.Vpcs)
		if lvpc != 1 {
			return nil, nil, fmt.Errorf("Found [%d] VPCs for ID [%s]", lvpc, immutable.Network.Identifier)
		}
		newResource.Identifier = *output.Vpcs[0].VpcId
		newResource.CIDR = *output.Vpcs[0].CidrBlock
		for _, tag := range output.Vpcs[0].Tags {
			key := *tag.Key
			val := *tag.Value
			newResource.Tags[key] = val
		}
	} else {
		newResource.CIDR = immutable.Network.CIDR
		newResource.Name = immutable.Network.Name
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *Vpc) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vpc.Expected")
	newResource := &Vpc{
		Shared: Shared{
			Tags: map[string]string{
				"Name":              r.Name,
				"KubernetesCluster": immutable.Name,
			},
			Identifier: immutable.Network.Identifier,
			Name:       r.Name,
		},
		CIDR: immutable.Network.CIDR,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Vpc) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vpc.Apply")
	applyResource := expected.(*Vpc)
	isEqual, err := compare.IsEqual(actual.(*Vpc), expected.(*Vpc))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}

	// Look up VPC
	input := &ec2.CreateVpcInput{
		CidrBlock: &applyResource.CIDR,
	}
	output, err := Sdk.Ec2.CreateVpc(input)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to create new VPC: %v", err)
	}

	// Modify VPC
	minput1 := &ec2.ModifyVpcAttributeInput{
		EnableDnsHostnames: &ec2.AttributeBooleanValue{
			Value: B(true),
		},
		VpcId: output.Vpc.VpcId,
	}
	_, err = Sdk.Ec2.ModifyVpcAttribute(minput1)
	if err != nil {
		return nil, nil, err
	}

	// Modify VPC
	minput2 := &ec2.ModifyVpcAttributeInput{
		EnableDnsSupport: &ec2.AttributeBooleanValue{
			Value: B(true),
		},
		VpcId: output.Vpc.VpcId,
	}
	_, err = Sdk.Ec2.ModifyVpcAttribute(minput2)
	if err != nil {
		return nil, nil, err
	}

	logger.Info("Created VPC [%s]", *output.Vpc.VpcId)

	newResource := &Vpc{
		Shared: Shared{
			Identifier: *output.Vpc.VpcId,
			Name:       applyResource.Name,
		},
		CIDR: *output.Vpc.CidrBlock,
	}

	// Tag newly created VPC
	err = newResource.tag(applyResource.Tags)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to tag new VPC: %v", err)
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *Vpc) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vpc.Delete")
	deleteResource := actual.(*Vpc)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete VPC resource without ID [%s]", deleteResource.Name)
	}
	input := &ec2.DeleteVpcInput{
		VpcId: &actual.(*Vpc).Identifier,
	}
	_, err := Sdk.Ec2.DeleteVpc(input)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted VPC [%s]", actual.(*Vpc).Identifier)

	newResource := &Vpc{
		Shared: Shared{
			Name: deleteResource.Name,
			Tags: deleteResource.Tags,
		},
		CIDR: deleteResource.CIDR,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Vpc) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("vpc.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	newCluster.Network.CIDR = newResource.(*Vpc).CIDR
	newCluster.Network.Identifier = newResource.(*Vpc).Identifier
	newCluster.Network.Name = newResource.(*Vpc).Name
	return newCluster
}

func (r *Vpc) tag(tags map[string]string) error {
	logger.Debug("vpc.Tag")
	tagInput := &ec2.CreateTagsInput{
		Resources: []*string{&r.Identifier},
	}
	for key, val := range tags {
		logger.Debug("Registering Vpc tag [%s] %s", key, val)
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
