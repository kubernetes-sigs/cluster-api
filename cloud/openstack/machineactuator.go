/*
Copyright 2018 The Kubernetes Authors.

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

package openstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/cluster-api/cloud/openstack/clients"
	"sigs.k8s.io/cluster-api/cloud/openstack/machinesetup"
	openstackconfigv1 "sigs.k8s.io/cluster-api/cloud/openstack/openstackproviderconfig/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
	"sigs.k8s.io/cluster-api/pkg/kubeadm"
	"sigs.k8s.io/cluster-api/pkg/util"
	"time"
)

const (
	MachineSetupConfigPath   = "/etc/machinesetup/machine_setup_configs.yaml"
	SshPrivateKeyPath        = "/etc/sshkeys/private"
	SshPublicKeyPath         = "/etc/sshkeys/public"
	SshKeyUserPath           = "/etc/sshkeys/user"
	CloudConfigPath          = "/etc/cloud/cloud_config.yaml"
	OpenstackIPAnnotationKey = "openstack-ip-address"
	OpenstackIdAnnotationKey = "openstack-resourceId"

	TimeoutInstanceCreate       = 5 * time.Minute
	RetryIntervalInstanceStatus = 10 * time.Second
)

type SshCreds struct {
	user           string
	privateKeyPath string
	publicKey      string
}

type OpenstackClient struct {
	scheme              *runtime.Scheme
	kubeadm             *kubeadm.Kubeadm
	machineClient       client.MachineInterface
	machineSetupWatcher *machinesetup.ConfigWatch
	codecFactory        *serializer.CodecFactory
	machineService      *clients.InstanceService
	sshCred             *SshCreds
	*DeploymentClient
}

// readCloudConfigFromFile read cloud config from file
// which should include username/password/region/tenentID...
func readCloudConfigFromFile(path string) *clients.CloudConfig {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil
	}
	cloudConfig := &clients.CloudConfig{}
	if yaml.Unmarshal(bytes, cloudConfig) != nil {
		return nil
	}
	return cloudConfig
}

func NewMachineActuator(machineClient client.MachineInterface) (*OpenstackClient, error) {
	cloudConfig := readCloudConfigFromFile(CloudConfigPath)
	if cloudConfig == nil {
		return nil, fmt.Errorf("Get cloud config from file %q err", CloudConfigPath)
	}
	machineService, err := clients.NewInstanceService(cloudConfig)
	if err != nil {
		return nil, err
	}
	var sshCred SshCreds
	b, err := ioutil.ReadFile(SshKeyUserPath)
	if err != nil {
		return nil, err
	}
	sshCred.user = string(b)
	b, err = ioutil.ReadFile(SshPublicKeyPath)
	if err != nil {
		return nil, err
	}
	sshCred.publicKey = string(b)

	keyPairList, err := machineService.GetKeyPairList()
	if err != nil {
		return nil, err
	}
	needCreate := true
	// check whether keypair already exist
	for i := range keyPairList {
		if sshCred.user == keyPairList[i].Name {
			if sshCred.publicKey == keyPairList[i].PublicKey {
				needCreate = false
			} else {
				machineService.DeleteKeyPair(keyPairList[i].Name)
			}
			break
		}
	}
	if needCreate {
		if _, err := os.Stat(SshPrivateKeyPath); err != nil {
			return nil, fmt.Errorf("ssh key pair need to be specified")
		}
		sshCred.privateKeyPath = SshPrivateKeyPath

		if machineService.CreateKeyPair(sshCred.user, sshCred.publicKey) != nil {
			return nil, fmt.Errorf("create ssh key pair err: %v", err)
		}
	}

	scheme, codecFactory, err := openstackconfigv1.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}

	setupConfigWatcher, err := machinesetup.NewConfigWatch(MachineSetupConfigPath)
	if err != nil {
		return nil, fmt.Errorf("error creating machine setup config watcher: %v", err)
	}

	return &OpenstackClient{
		machineClient:       machineClient,
		machineService:      machineService,
		codecFactory:        codecFactory,
		machineSetupWatcher: setupConfigWatcher,
		kubeadm:             kubeadm.New(),
		scheme:              scheme,
		sshCred:             &sshCred,
		DeploymentClient:    NewDeploymentClient(),
	}, nil
}

func (oc *OpenstackClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if oc.machineSetupWatcher == nil {
		return errors.New("a valid machine setup config watcher is required!")
	}

	providerConfig, err := oc.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return oc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err))
	}

	if verr := oc.validateMachine(machine, providerConfig); verr != nil {
		return oc.handleMachineError(machine, verr)
	}

	instance, err := oc.instanceExists(machine)
	if err != nil {
		return err
	}
	if instance != nil {
		glog.Infof("Skipped creating a VM that already exists.\n")
		return nil
	}

	// get machine startup script
	machineSetupConfig, err := oc.machineSetupWatcher.GetMachineSetupConfig()
	if err != nil {
		return err
	}
	configParams := &machinesetup.ConfigParams{
		Roles:    machine.Spec.Roles,
		Versions: machine.Spec.Versions,
	}
	startupScript, err := machineSetupConfig.GetSetupScript(configParams)
	if err != nil {
		return err
	}
	if util.IsMaster(machine) {
		startupScript, err = masterStartupScript(cluster, machine, startupScript)
		if err != nil {
			return oc.handleMachineError(machine, apierrors.CreateMachine(
				"error creating Openstack instance: %v", err))
		}
	} else {
		token, err := oc.getKubeadmToken()
		if err != nil {
			return oc.handleMachineError(machine, apierrors.CreateMachine(
				"error creating Openstack instance: %v", err))
		}
		startupScript, err = nodeStartupScript(cluster, machine, token, startupScript)
		if err != nil {
			return oc.handleMachineError(machine, apierrors.CreateMachine(
				"error creating Openstack instance: %v", err))
		}
	}

	instance, err = oc.machineService.InstanceCreate(providerConfig, startupScript, oc.sshCred.user)
	if err != nil {
		return oc.handleMachineError(machine, apierrors.CreateMachine(
			"error creating Openstack instance: %v", err))
	}
	// TODO: wait instance ready
	err = util.PollImmediate(RetryIntervalInstanceStatus, TimeoutInstanceCreate, func() (bool, error) {
		instance, err := oc.machineService.GetInstance(instance.ID)
		if err != nil {
			return false, nil
		}
		return instance.Status == "ACTIVE", nil
	})
	if err != nil {
		return oc.handleMachineError(machine, apierrors.CreateMachine(
			"error creating Openstack instance: %v", err))
	}

	if providerConfig.FloatingIP != "" {
		err := oc.machineService.AssociateFloatingIP(instance.ID, providerConfig.FloatingIP)
		return oc.handleMachineError(machine, apierrors.CreateMachine(
			"Associate floatingIP err: %v", err))
	}

	return oc.updateAnnotation(machine, instance.ID)
}

func (oc *OpenstackClient) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	instance, err := oc.instanceExists(machine)
	if err != nil {
		return err
	}

	if instance == nil {
		glog.Infof("Skipped deleting a VM that is already deleted.\n")
		return nil
	}

	id := machine.ObjectMeta.Annotations[OpenstackIdAnnotationKey]
	err = oc.machineService.InstanceDelete(id)
	if err != nil {
		return oc.handleMachineError(machine, apierrors.DeleteMachine(
			"error deleting Openstack instance: %v", err))
	}

	return nil
}

func (oc *OpenstackClient) Update(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	status, err := oc.instanceStatus(machine)
	if err != nil {
		return err
	}

	currentMachine := (*clusterv1.Machine)(status)
	if currentMachine == nil {
		instance, err := oc.instanceExists(machine)
		if err != nil {
			return err
		}
		if instance != nil && instance.Status == "ACTIVE" {
			glog.Infof("Populating current state for boostrap machine %v", machine.ObjectMeta.Name)
			return oc.updateAnnotation(machine, instance.ID)
		} else {
			return fmt.Errorf("Cannot retrieve current state to update machine %v", machine.ObjectMeta.Name)
		}
	}

	if !oc.requiresUpdate(currentMachine, machine) {
		return nil
	}

	if util.IsMaster(currentMachine) {
		// TODO: add master inplace
		glog.Errorf("master inplace update failed: %v", err)
	} else {
		glog.Infof("re-creating machine %s for update.", currentMachine.ObjectMeta.Name)
		err = oc.Delete(cluster, currentMachine)
		if err != nil {
			glog.Errorf("delete machine %s for update failed: %v", currentMachine.ObjectMeta.Name, err)
		} else {
			err = oc.Create(cluster, machine)
			if err != nil {
				glog.Errorf("create machine %s for update failed: %v", machine.ObjectMeta.Name, err)
			}
		}
	}

	return nil
}

func (oc *OpenstackClient) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	instance, err := oc.instanceExists(machine)
	if err != nil {
		return false, err
	}
	return instance != nil, err
}

func getIPFromInstance(instance *clients.Instance) (string, error) {
	if instance.AccessIPv4 != "" && net.ParseIP(instance.AccessIPv4) != nil {
		return instance.AccessIPv4, nil
	}
	type network struct {
		Addr    string  `json:"addr"`
		Version float64 `json:"version"`
		Type    string  `json:"OS-EXT-IPS:type"`
	}

	for _, b := range instance.Addresses {
		list, err := json.Marshal(b)
		if err != nil {
			return "", fmt.Errorf("extract IP from instance err: %v", err)
		}
		var address []interface{}
		json.Unmarshal(list, &address)
		var addrList []string
		for _, addr := range address {
			var net network
			b, _ := json.Marshal(addr)
			json.Unmarshal(b, &net)
			if net.Version == 4.0 {
				if net.Type == "floating" {
					return net.Addr, nil
				}
				addrList = append(addrList, net.Addr)
			}
			if len(addrList) != 0 {
				return addrList[0], nil
			}
		}
	}
	return "", fmt.Errorf("extract IP from instance err")
}

func (oc *OpenstackClient) GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error) {
	if oc.sshCred == nil {
		return "", fmt.Errorf("Get kubeConfig failed, don't have ssh keypair information")
	}
	ip, err := oc.GetIP(cluster, master)
	if err != nil {
		return "", err
	}

	result := strings.TrimSpace(util.ExecCommand(
		"ssh", "-i", oc.sshCred.privateKeyPath,
		"-o", "StrictHostKeyChecking no",
		"-o", "UserKnownHostsFile /dev/null",
		fmt.Sprintf("cc@%s", ip),
		"echo STARTFILE; sudo cat /etc/kubernetes/admin.conf"))
	parts := strings.Split(result, "STARTFILE")
	if len(parts) != 2 {
		return "", nil
	}
	return strings.TrimSpace(parts[1]), nil
}

// If the OpenstackClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (oc *OpenstackClient) handleMachineError(machine *clusterv1.Machine, err *apierrors.MachineError) error {
	if oc.machineClient != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		oc.machineClient.UpdateStatus(machine)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}

func (oc *OpenstackClient) updateAnnotation(machine *clusterv1.Machine, id string) error {
	if machine.ObjectMeta.Annotations == nil {
		machine.ObjectMeta.Annotations = make(map[string]string)
	}
	machine.ObjectMeta.Annotations[OpenstackIdAnnotationKey] = id
	instance, _ := oc.instanceExists(machine)
	ip, err := getIPFromInstance(instance)
	if err != nil {
		return err
	}
	machine.ObjectMeta.Annotations[OpenstackIPAnnotationKey] = ip
	_, err = oc.machineClient.Update(machine)
	if err != nil {
		return err
	}
	return oc.updateInstanceStatus(machine)
}

func (oc *OpenstackClient) requiresUpdate(a *clusterv1.Machine, b *clusterv1.Machine) bool {
	if a == nil || b == nil {
		return true
	}
	// Do not want status changes. Do want changes that impact machine provisioning
	return !reflect.DeepEqual(a.Spec.ObjectMeta, b.Spec.ObjectMeta) ||
		!reflect.DeepEqual(a.Spec.ProviderConfig, b.Spec.ProviderConfig) ||
		!reflect.DeepEqual(a.Spec.Roles, b.Spec.Roles) ||
		!reflect.DeepEqual(a.Spec.Versions, b.Spec.Versions) ||
		a.ObjectMeta.Name != b.ObjectMeta.Name
}

func (oc *OpenstackClient) instanceExists(machine *clusterv1.Machine) (instance *clients.Instance, err error) {
	machineConfig, err := oc.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return nil, err
	}
	opts := &clients.InstanceListOpts{
		Name:   machineConfig.Name,
		Image:  machineConfig.Image,
		Flavor: machineConfig.Flavor,
	}
	instanceList, err := oc.machineService.GetInstanceList(opts)
	if err != nil {
		return nil, err
	}
	if len(instanceList) == 0 {
		return nil, nil
	}
	return instanceList[0], nil
}

// providerconfig get openstack provider config
func (oc *OpenstackClient) providerconfig(providerConfig clusterv1.ProviderConfig) (*openstackconfigv1.OpenstackProviderConfig, error) {
	obj, gvk, err := oc.codecFactory.UniversalDecoder(openstackconfigv1.SchemeGroupVersion).Decode(providerConfig.Value.Raw, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decoding failure: %v", err)
	}
	config, ok := obj.(*openstackconfigv1.OpenstackProviderConfig)
	if !ok {
		return nil, fmt.Errorf("failure to cast to openstack; type: %v", gvk)
	}

	return config, nil
}

func (oc *OpenstackClient) getKubeadmToken() (string, error) {
	tokenParams := kubeadm.TokenCreateParams{
		Ttl: time.Duration(10) * time.Minute,
	}
	output, err := oc.kubeadm.TokenCreate(tokenParams)
	if err != nil {
		glog.Errorf("unable to create token: %v", err)
		return "", err
	}
	return strings.TrimSpace(output), err
}

func (oc *OpenstackClient) validateMachine(machine *clusterv1.Machine, config *openstackconfigv1.OpenstackProviderConfig) *apierrors.MachineError {
	if machine.Spec.Versions.Kubelet == "" {
		return apierrors.InvalidMachineConfiguration("spec.versions.kubelet can't be empty")
	}
	// TODO: other validate of openstackCloud
	return nil
}
