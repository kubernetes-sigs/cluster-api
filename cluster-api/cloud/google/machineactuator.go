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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	compute "google.golang.org/api/compute/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
	"k8s.io/kube-deploy/cluster-api/client"
	gceconfig "k8s.io/kube-deploy/cluster-api/cloud/google/gceproviderconfig"
	gceconfigv1 "k8s.io/kube-deploy/cluster-api/cloud/google/gceproviderconfig/v1alpha1"
	apierrors "k8s.io/kube-deploy/cluster-api/errors"
	"k8s.io/kube-deploy/cluster-api/util"
)

const (
	ProjectAnnotationKey = "gcp-project"
	ZoneAnnotationKey    = "gcp-zone"
	NameAnnotationKey    = "gcp-name"

	UIDLabelKey = "machine-crd-uid"
)

type SshCreds struct {
	user           string
	privateKeyPath string
}

type GCEClient struct {
	service       *compute.Service
	scheme        *runtime.Scheme
	codecFactory  *serializer.CodecFactory
	kubeadmToken  string
	sshCreds      SshCreds
	machineClient client.MachinesInterface
}

const (
	gceTimeout   = time.Minute * 10
	gceWaitSleep = time.Second * 5
)

func NewMachineActuator(kubeadmToken string, machineClient client.MachinesInterface) (*GCEClient, error) {
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
		machineClient: machineClient,
	}, nil
}

func (gce *GCEClient) CreateMachineController(cluster *clusterv1.Cluster, initialMachines []*clusterv1.Machine) error {
	if err := gce.CreateMachineControllerServiceAccount(cluster, initialMachines); err != nil {
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
		return gce.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err))
	}

	if verr := gce.validateMachine(machine, config); verr != nil {
		return gce.handleMachineError(machine, verr)
	}

	var metadata map[string]string
	if cluster.Spec.ClusterNetwork.DNSDomain == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.DNSDomain")
	}
	if cluster.Spec.ClusterNetwork.PodSubnet == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.PodSubnet")
	}
	if cluster.Spec.ClusterNetwork.ServiceSubnet == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.ServiceSubnet")
	}
	if machine.Spec.Versions.Kubelet == "" {
		return errors.New("invalid master configuration: missing Machine.Spec.Versions.Kubelet")
	}

	image, preloaded := gce.getImage(machine, config)

	if util.IsMaster(machine) {
		if machine.Spec.Versions.ControlPlane == "" {
			return gce.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
				"invalid master configuration: missing Machine.Spec.Versions.ControlPlane"))
		}
		var err error
		metadata, err = masterMetadata(
			templateParams{
				Token:     gce.kubeadmToken,
				Cluster:   cluster,
				Machine:   machine,
				Preloaded: preloaded,
			},
		)
		if err != nil {
			return err
		}
	} else {
		if len(cluster.Status.APIEndpoints) == 0 {
			return errors.New("invalid cluster state: cannot create a Kubernetes node without an API endpoint")
		}
		var err error
		metadata, err = nodeMetadata(
			templateParams{
				Token:     gce.kubeadmToken,
				Cluster:   cluster,
				Machine:   machine,
				Preloaded: preloaded,
			},
		)
		if err != nil {
			return err
		}
	}

	var metadataItems []*compute.MetadataItems
	for k, v := range metadata {
		v := v // rebind scope to avoid loop aliasing below
		metadataItems = append(metadataItems, &compute.MetadataItems{
			Key:   k,
			Value: &v,
		})
	}

	name := machine.ObjectMeta.Name
	project := config.Project
	zone := config.Zone
	diskSize := int64(10)

	// Our preloaded image already has a lot stored on it, so increase the
	// disk size to have more free working space.
	if preloaded {
		diskSize = 30
	}

	op, err := gce.service.Instances.Insert(project, zone, &compute.Instance{
		Name:        name,
		MachineType: fmt.Sprintf("zones/%s/machineTypes/%s", zone, config.MachineType),
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
					SourceImage: image,
					DiskSizeGb:  diskSize,
				},
			},
		},
		Metadata: &compute.Metadata{
			Items: metadataItems,
		},
		Tags: &compute.Tags{
			Items: []string{"https-server"},
		},
		Labels: map[string]string{
			UIDLabelKey: fmt.Sprintf("%v", machine.ObjectMeta.UID),
		},
	}).Do()

	if err == nil {
		err = gce.waitForOperation(config, op)
	}

	if err != nil {
		return gce.handleMachineError(machine, apierrors.CreateMachine(
			"error creating GCE instance: %v", err))
	}

	// If we have a machineClient, then annotate the machine so that we
	// remember exactly what VM we created for it.
	if gce.machineClient != nil {
		if machine.ObjectMeta.Annotations == nil {
			machine.ObjectMeta.Annotations = make(map[string]string)
		}
		machine.ObjectMeta.Annotations[ProjectAnnotationKey] = project
		machine.ObjectMeta.Annotations[ZoneAnnotationKey] = zone
		machine.ObjectMeta.Annotations[NameAnnotationKey] = name
		_, err := gce.machineClient.Update(machine)
		return err
	}
	return nil
}

func (gce *GCEClient) Delete(machine *clusterv1.Machine) error {
	config, err := gce.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return gce.handleMachineError(machine,
			apierrors.InvalidMachineConfiguration("Cannot unmarshal providerConfig field: %v", err))
	}

	if verr := gce.validateMachine(machine, config); verr != nil {
		return gce.handleMachineError(machine, verr)
	}

	var project, zone, name string

	if machine.ObjectMeta.Annotations != nil {
		project = machine.ObjectMeta.Annotations[ProjectAnnotationKey]
		zone = machine.ObjectMeta.Annotations[ZoneAnnotationKey]
		name = machine.ObjectMeta.Annotations[NameAnnotationKey]
	}

	// If the annotations are missing, fall back on providerConfig
	if project == "" || zone == "" || name == "" {
		project = config.Project
		zone = config.Zone
		name = machine.ObjectMeta.Name
	}

	op, err := gce.service.Instances.Delete(project, zone, name).Do()
	if err == nil {
		err = gce.waitForOperation(config, op)
	}
	if err != nil {
		return gce.handleMachineError(machine, apierrors.DeleteMachine(
			"error deleting GCE instance: %v", err))
	}
	return nil
}

func (gce *GCEClient) PostDelete(cluster *clusterv1.Cluster, machines []*clusterv1.Machine) error {
	return gce.DeleteMachineControllerServiceAccount(cluster, machines)
}

func (gce *GCEClient) Update(cluster *clusterv1.Cluster, oldMachine *clusterv1.Machine, newMachine *clusterv1.Machine) error {
	// Before updating, do some basic validation of the object first.
	config, err := gce.providerconfig(newMachine.Spec.ProviderConfig)
	if err != nil {
		return gce.handleMachineError(newMachine,
			apierrors.InvalidMachineConfiguration("Cannot unmarshal providerConfig field: %v", err))
	}
	if verr := gce.validateMachine(newMachine, config); verr != nil {
		return gce.handleMachineError(newMachine, verr)
	}

	if util.IsMaster(oldMachine) {
		glog.Infof("Doing an in-place upgrade for master.\n")
		err = gce.updateMasterInplace(oldMachine, newMachine)
		if err != nil {
			glog.Errorf("master inplace update failed: %v", err)
		}
	} else {
		// It's a little counter-intuitive to Delete(newMachine)
		// instead of Delete(oldMachine), but this fixes the case where
		// a Machine is edited to have invalid configuration, and then
		// fixed with correct configuration. If we try to do
		// Delete(oldMachine), the old Spec is invalid, and we might
		// not even be able to unmarshal the ProviderConfig, so the
		// deletion will fail.
		//
		// There is still an edge case we need to handle where someone
		// wants to change the project or zone of the Machine in the
		// ProviderConfig, where using Delete(newMachine) won't
		// actually delete the correct VM.
		glog.Infof("re-creating machine %s for update.", newMachine.ObjectMeta.Name)
		err = gce.Delete(newMachine)
		if err != nil {
			glog.Errorf("delete machine %s for update failed: %v", newMachine.ObjectMeta.Name, err)
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
	glog.Infof("Wait for %v %q...", op.OperationType, op.Name)
	defer glog.Infof("Finish wait for %v %q...", op.OperationType, op.Name)

	start := time.Now()
	ctx, cf := context.WithTimeout(context.Background(), gceTimeout)
	defer cf()

	var err error
	for {
		if err = gce.checkOp(op, err); err != nil || op.Status == "DONE" {
			return err
		}
		glog.V(1).Infof("Wait for %v %q: %v (%d%%): %v", op.OperationType, op.Name, op.Status, op.Progress, op.StatusMessage)
		select {
		case <-ctx.Done():
			return fmt.Errorf("gce operation %v %q timed out after %v", op.OperationType, op.Name, time.Since(start))
		case <-time.After(gceWaitSleep):
		}
		op, err = gce.getOp(c, op)
	}
}

// getOp returns an updated operation.
func (gce *GCEClient) getOp(c *gceconfig.GCEProviderConfig, op *compute.Operation) (*compute.Operation, error) {
	return gce.service.ZoneOperations.Get(c.Project, path.Base(op.Zone), op.Name).Do()
}

func (gce *GCEClient) checkOp(op *compute.Operation, err error) error {
	if err != nil || op.Error == nil || len(op.Error.Errors) == 0 {
		return err
	}

	var errs bytes.Buffer
	for _, v := range op.Error.Errors {
		errs.WriteString(v.Message)
		errs.WriteByte('\n')
	}
	return errors.New(errs.String())
}

func (gce *GCEClient) updateMasterInplace(oldMachine *clusterv1.Machine, newMachine *clusterv1.Machine) error {
	if oldMachine.Spec.Versions.ControlPlane != newMachine.Spec.Versions.ControlPlane {
		// First pull off the latest kubeadm.
		cmd := "export KUBEADM_VERSION=$(curl -sSL https://dl.k8s.io/release/stable.txt); " +
			"curl -sSL https://dl.k8s.io/release/${KUBEADM_VERSION}/bin/linux/amd64/kubeadm | sudo tee /usr/bin/kubeadm > /dev/null; " +
			"sudo chmod a+rx /usr/bin/kubeadm"
		_, err := gce.remoteSshCommand(newMachine, cmd)
		if err != nil {
			glog.Infof("remotesshcomand error: %v", err)
			return err
		}

		// TODO: We might want to upgrade kubeadm if the target control plane version is newer.
		// Upgrade control plan.
		cmd = fmt.Sprintf("sudo kubeadm upgrade apply %s -y", "v"+newMachine.Spec.Versions.ControlPlane)
		_, err = gce.remoteSshCommand(newMachine, cmd)
		if err != nil {
			glog.Infof("remotesshcomand error: %v", err)
			return err
		}
	}

	// Upgrade kubelet.
	if oldMachine.Spec.Versions.Kubelet != newMachine.Spec.Versions.Kubelet {
		cmd := fmt.Sprintf("sudo kubectl drain %s --kubeconfig /etc/kubernetes/admin.conf --ignore-daemonsets", newMachine.Name)
		// The errors are intentionally ignored as master has static pods.
		gce.remoteSshCommand(newMachine, cmd)
		// Upgrade kubelet to desired version.
		cmd = fmt.Sprintf("sudo apt-get install kubelet=%s", newMachine.Spec.Versions.Kubelet+"-00")
		_, err := gce.remoteSshCommand(newMachine, cmd)
		if err != nil {
			glog.Infof("remotesshcomand error: %v", err)
			return err
		}
		cmd = fmt.Sprintf("sudo kubectl uncordon %s --kubeconfig /etc/kubernetes/admin.conf", newMachine.Name)
		_, err = gce.remoteSshCommand(newMachine, cmd)
		if err != nil {
			glog.Infof("remotesshcomand error: %v", err)
			return err
		}
	}

	return nil
}

func (gce *GCEClient) validateMachine(machine *clusterv1.Machine, config *gceconfig.GCEProviderConfig) *apierrors.MachineError {
	if machine.Spec.Versions.Kubelet == "" {
		return apierrors.InvalidMachineConfiguration("spec.versions.kubelet can't be empty")
	}
	if machine.Spec.Versions.ContainerRuntime.Name != "docker" {
		return apierrors.InvalidMachineConfiguration("Only docker is supported")
	}
	if machine.Spec.Versions.ContainerRuntime.Version != "1.12.0" {
		return apierrors.InvalidMachineConfiguration("Only docker 1.12.0 is supported")
	}
	return nil
}

// If the GCEClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (gce *GCEClient) handleMachineError(machine *clusterv1.Machine, err *apierrors.MachineError) error {
	if gce.machineClient != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		gce.machineClient.Update(machine)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}

func (gce *GCEClient) getImage(machine *clusterv1.Machine, config *gceconfig.GCEProviderConfig) (image string, isPreloaded bool) {
	project := config.Project
	imgName := "prebaked-ubuntu-1604-lts"
	fullName := fmt.Sprintf("projects/%s/global/images/%s", project, imgName)

	// Check to see if a preloaded image exists in this project. If so, use it.
	_, err := gce.service.Images.Get(project, imgName).Do()
	if err == nil {
		return fullName, true
	}

	// Otherwise, fall back to the non-preloaded base image.
	return "projects/ubuntu-os-cloud/global/images/family/ubuntu-1604-lts", false
}
