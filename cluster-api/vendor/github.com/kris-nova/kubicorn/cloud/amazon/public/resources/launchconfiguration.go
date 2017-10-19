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
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/cutil/script"
)

var _ cloud.Resource = &Lc{}

type Lc struct {
	Shared
	InstanceType     string
	Image            string
	SpotPrice        string
	ServerPool       *cluster.ServerPool
	BootstrapScripts []string
}

const (
	MasterIPAttempts               = 40
	MasterIPSleepSecondsPerAttempt = 3
)

func (r *Lc) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("lc.Actual")
	newResource := &Lc{
		Shared: Shared{
			Name: r.Name,
			Tags: make(map[string]string),
		},
	}
	if r.ServerPool.Identifier != "" {
		lcInput := &autoscaling.DescribeLaunchConfigurationsInput{
			LaunchConfigurationNames: []*string{&r.ServerPool.Identifier},
		}
		lcOutput, err := Sdk.ASG.DescribeLaunchConfigurations(lcInput)
		if err != nil {
			return nil, nil, err
		}
		llc := len(lcOutput.LaunchConfigurations)
		if llc != 1 {
			return nil, nil, fmt.Errorf("Found [%d] Launch Configurations for ID [%s]", llc, r.ServerPool.Identifier)
		}
		lc := lcOutput.LaunchConfigurations[0]
		newResource.Image = *lc.ImageId
		if lc.SpotPrice != nil {
			newResource.SpotPrice = *lc.SpotPrice
		}
		newResource.Identifier = *lc.LaunchConfigurationName
		newResource.Tags = map[string]string{
			"Name":              r.Name,
			"KubernetesCluster": immutable.Name,
		}
	} else {
		newResource.Image = r.ServerPool.Image
		newResource.InstanceType = r.ServerPool.Size
		if r.ServerPool.Type == cluster.ServerPoolTypeNode && r.ServerPool.AwsConfiguration != nil {
			newResource.SpotPrice = r.ServerPool.AwsConfiguration.SpotPrice
		}
	}
	newResource.BootstrapScripts = r.ServerPool.BootstrapScripts

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Lc) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("asg.Expected")
	newResource := &Lc{
		Shared: Shared{
			Tags: map[string]string{
				"Name":              r.Name,
				"KubernetesCluster": immutable.Name,
			},
			Identifier: r.ServerPool.Identifier,
			Name:       r.Name,
		},
		InstanceType:     r.ServerPool.Size,
		Image:            r.ServerPool.Image,
		BootstrapScripts: r.ServerPool.BootstrapScripts,
	}
	if r.ServerPool.Type == cluster.ServerPoolTypeNode && r.ServerPool.AwsConfiguration != nil {
		newResource.SpotPrice = r.ServerPool.AwsConfiguration.SpotPrice
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Lc) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("lc.Apply")
	applyResource := expected.(*Lc)
	isEqual, err := compare.IsEqual(actual.(*Lc), expected.(*Lc))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}
	var sgs []*string
	found := false
	for _, serverPool := range immutable.ServerPools {
		if serverPool.Name == expected.(*Lc).Name {
			for _, firewall := range serverPool.Firewalls {
				sgs = append(sgs, &firewall.Identifier)
			}
			found = true
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("Unable to lookup serverpool for Launch Configuration %s", r.Name)
	}

	// --- Hack in here for master IP
	privip := ""
	pubip := ""
	if strings.Contains(r.ServerPool.Name, "node") {
		found := false
		logger.Debug("Tag query: [%s] %s", "Name", fmt.Sprintf("%s.master", immutable.Name))
		logger.Debug("Tag query: [%s] %s", "KubernetesCluster", immutable.Name)
		for i := 0; i < MasterIPAttempts; i++ {
			logger.Debug("Attempting to lookup master IP for node registration..")
			input := &ec2.DescribeInstancesInput{
				Filters: []*ec2.Filter{
					{
						Name:   S("tag:Name"),
						Values: []*string{S(fmt.Sprintf("%s.master", immutable.Name))},
					},
					{
						Name:   S("tag:KubernetesCluster"),
						Values: []*string{S(immutable.Name)},
					},
				},
			}
			output, err := Sdk.Ec2.DescribeInstances(input)
			if err != nil {
				return nil, nil, err
			}
			lr := len(output.Reservations)
			if lr == 0 {
				logger.Debug("Found %d Reservations, hanging ", lr)
				time.Sleep(time.Duration(MasterIPSleepSecondsPerAttempt) * time.Second)
				continue
			}
			for _, reservation := range output.Reservations {
				for _, instance := range reservation.Instances {
					if instance.PublicIpAddress != nil {
						privip = *instance.PrivateIpAddress
						pubip = *instance.PublicIpAddress
						immutable.Values.ItemMap["INJECTEDMASTER"] = fmt.Sprintf("%s:%s", privip, immutable.KubernetesAPI.Port)
						immutable.KubernetesAPI.Endpoint = pubip
						logger.Info("Found public IP for master: [%s]", pubip)
						found = true
					}
				}
			}
			if found == true {
				break
			}
			time.Sleep(time.Duration(MasterIPSleepSecondsPerAttempt) * time.Second)
		}
		if !found {
			return nil, nil, fmt.Errorf("Unable to find Master IP")
		}
	}

	immutable.Values.ItemMap["INJECTEDPORT"] = immutable.KubernetesAPI.Port

	newResource := &Lc{}
	userData, err := script.BuildBootstrapScript(r.ServerPool.BootstrapScripts, immutable)
	if err != nil {
		return nil, nil, err
	}

	b64data := base64.StdEncoding.EncodeToString(userData)
	lcInput := &autoscaling.CreateLaunchConfigurationInput{
		AssociatePublicIpAddress: B(true),
		LaunchConfigurationName:  &expected.(*Lc).Name,
		ImageId:                  &expected.(*Lc).Image,
		InstanceType:             &expected.(*Lc).InstanceType,
		KeyName:                  &immutable.SSH.Identifier,
		SecurityGroups:           sgs,
		UserData:                 &b64data,
	}

	spotPrice, err := strconv.ParseFloat(*&expected.(*Lc).SpotPrice, 64)
	if *&expected.(*Lc).InstanceType != cluster.ServerPoolTypeMaster && err == nil && spotPrice > 0 {
		lcInput.SpotPrice = &expected.(*Lc).SpotPrice
	}
	_, err = Sdk.ASG.CreateLaunchConfiguration(lcInput)
	if err != nil {
		if awserr, ok := err.(awserr.Error); ok {
			switch awserr.Code() {
			case autoscaling.ErrCodeAlreadyExistsFault:
				logger.Debug(autoscaling.ErrCodeAlreadyExistsFault, awserr.Error())
			case autoscaling.ErrCodeLimitExceededFault:
				logger.Debug(autoscaling.ErrCodeLimitExceededFault, awserr.Error())
			case autoscaling.ErrCodeResourceContentionFault:
				logger.Debug(autoscaling.ErrCodeResourceContentionFault, awserr.Error())
			default:
				logger.Debug(awserr.Error())
			}
		} else {
			logger.Debug(err.Error())
		}
		return nil, nil, err
	}
	logger.Info("Created Launch Configuration [%s]", r.Name)
	newResource.Image = expected.(*Lc).Image
	newResource.InstanceType = expected.(*Lc).InstanceType
	newResource.Name = expected.(*Lc).Name
	newResource.Identifier = expected.(*Lc).Name
	newResource.BootstrapScripts = r.ServerPool.BootstrapScripts

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Lc) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("lc.Delete")
	deleteResource := actual.(*Lc)
	if deleteResource.Name == "" {
		return nil, nil, fmt.Errorf("Unable to delete Launch Configuration resource without Name [%s]", deleteResource.Name)
	}
	input := &autoscaling.DeleteLaunchConfigurationInput{
		LaunchConfigurationName: &actual.(*Lc).Name,
	}
	_, err := Sdk.ASG.DeleteLaunchConfiguration(input)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("Deleted Launch Configuration [%s]", actual.(*Lc).Name)

	// Kubernetes API
	// Todo (@kris-nova) this obviously isn't immutable
	immutable.KubernetesAPI.Endpoint = ""

	newResource := &Lc{}
	newResource.Name = actual.(*Lc).Name
	newResource.Tags = actual.(*Lc).Tags
	newResource.Image = actual.(*Lc).Image
	newResource.InstanceType = actual.(*Lc).InstanceType
	newResource.BootstrapScripts = actual.(*Lc).BootstrapScripts

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *Lc) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("lc.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	serverPool := &cluster.ServerPool{}
	serverPool.Image = newResource.(*Lc).Image
	serverPool.Size = newResource.(*Lc).InstanceType
	serverPool.BootstrapScripts = newResource.(*Lc).BootstrapScripts
	found := false
	for i := 0; i < len(newCluster.ServerPools); i++ {
		if newCluster.ServerPools[i].Name == newResource.(*Lc).Name {
			newCluster.ServerPools[i].Image = newResource.(*Lc).Image
			newCluster.ServerPools[i].Size = newResource.(*Lc).InstanceType
			newCluster.ServerPools[i].BootstrapScripts = newResource.(*Lc).BootstrapScripts
			found = true
		}
	}
	if !found {
		newCluster.ServerPools = append(newCluster.ServerPools, serverPool)
	}

	return newCluster
}
