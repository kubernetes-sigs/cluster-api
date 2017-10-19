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

var _ cloud.Resource = &InternetGateway{}

type InternetGateway struct {
	Shared
}

func (r *InternetGateway) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("internetgateway.Actual")
	newResource := &InternetGateway{
		Shared: Shared{
			Name: r.Name,
			Tags: make(map[string]string),
		},
	}
	if immutable.Network.InternetGW.Identifier != "" {
		input := &ec2.DescribeInternetGatewaysInput{
			Filters: []*ec2.Filter{
				{
					Name:   S("tag:kubicorn-internet-gateway-name"),
					Values: []*string{S(r.Name)},
				},
			},
		}
		output, err := Sdk.Ec2.DescribeInternetGateways(input)
		if err != nil {
			return nil, nil, err
		}
		lsn := len(output.InternetGateways)
		if lsn != 0 {
			ig := output.InternetGateways[0]
			for _, tag := range ig.Tags {
				key := *tag.Key
				val := *tag.Value
				newResource.Tags[key] = val
			}
			newResource.Identifier = *ig.InternetGatewayId
		}
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *InternetGateway) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("internetgateway.Expected %v", r.Identifier)
	newResource := &InternetGateway{
		Shared: Shared{
			Tags: map[string]string{
				"Name":                           r.Name,
				"KubernetesCluster":              immutable.Name,
				"kubicorn-internet-gateway-name": r.Name,
			},
			Identifier: immutable.Network.InternetGW.Identifier,
			Name:       r.Name,
		},
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *InternetGateway) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("internetgateway.Apply")
	applyResource := expected.(*InternetGateway)
	isEqual, err := compare.IsEqual(actual.(*InternetGateway), expected.(*InternetGateway))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}
	input := &ec2.CreateInternetGatewayInput{}
	output, err := Sdk.Ec2.CreateInternetGateway(input)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Created Internet Gateway [%s]", *output.InternetGateway.InternetGatewayId)
	ig := output.InternetGateway

	// --- Attach Internet Gateway to VPC
	atchinput := &ec2.AttachInternetGatewayInput{
		InternetGatewayId: ig.InternetGatewayId,
		VpcId:             &immutable.Network.Identifier,
	}
	_, err = Sdk.Ec2.AttachInternetGateway(atchinput)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Attaching Internet Gateway [%s] to VPC [%s]", *ig.InternetGatewayId, immutable.Network.Identifier)
	newResource := &InternetGateway{
		Shared: Shared{
			Tags: make(map[string]string),
		},
	}
	newResource.Identifier = *ig.InternetGatewayId
	newResource.Name = expected.(*InternetGateway).Name
	for key, value := range expected.(*InternetGateway).Tags {
		newResource.Tags[key] = value
	}
	expected.(*InternetGateway).Identifier = *output.InternetGateway.InternetGatewayId
	err = expected.(*InternetGateway).tag(expected.(*InternetGateway).Tags)
	if err != nil {
		return nil, nil, err
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *InternetGateway) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("internetgateway.Delete")
	deleteResource := actual.(*InternetGateway)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete internetgateway resource without ID [%s]", deleteResource.Name)
	}

	input := &ec2.DescribeInternetGatewaysInput{
		Filters: []*ec2.Filter{
			{
				Name:   S("tag:kubicorn-internet-gateway-name"),
				Values: []*string{S(r.Name)},
			},
		},
	}
	output, err := Sdk.Ec2.DescribeInternetGateways(input)
	if err != nil {
		return nil, nil, err
	}
	lsn := len(output.InternetGateways)
	if lsn == 0 {
		return nil, nil, nil
	}
	if lsn != 1 {
		return nil, nil, fmt.Errorf("Found [%d] Internet Gateways for ID [%s]", lsn, r.Name)
	}
	ig := output.InternetGateways[0]

	detinput := &ec2.DetachInternetGatewayInput{
		InternetGatewayId: ig.InternetGatewayId,
		VpcId:             &immutable.Network.Identifier,
	}
	_, err = Sdk.Ec2.DetachInternetGateway(detinput)
	if err != nil {
		return nil, nil, err
	}

	delinput := &ec2.DeleteInternetGatewayInput{
		InternetGatewayId: ig.InternetGatewayId,
	}
	_, err = Sdk.Ec2.DeleteInternetGateway(delinput)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted internetgateway [%s]", actual.(*InternetGateway).Identifier)
	newResource := &InternetGateway{}
	newResource.Name = actual.(*InternetGateway).Name
	newResource.Tags = actual.(*InternetGateway).Tags

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *InternetGateway) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("internetgateway.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	newCluster.Network.InternetGW.Identifier = newResource.(*InternetGateway).Identifier
	return newCluster
}

func (r *InternetGateway) tag(tags map[string]string) error {
	logger.Debug("internetgateway.Tag")
	tagInput := &ec2.CreateTagsInput{
		Resources: []*string{&r.Identifier},
	}
	for key, val := range tags {
		logger.Debug("Registering Internet Gateway tag [%s] %s", key, val)
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
