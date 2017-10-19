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

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
)

var _ cloud.Resource = &Asg{}

type Asg struct {
	Shared
	MinCount     int
	MaxCount     int
	InstanceType string
	Image        string
	ServerPool   *cluster.ServerPool
}

func (r *Asg) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("asg.Actual")
	newResource := &Asg{
		Shared: Shared{
			Name: r.Name,
			Tags: make(map[string]string),
		},
		ServerPool: r.ServerPool,
	}
	if r.ServerPool.Identifier != "" {
		input := &autoscaling.DescribeAutoScalingGroupsInput{
			AutoScalingGroupNames: []*string{S(r.ServerPool.Identifier)},
		}
		output, err := Sdk.ASG.DescribeAutoScalingGroups(input)
		if err != nil {
			return nil, nil, err
		}
		lasg := len(output.AutoScalingGroups)
		if lasg != 1 {
			return nil, nil, fmt.Errorf("Found [%d] ASGs for ID [%s]", lasg, r.ServerPool.Identifier)
		}
		asg := output.AutoScalingGroups[0]
		for _, tag := range asg.Tags {
			key := *tag.Key
			val := *tag.Value
			newResource.Tags[key] = val
		}
		newResource.MaxCount = int(*asg.MaxSize)
		newResource.MinCount = int(*asg.MinSize)
		newResource.Identifier = *asg.AutoScalingGroupName
		newResource.Name = *asg.AutoScalingGroupName
	} else {
		newResource.MaxCount = r.ServerPool.MaxCount
		newResource.MinCount = r.ServerPool.MinCount
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *Asg) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("asg.Expected")
	newResource := &Asg{
		Shared: Shared{
			Tags: map[string]string{
				"Name":              r.Name,
				"KubernetesCluster": immutable.Name,
			},
			Identifier: r.ServerPool.Identifier,
			Name:       r.Name,
		},
		ServerPool: r.ServerPool,
		MaxCount:   r.ServerPool.MaxCount,
		MinCount:   r.ServerPool.MinCount,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}
func (r *Asg) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("asg.Apply")
	applyResource := expected.(*Asg)
	isEqual, err := compare.IsEqual(actual.(*Asg), expected.(*Asg))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
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

	newResource := &Asg{}
	input := &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName:    &r.Name,
		MinSize:                 I64(expected.(*Asg).MinCount),
		MaxSize:                 I64(expected.(*Asg).MaxCount),
		LaunchConfigurationName: &r.Name,
		VPCZoneIdentifier:       &subnetID,
	}
	_, err = Sdk.ASG.CreateAutoScalingGroup(input)
	if err != nil {
		if awserr, ok := err.(awserr.Error); ok {
			switch awserr.Code() {
			case autoscaling.ErrCodeAlreadyExistsFault:
				{
					input := &autoscaling.UpdateAutoScalingGroupInput{
						AutoScalingGroupName:    &r.Name,
						MinSize:                 I64(expected.(*Asg).MinCount),
						MaxSize:                 I64(expected.(*Asg).MaxCount),
						LaunchConfigurationName: &r.Name,
						VPCZoneIdentifier:       &subnetID,
					}
					resp, err := Sdk.ASG.UpdateAutoScalingGroup(input)
					if err != nil {
						logger.Debug("Error updating ASG: %v", err)
					}
					logger.Debug("ASG Update succeeded: %s", resp)
				}
			case autoscaling.ErrCodeResourceContentionFault:
				{
					logger.Debug("Pending ASG update - retry later")
				}
			default:
				logger.Debug("Unknown error during ASG update, v%", err)
			}
		}
	}

	logger.Info("Created Asg [%s]", r.Name)

	newResource.Name = r.Name
	newResource.Identifier = r.Name
	newResource.MaxCount = r.MaxCount
	newResource.MinCount = r.MinCount

	err = newResource.tag(applyResource.Tags)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to tag new VPC: %v", err)
	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Asg) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("asg.Delete")
	deleteResource := actual.(*Asg)
	if deleteResource.Identifier == "" {
		return nil, nil, fmt.Errorf("Unable to delete ASG resource without ID [%s]", deleteResource.Name)
	}

	input := &autoscaling.DeleteAutoScalingGroupInput{
		AutoScalingGroupName: &actual.(*Asg).Identifier,
		ForceDelete:          B(true),
	}
	_, err := Sdk.ASG.DeleteAutoScalingGroup(input)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted ASG [%s]", actual.(*Asg).Identifier)
	newResource := &Asg{}
	newResource.Name = actual.(*Asg).Name
	newResource.Tags = actual.(*Asg).Tags
	newResource.MaxCount = actual.(*Asg).MaxCount
	newResource.MinCount = actual.(*Asg).MinCount

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Asg) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("asg.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	serverPool := &cluster.ServerPool{}

	serverPool.MaxCount = newResource.(*Asg).MaxCount
	serverPool.MinCount = newResource.(*Asg).MinCount
	serverPool.Name = newResource.(*Asg).Name
	serverPool.Identifier = newResource.(*Asg).Identifier

	found := false

	for i := 0; i < len(newCluster.ServerPools); i++ {
		if newCluster.ServerPools[i].Name == newResource.(*Asg).Name {
			if newResource.(*Asg).ServerPool != nil {
				newCluster.ServerPools[i].MaxCount = newResource.(*Asg).ServerPool.MaxCount
				newCluster.ServerPools[i].MinCount = newResource.(*Asg).ServerPool.MinCount
			} else {
				newCluster.ServerPools[i].MaxCount = newResource.(*Asg).MaxCount
				newCluster.ServerPools[i].MinCount = newResource.(*Asg).MinCount
			}
			newCluster.ServerPools[i].Name = newResource.(*Asg).Name
			newCluster.ServerPools[i].Identifier = newResource.(*Asg).Identifier
			found = true
		}
	}
	if !found {
		newCluster.ServerPools = append(newCluster.ServerPools, serverPool)
	}

	return newCluster
}

func (r *Asg) tag(tags map[string]string) error {
	logger.Debug("asg.Tag")
	tagInput := &autoscaling.CreateOrUpdateTagsInput{}
	for key, val := range tags {
		logger.Debug("Registering Asg tag [%s] %s", key, val)
		tagInput.Tags = append(tagInput.Tags, &autoscaling.Tag{
			Key:               S("%s", key),
			Value:             S("%s", val),
			ResourceType:      S("auto-scaling-group"),
			ResourceId:        &r.Identifier,
			PropagateAtLaunch: B(true),
		})
	}
	_, err := Sdk.ASG.CreateOrUpdateTags(tagInput)
	if err != nil {
		return err
	}
	return nil
}
