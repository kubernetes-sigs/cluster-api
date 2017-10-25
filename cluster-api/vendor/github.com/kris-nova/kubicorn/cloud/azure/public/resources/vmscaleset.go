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

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/azuremaps"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/cutil/defaults"
	"github.com/kris-nova/kubicorn/cutil/logger"
	//"github.com/kris-nova/kubicorn/cutil/retry"
	//"bytes"
	//"encoding/json"
	"github.com/kris-nova/kubicorn/cutil/retry"
	//"time"
	"time"
)

var _ cloud.Resource = &VMScaleSet{}

type VMScaleSet struct {
	Shared
	ServerPool *cluster.ServerPool
	Image      string
}

func (r *VMScaleSet) Actual(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vmscaleset.Actual")

	newResource := &VMScaleSet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: r.ServerPool.Identifier,
		},
	}

	if r.ServerPool.Identifier != "" {
		vsss, err := Sdk.Compute.List(immutable.Name)
		if err != nil {
			return nil, nil, err
		}
		for _, vss := range *vsss.Value {
			if r.ServerPool.Identifier == *vss.ID {
				logger.Debug("Looked up VM scale set [%s]", *vss.ID)
				newResource.Identifier = *vss.ID
				newResource.Image = *vss.Sku.Name
				// todo (@kris-nova) we need to define WAY more here
			}
		}

	}

	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *VMScaleSet) Expected(immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vmscaleset.Expected")
	newResource := &VMScaleSet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: r.ServerPool.Identifier,
		},
		Image: r.ServerPool.Image,
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *VMScaleSet) Apply(actual, expected cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vmscaleset.Apply")
	applyResource := expected.(*VMScaleSet)
	isEqual, err := compare.IsEqual(actual.(*VMScaleSet), expected.(*VMScaleSet))
	if err != nil {
		return nil, nil, err
	}
	if isEqual {
		return immutable, applyResource, nil
	}

	if r.ServerPool.Type == cluster.ServerPoolTypeMaster {

		// -------------------------------------------------------------------------------------
		// IP Configs
		var ipConfigsToAdd []compute.VirtualMachineScaleSetIPConfiguration
		for _, serverPool := range immutable.ServerPools {
			if serverPool.Type == cluster.ServerPoolTypeMaster {
				for _, subnet := range serverPool.Subnets {
					var backEndPools []compute.SubResource

					for _, id := range subnet.LoadBalancer.BackendIDs {
						backEndPools = append(backEndPools, compute.SubResource{ID: &id})
					}
					var inboundNatPools []compute.SubResource
					for _, id := range subnet.LoadBalancer.NATIDs {
						myId := id
						inboundNatPools = append(inboundNatPools, compute.SubResource{ID: &myId})
					}

					newIpConfig := compute.VirtualMachineScaleSetIPConfiguration{
						VirtualMachineScaleSetIPConfigurationProperties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
							Subnet: &compute.APIEntityReference{
								ID: s(subnet.Identifier),
							},
							LoadBalancerBackendAddressPools: &backEndPools,
							LoadBalancerInboundNatPools:     &inboundNatPools,
						},
						Name: &serverPool.Name,
					}
					ipConfigsToAdd = append(ipConfigsToAdd, newIpConfig)
				}
			}
		}

		//// ----- Debug request
		//byteslice, _ := json.Marshal(ipConfigsToAdd)
		//var out bytes.Buffer
		//json.Indent(&out, byteslice, "", "  ")
		//fmt.Println(string(out.Bytes()))

		imageRef, err := azuremaps.GetImageReferenceFromImage(r.ServerPool.Image)
		if err != nil {
			return nil, nil, err
		}
		nicName := fmt.Sprintf("%s-nic", immutable.Name)
		vhdcontainername := fmt.Sprintf("https://%s.blob.core.windows.net/%s", immutable.Name, immutable.Name)
		parameters := compute.VirtualMachineScaleSet{
			Location: &immutable.Location,
			VirtualMachineScaleSetProperties: &compute.VirtualMachineScaleSetProperties{
				Overprovision: b(true),
				VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{

					// StorageProfile
					StorageProfile: &compute.VirtualMachineScaleSetStorageProfile{
						OsDisk: &compute.VirtualMachineScaleSetOSDisk{
							OsType:       compute.Linux,
							CreateOption: compute.DiskCreateOptionTypesFromImage,
							VhdContainers: &[]string{
								vhdcontainername,
							},
							Name: s(applyResource.Name),
						},
						ImageReference: imageRef,
					},

					// OsProfile
					OsProfile: &compute.VirtualMachineScaleSetOSProfile{
						AdminUsername:      s(immutable.SSH.User),
						ComputerNamePrefix: s(r.ServerPool.Name),
						LinuxConfiguration: &compute.LinuxConfiguration{
							DisablePasswordAuthentication: b(true),
							SSH: &compute.SSHConfiguration{
								PublicKeys: &[]compute.SSHPublicKey{
									{
										KeyData: s(string(immutable.SSH.PublicKeyData)),
										Path:    s(fmt.Sprintf("/home/%s/.ssh/authorized_keys", immutable.SSH.User)),
									},
								},
							},
						},
					},

					// NetworkProfile
					NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
						NetworkInterfaceConfigurations: &[]compute.VirtualMachineScaleSetNetworkConfiguration{
							{
								VirtualMachineScaleSetNetworkConfigurationProperties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
									IPConfigurations: &ipConfigsToAdd,
									Primary:          b(true),
								},
								Name: &nicName,
							},
						},
					},
				},
				UpgradePolicy: &compute.UpgradePolicy{
					Mode: compute.Automatic,
				},
			},
			Sku: &compute.Sku{
				Name:     s(r.ServerPool.Size),
				Tier:     s(azuremaps.GetTierFromSize(r.ServerPool.Size)),
				Capacity: i64(int64(r.ServerPool.MaxCount)),
			},
		}
		vmssch, errch := Sdk.Compute.CreateOrUpdate(immutable.Name, applyResource.Name, parameters, make(chan struct{}))

		// ---------------
		// Hack in here to fluff a vm scale set if the ARM API is having bad weather
		logger.Debug("[vmss hack] Begin hack searching for vmss")

		timer := time.NewTimer(time.Duration(15 * time.Second))
		defer timer.Stop()
		var vmss compute.VirtualMachineScaleSet
		loop := true
		for loop {
			select {
			case vmss = <-vmssch:
				// vmss
				logger.Debug("[vmss hack] Received vmss on channel")
				loop = false
				break
			case err = <-errch:
				if err != nil {
					logger.Debug("[vmss hack] Error")
					return nil, nil, err
				} else {
					logger.Debug("[vmss hack] Error returned nil")
				}
			case <-timer.C:
				logger.Debug("[vmss hack] [%d] second wait timer exceeded!", 15)
				loop = false
				break

				// Todo support sig handling here
			}
		}

		if vmss.ID == nil {

			l := &lookUpVmSsRetrier{
				resourceGroup: immutable.Name,
				vmssname:      applyResource.Name,
			}
			retrier := retry.NewRetrier(20, 5, l)
			err = retrier.RunRetry()
			if err != nil {
				return nil, nil, err
			}
			vmss = retrier.Retryable().(*lookUpVmSsRetrier).vmss

		}

		logger.Info("Created or found vm scale set [%s]", *vmss.ID)
		newResource := &VMScaleSet{
			Shared: Shared{
				Name:       r.Name,
				Tags:       r.Tags,
				Identifier: *vmss.ID,
			},
		}
		newCluster := r.immutableRender(newResource, immutable)
		return newCluster, newResource, nil
	}

	// slave
	newResource := &VMScaleSet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: "",
		},
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

type lookUpVmSsRetrier struct {
	resourceGroup string
	vmssname      string
	vmss          compute.VirtualMachineScaleSet
}

func (l *lookUpVmSsRetrier) Try() error {
	vmss, err := Sdk.Compute.Get(l.resourceGroup, l.vmssname)
	if err != nil {
		// fmt.Println(err)
		return err
	}
	if vmss.ID == nil || *vmss.ID == "" {
		// fmt.Println("empty id")
		return fmt.Errorf("Empty id")
	}
	l.vmss = vmss
	return nil
}

func (r *VMScaleSet) Delete(actual cloud.Resource, immutable *cluster.Cluster) (*cluster.Cluster, cloud.Resource, error) {
	logger.Debug("vmscaleset.Delete")
	deleteResource := actual.(*VMScaleSet)
	if deleteResource.Identifier == "" {
		return immutable, actual, nil
		return nil, nil, fmt.Errorf("Unable to delete VPC resource without ID [%s]", deleteResource.Name)
	}
	_, errch := Sdk.Compute.Delete(immutable.Name, deleteResource.Name, make(chan struct{}))
	err := <-errch
	if err != nil {
		return nil, nil, err
	}
	newResource := &VMScaleSet{
		Shared: Shared{
			Name:       r.Name,
			Tags:       r.Tags,
			Identifier: "",
		},
	}
	newCluster := r.immutableRender(newResource, immutable)
	return newCluster, newResource, nil
}

func (r *VMScaleSet) immutableRender(newResource cloud.Resource, inaccurateCluster *cluster.Cluster) *cluster.Cluster {
	logger.Debug("vmscaleset.Render")
	newCluster := defaults.NewClusterDefaults(inaccurateCluster)
	for _, serverPool := range newCluster.ServerPools {
		if serverPool.Name == newResource.(*VMScaleSet).Name {
			serverPool.Identifier = newResource.(*VMScaleSet).Identifier
		}
	}
	return newCluster
}
