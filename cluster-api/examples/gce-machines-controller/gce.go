/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	gceconfig "k8s.io/kube-deploy/cluster-api/examples/gce-machines-controller/apis/gceproviderconfig"
	gceconfigv1 "k8s.io/kube-deploy/cluster-api/examples/gce-machines-controller/apis/gceproviderconfig/v1alpha1"
)

type GCEClient struct {
	service      *compute.Service
	scheme       *runtime.Scheme
	codecFactory *serializer.CodecFactory
	kube         *rest.RESTClient
}

func New(kube *rest.RESTClient) (*GCEClient, error) {
	client, err := google.DefaultClient(context.TODO(), compute.ComputeScope)
	if err != nil {
		return nil, err
	}

	service, err := compute.New(client)
	if err != nil {
		return nil, err
	}

	scheme, codecFactory, err := gceconfigv1.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}

	return &GCEClient{
		service:      service,
		scheme:       scheme,
		codecFactory: codecFactory,
		kube:         kube,
	}, nil
}

func (gce *GCEClient) CreateVM(machine *clusterv1.Machine) error {
	if err := gce.validateConfig(machine); err != nil {
		errorReason := clusterv1.InvalidConfigurationMachineError
		errorMessage := fmt.Sprintf("Invalid configuration: %v", err)

		update := machine.DeepCopy()
		update.Status.ErrorReason = &errorReason
		update.Status.ErrorMessage = &errorMessage

		kerr := gce.kube.Put().
			Name(update.ObjectMeta.Name).
			Resource(clusterv1.MachineResourcePlural).
			Body(update).
			Do().
			Error()
		if kerr != nil {
			fmt.Printf("error updating machine status: %v\n", kerr)
		} else {
			fmt.Printf("updated machine status\n")
		}

		return err
	}

	config, err := gce.providerConfig(machine)
	if err != nil {
		return err
	}

	startupScript := nodeStartupTemplate

	_, err = gce.service.Instances.Insert(config.Project, config.Zone, &compute.Instance{
		Name:        machine.ObjectMeta.Name,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", config.Zone, config.MachineType),
		NetworkInterfaces: []*compute.NetworkInterface{
			{
				Network: "global/networks/default",
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: "External NAT",
					},
				},
			},
		},
		Disks: []*compute.AttachedDisk{
			{
				AutoDelete: true,
				Boot:       true,
				InitializeParams: &compute.AttachedDiskInitializeParams{
					SourceImage: config.Image,
					DiskSizeGb:  10,
				},
			},
		},
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				&compute.MetadataItems{
					Key:   "startup-script",
					Value: &startupScript,
				},
			},
		},
	}).Do()
	return err
}

func (gce *GCEClient) DeleteVM(machine *clusterv1.Machine) error {
	config, err := gce.providerConfig(machine)
	if err != nil {
		return err
	}

	_, err = gce.service.Instances.Delete(config.Project, config.Zone, machine.ObjectMeta.Name).Do()
	return err
}

func (gce *GCEClient) validateConfig(machine *clusterv1.Machine) error {
	if !strings.HasPrefix(machine.Spec.Versions.Kubelet, "1.8.") {
		return fmt.Errorf("unsupported kubelet version: %v", machine.Spec.Versions.Kubelet)
	}
	return nil
}

func (gce *GCEClient) providerConfig(machine *clusterv1.Machine) (*gceconfig.GCEProviderConfig, error) {
	obj, gvk, err := gce.codecFactory.UniversalDecoder().Decode([]byte(machine.Spec.ProviderConfig), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decoding failure: %v", err)
	}

	config, ok := obj.(*gceconfig.GCEProviderConfig)
	if !ok {
		return nil, fmt.Errorf("failure to cast to gce; type: %v", gvk)
	}

	return config, nil
}
