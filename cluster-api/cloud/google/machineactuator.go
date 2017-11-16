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
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	//"os/exec"
	"io/ioutil"

	"github.com/golang/glog"
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

type SshCreds struct {
	user           string
	privateKeyPath string
}

type GCEClient struct {
	service      *compute.Service
	scheme       *runtime.Scheme
	codecFactory *serializer.CodecFactory
	kubeadmToken string
	sshCreds     SshCreds
}

func NewMachineActuator(kubeadmToken string) (*GCEClient, error) {
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

	// Only applicable if it's running inside machine controller pod.
	var privateKeyPath, user string
	if _, err := os.Stat("/etc/sshkeys/private"); err == nil {
		privateKeyPath = "/etc/sshkeys/private"

		b, err := ioutil.ReadFile("/etc/sshkeys/user")
		if err == nil {
			user = string(b)
		} else {
			return nil, err
		}
	}

	return &GCEClient{
		service:      service,
		scheme:       scheme,
		codecFactory: codecFactory,
		kubeadmToken: kubeadmToken,
		sshCreds: SshCreds{
			privateKeyPath: privateKeyPath,
			user:           user,
		},
	}, nil
}

func (gce *GCEClient) CreateMachineController(initialMachines []*clusterv1.Machine) error {
	// Figure out what projects the service account needs permission to.
	var projects []string
	for _, machine := range initialMachines {
		config, err := gce.providerconfig(machine.Spec.ProviderConfig)
		if err != nil {
			return err
		}

		projects = append(projects, config.Project)
	}

	if err := CreateMachineControllerServiceAccount(projects); err != nil {
		return err
	}

	// Setup SSH access to master VM
	if err := gce.setupSSHAccess(util.GetMaster(initialMachines)); err != nil {
		return err
	}

	if err := CreateMachineControllerPod(gce.kubeadmToken); err != nil {
		return err
	}
	return nil
}

func (gce *GCEClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	config, err := gce.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return err
	}

	var startupScript string
	dnsDomain := cluster.Spec.ClusterNetwork.DNSDomain
	podCIDR := cluster.Spec.ClusterNetwork.PodSubnet
	serviceCIDR := cluster.Spec.ClusterNetwork.ServiceSubnet
	// TODO we need to have API defaults, but I don't think CRDs have defaulting
	if dnsDomain == "" {
		dnsDomain = "cluster.local"
	}
	if podCIDR == "" {
		podCIDR = "192.168.0.0/16"
	}
	if serviceCIDR == "" {
		serviceCIDR = "10.96.0.0/12"
	}
	if util.IsMaster(machine) {
		kubeletVersion := machine.Spec.Versions.Kubelet
		controlPlaneVersion := machine.Spec.Versions.ControlPlane
		if kubeletVersion == "" {
			return fmt.Errorf("invalid master configuration: missing Machine.Spec.Versions.Kubelet")
		}
		if controlPlaneVersion == "" {
			return fmt.Errorf("invalid master configuration: missing Machine.Spec.Versions.ControlPlane")
		}
		startupScript = masterStartupScript(
			gce.kubeadmToken,
			"443",
			machine.ObjectMeta.Name,
			kubeletVersion,
			controlPlaneVersion,
			dnsDomain,
			podCIDR,
			serviceCIDR,
		)
	} else {
		master := ""
		for _, endpoint := range cluster.Status.APIEndpoints {
			master = fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
		}
		if master == "" {
			return errors.New("missing master endpoints in cluster")
		}
		startupScript = nodeStartupScript(
			gce.kubeadmToken,
			master,
			machine.ObjectMeta.Name,
			machine.Spec.Versions.Kubelet,
			dnsDomain,
			serviceCIDR,
		)
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

func (gce *GCEClient) PostDelete(machines []*clusterv1.Machine) error {
	var projects []string
	for _, machine := range machines {
		config, err := gce.providerconfig(machine.Spec.ProviderConfig)
		if err != nil {
			return err
		}

		projects = append(projects, config.Project)
	}

	return DeleteMachineControllerServiceAccount(projects)
}

func (gce *GCEClient) Update(cluster *clusterv1.Cluster, oldMachine *clusterv1.Machine, newMachine *clusterv1.Machine) error {
	var err error
	if util.IsMaster(oldMachine) {
		glog.Infof("Doing an in-place upgrade for master.\n")
		err = gce.updateMasterInplace(oldMachine, newMachine)
		if err != nil {
			glog.Errorf("master inplace update failed: %v", err)
		}
	} else {
		glog.Infof("re-creating machine %s for update.", oldMachine.ObjectMeta.Name)
		err = gce.Delete(oldMachine)
		if err != nil {
			glog.Errorf("delete machine %s for update failed: %v", oldMachine.ObjectMeta.Name, err)
		} else {
			err = gce.Create(cluster, newMachine)
			if err != nil {
				glog.Errorf("create machine %s for update failed: %v", newMachine.ObjectMeta.Name, err)
			}
		}
	}
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

	var publicIP string

	for _, networkInterface := range instance.NetworkInterfaces {
		if networkInterface.Name == "nic0" {
			for _, accessConfigs := range networkInterface.AccessConfigs {
				publicIP = accessConfigs.NatIP
			}
		}
	}
	return publicIP, nil
}

func (gce *GCEClient) GetKubeConfig(master *clusterv1.Machine) (string, error) {
	config, err := gce.providerconfig(master.Spec.ProviderConfig)
	if err != nil {
		return "", err
	}

	command := "echo STARTFILE; sudo cat /etc/kubernetes/admin.conf"
	result := strings.TrimSpace(util.ExecCommand(
		"gcloud", "compute", "ssh", "--project", config.Project,
		"--zone", config.Zone, master.ObjectMeta.Name, "--command", command))
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

func (gce *GCEClient) updateMasterInplace(oldMachine *clusterv1.Machine, newMachine *clusterv1.Machine) error {
	if oldMachine.Spec.Versions.ControlPlane != newMachine.Spec.Versions.ControlPlane {
		// TODO: We might want to upgrade kubeadm if the target control plane version is newer.
		// Upgrade control plan.
		cmd := fmt.Sprintf("sudo kubeadm upgrade apply %s -y", "v"+newMachine.Spec.Versions.ControlPlane)
		_, err := gce.remoteSshCommand(newMachine, cmd)
		if err != nil {
			glog.Infof("remotesshcomand error: %v", err)
			return err
		}
	}

	// Upgrade kubelet.
	// TODO: Mark master as unscheduleable and evict the workloads.
	if oldMachine.Spec.Versions.Kubelet != newMachine.Spec.Versions.Kubelet {
		cmd := fmt.Sprintf("sudo apt-get install kubelet=%s", newMachine.Spec.Versions.Kubelet+"-00")
		_, err := gce.remoteSshCommand(newMachine, cmd)
		if err != nil {
			glog.Infof("remotesshcomand error: %v", err)
			return err
		}
	}

	return nil
}
