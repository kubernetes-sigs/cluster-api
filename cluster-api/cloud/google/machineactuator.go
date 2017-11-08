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

package google

import (
	"fmt"
	"path"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	gceconfig "k8s.io/kube-deploy/cluster-api/cloud/google/gceproviderconfig"
	gceconfigv1 "k8s.io/kube-deploy/cluster-api/cloud/google/gceproviderconfig/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/util"
)

type GCEClient struct {
	service      *compute.Service
	scheme       *runtime.Scheme
	codecFactory *serializer.CodecFactory
	kubeadmToken string
	masterIP     string
}

func NewMachineActuator(kubeadmToken string, masterIP string) (*GCEClient, error) {
	// The default GCP client expects the environment variable
	// GOOGLE_APPLICATION_CREDENTIALS to point to a file with service credentials.
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
		kubeadmToken: kubeadmToken,
		masterIP:     masterIP,
	}, nil
}

func (gce *GCEClient) CreateMachineController(cluster *clusterv1.Cluster) error {
	config, err := gce.providerconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return err
	}
	if err := CreateMachineControllerServiceAccount(config.Project); err != nil {
		return err
	}
	if err := CreateMachineControllerPod(gce.kubeadmToken); err != nil {
		return err
	}
	return nil
}

func (gce *GCEClient) Create(machine *clusterv1.Machine) error {
	config, err := gce.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return err
	}

	var startupScript string
	if util.IsMaster(machine) {
		startupScript = masterStartupScript(gce.kubeadmToken, "443")
	} else {
		startupScript = nodeStartupScript(gce.kubeadmToken, gce.masterIP, machine.ObjectMeta.Name, machine.Spec.Versions.Kubelet)
	}

	op, err := gce.service.Instances.Insert(config.Project, config.Zone, &compute.Instance{
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
		Tags: &compute.Tags{
			Items: []string{"https-server"},
		},
	}).Do()

	if err != nil {
		return err
	}

	return gce.waitForOperation(config, op)
}

func (gce *GCEClient) Delete(machine *clusterv1.Machine) error {
	config, err := gce.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return err
	}

	op, err := gce.service.Instances.Delete(config.Project, config.Zone, machine.ObjectMeta.Name).Do()
	gce.waitForOperation(config, op)
	return err
}

func (gce *GCEClient) Get(name string) (*clusterv1.Machine, error) {
	return nil, fmt.Errorf("Get machine is not implemented on Google")
}

func (gce *GCEClient) GetIP(machine *clusterv1.Machine) (string, error) {
	config, err := gce.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return "", err
	}

	instance, err := gce.service.Instances.Get(config.Project, config.Zone, machine.ObjectMeta.Name).Do()
	if err != nil {
		return "", err
	}

	var masterIPPublic string

	for _, networkInterface := range instance.NetworkInterfaces {
		if networkInterface.Name == "nic0" {
			for _, accessConfigs := range networkInterface.AccessConfigs {
				masterIPPublic = accessConfigs.NatIP
			}
		}
	}
	return masterIPPublic, nil
}

func (gce *GCEClient) GetKubeConfig(master *clusterv1.Machine) (string, error) {
	config, err := gce.providerconfig(master.Spec.ProviderConfig)
	if err != nil {
		return "", err
	}

	command := "echo STARTFILE; sudo cat /etc/kubernetes/admin.conf"
	args := []string{"compute", "ssh", "--project", config.Project, "--zone", config.Zone, master.ObjectMeta.Name, "--command", command}
	result := strings.TrimSpace(util.ExecCommand("gcloud", args))
	parts := strings.Split(result, "STARTFILE")
	if len(parts) != 2 {
		return "", nil
	}
	return strings.TrimSpace(parts[1]), nil
}

func (gce *GCEClient) providerconfig(providerConfig string) (*gceconfig.GCEProviderConfig, error) {
	obj, gvk, err := gce.codecFactory.UniversalDecoder().Decode([]byte(providerConfig), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decoding failure: %v", err)
	}

	config, ok := obj.(*gceconfig.GCEProviderConfig)
	if !ok {
		return nil, fmt.Errorf("failure to cast to gce; type: %v", gvk)
	}

	return config, nil
}

func (gce *GCEClient) waitForOperation(c *gceconfig.GCEProviderConfig, op *compute.Operation) error {
	for op != nil && op.Status != "DONE" {
		var err error
		op, err = gce.getOp(c, op)
		if err != nil {
			return err
		}
	}
	return nil
}

// getOp returns an updated operation.
func (gce *GCEClient) getOp(c *gceconfig.GCEProviderConfig, op *compute.Operation) (*compute.Operation, error) {
	return gce.service.ZoneOperations.Get(c.Project, path.Base(op.Zone), op.Name).Do()
}
