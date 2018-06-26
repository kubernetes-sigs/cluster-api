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

package vsphere

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/tools/record"

	"encoding/base64"
	"sigs.k8s.io/cluster-api/cloud/vsphere/namedmachines"
	vsphereconfig "sigs.k8s.io/cluster-api/cloud/vsphere/vsphereproviderconfig"
	vsphereconfigv1 "sigs.k8s.io/cluster-api/cloud/vsphere/vsphereproviderconfig/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
	"sigs.k8s.io/cluster-api/pkg/kubeadm"
	"sigs.k8s.io/cluster-api/pkg/util"
	"time"
)

const (
	VmIpAnnotationKey                = "vm-ip-address"
	ControlPlaneVersionAnnotationKey = "control-plane-version"
	KubeletVersionAnnotationKey      = "kubelet-version"

	// Filename in which named machines are saved using a ConfigMap (in master).
	NamedMachinesFilename = "vsphere_named_machines.yaml"

	// The contents of the tfstate file for a machine.
	StatusMachineTerraformState = "tf-state"

	createEventAction = "Create"
	deleteEventAction = "Delete"
)

const (
	StageDir               = "/tmp/cluster-api/machines"
	MachinePathStageFormat = "/tmp/cluster-api/machines/%s/"
	TfConfigFilename       = "terraform.tf"
	TfVarsFilename         = "variables.tfvars"
	TfStateFilename        = "terraform.tfstate"
	startupScriptFilename  = "machine-startup.sh"
	KubeadmTokenTtl        = time.Duration(10) * time.Minute
)

type VsphereClient struct {
	scheme            *runtime.Scheme
	codecFactory      *serializer.CodecFactory
	machineClient     client.MachineInterface
	namedMachineWatch *namedmachines.ConfigWatch
	eventRecorder     record.EventRecorder
	// Once the vsphere-deployer is deleted, both DeploymentClient and VsphereClient can depend on
	// something that implements GetIP instead of the VsphereClient depending on DeploymentClient.
	*DeploymentClient
}

func NewMachineActuator(machineClient client.MachineInterface, eventRecorder record.EventRecorder, namedMachinePath string) (*VsphereClient, error) {
	scheme, codecFactory, err := vsphereconfigv1.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}

	// DEPRECATED: Remove when vsphere-deployer is deleted.
	var nmWatch *namedmachines.ConfigWatch
	nmWatch, err = namedmachines.NewConfigWatch(namedMachinePath)
	if err != nil {
		glog.Errorf("error creating named machine config watch: %+v", err)
	}
	return &VsphereClient{
		scheme:            scheme,
		codecFactory:      codecFactory,
		machineClient:     machineClient,
		namedMachineWatch: nmWatch,
		eventRecorder:     eventRecorder,
		DeploymentClient:  NewDeploymentClient(),
	}, nil
}

func saveFile(contents, path string, perm os.FileMode) error {
	return ioutil.WriteFile(path, []byte(contents), perm)
}

// Stage the machine for running terraform.
// Return: machine's staging dir path, error
func (vc *VsphereClient) prepareStageMachineDir(machine *clusterv1.Machine, eventAction string) (string, error) {
	err := vc.cleanUpStagingDir(machine)
	if err != nil {
		return "", err
	}

	machineName := machine.ObjectMeta.Name
	config, err := vc.machineproviderconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return "", vc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err), eventAction)
	}

	machinePath := fmt.Sprintf(MachinePathStageFormat, machineName)
	if _, err := os.Stat(machinePath); os.IsNotExist(err) {
		os.MkdirAll(machinePath, 0755)
	}

	// Save the config file and variables file to the machinePath
	tfConfigPath := path.Join(machinePath, TfConfigFilename)
	tfVarsPath := path.Join(machinePath, TfVarsFilename)

	namedMachines, err := vc.namedMachineWatch.NamedMachines()
	if err != nil {
		return "", err
	}
	matchedMachine, err := namedMachines.MatchMachine(config.VsphereMachine)
	if err != nil {
		return "", err
	}
	if err := saveFile(matchedMachine.MachineHcl, tfConfigPath, 0644); err != nil {
		return "", err
	}

	var tfVarsContents []string
	for key, value := range config.MachineVariables {
		tfVarsContents = append(tfVarsContents, fmt.Sprintf("%s=\"%s\"", key, value))
	}
	if err := saveFile(strings.Join(tfVarsContents, "\n"), tfVarsPath, 0644); err != nil {
		return "", err
	}

	// Save the tfstate file (if not bootstrapping).
	_, err = vc.stageTfState(machine)
	if err != nil {
		return "", err
	}

	return machinePath, nil
}

// Returns the path to the tfstate file staged from the tf state in annotations.
func (vc *VsphereClient) stageTfState(machine *clusterv1.Machine) (string, error) {
	machinePath := fmt.Sprintf(MachinePathStageFormat, machine.ObjectMeta.Name)
	tfStateFilePath := path.Join(machinePath, TfStateFilename)

	if _, err := os.Stat(machinePath); os.IsNotExist(err) {
		os.MkdirAll(machinePath, 0755)
	}

	// Attempt to stage the file from annotations.
	glog.Infof("Attempting to stage tf state for machine %s", machine.ObjectMeta.Name)
	if machine.ObjectMeta.Annotations == nil {
		glog.Infof("machine does not have annotations, state does not exist")
		return "", nil
	}

	if _, ok := machine.ObjectMeta.Annotations[StatusMachineTerraformState]; !ok {
		glog.Info("machine does not have annotation for tf state.")
		return "", nil
	}

	tfStateB64, _ := machine.ObjectMeta.Annotations[StatusMachineTerraformState]
	tfState, err := base64.StdEncoding.DecodeString(tfStateB64)
	if err != nil {
		glog.Errorf("error decoding tfstate while staging. %+v", err)
		return "", err
	}
	if err := saveFile(string(tfState), tfStateFilePath, 0644); err != nil {
		return "", err
	}

	return tfStateFilePath, nil
}

// Cleans up the staging directory.
func (vc *VsphereClient) cleanUpStagingDir(machine *clusterv1.Machine) error {
	glog.Infof("Cleaning up the staging dir for machine %s", machine.ObjectMeta.Name)
	return os.RemoveAll(fmt.Sprintf(MachinePathStageFormat, machine.ObjectMeta.Name))
}

// Builds and saves the startup script for the passed machine and cluster.
// Returns the full path of the saved startup script and possible error.
func (vc *VsphereClient) saveStartupScript(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	preloaded := false
	var startupScript string

	if util.IsMaster(machine) {
		if machine.Spec.Versions.ControlPlane == "" {
			return "", vc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
				"invalid master configuration: missing Machine.Spec.Versions.ControlPlane"), createEventAction)
		}
		var err error
		startupScript, err = getMasterStartupScript(
			templateParams{
				Cluster:   cluster,
				Machine:   machine,
				Preloaded: preloaded,
			},
		)
		if err != nil {
			return "", err
		}
	} else {
		if len(cluster.Status.APIEndpoints) == 0 {
			return "", errors.New("invalid cluster state: cannot create a Kubernetes node without an API endpoint")
		}
		kubeadmToken, err := getKubeadmToken()
		if err != nil {
			return "", err
		}
		startupScript, err = getNodeStartupScript(
			templateParams{
				Token:     kubeadmToken,
				Cluster:   cluster,
				Machine:   machine,
				Preloaded: preloaded,
			},
		)
		if err != nil {
			return "", err
		}
	}

	glog.Infof("Saving startup script for machine %s", machine.ObjectMeta.Name)
	// Save the startup script.
	machinePath := fmt.Sprintf(MachinePathStageFormat, machine.Name)
	startupScriptPath := path.Join(machinePath, startupScriptFilename)
	if err := saveFile(startupScript, startupScriptPath, 0644); err != nil {
		return "", errors.New(fmt.Sprintf("Could not write startup script %s", err))
	}

	return startupScriptPath, nil
}

func (vc *VsphereClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	config, err := vc.machineproviderconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return vc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err), createEventAction)
	}

	clusterConfig, err := vc.clusterproviderconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return err
	}

	if verr := vc.validateMachine(machine, config); verr != nil {
		return vc.handleMachineError(machine, verr, createEventAction)
	}

	if verr := vc.validateCluster(cluster); verr != nil {
		return verr
	}

	machinePath, err := vc.prepareStageMachineDir(machine, createEventAction)
	if err != nil {
		return errors.New(fmt.Sprintf("error while staging machine: %+v", err))
	}

	glog.Infof("Staged for machine create at %s", machinePath)

	// Save the startup script.
	startupScriptPath, err := vc.saveStartupScript(cluster, machine)
	if err != nil {
		return errors.New(fmt.Sprintf("could not write startup script %+v", err))
	}
	defer cleanUpStartupScript(machine.Name, startupScriptPath)

	glog.Infof("Checking if machine %s exists", machine.ObjectMeta.Name)
	instance, err := vc.instanceIfExists(machine)
	if err != nil {
		return err
	}

	if instance == nil {
		glog.Infof("Machine instance does not exist. will create %s", machine.ObjectMeta.Name)

		var args []string
		args = append(args, "apply")
		args = append(args, "-auto-approve")
		args = append(args, "-input=false")
		args = append(args, "-var")
		args = append(args, fmt.Sprintf("vm_name=%s", machine.ObjectMeta.Name))
		args = append(args, "-var")
		args = append(args, fmt.Sprintf("vsphere_user=%s", clusterConfig.VsphereUser))
		args = append(args, "-var")
		args = append(args, fmt.Sprintf("vsphere_password=%s", clusterConfig.VspherePassword))
		args = append(args, "-var")
		args = append(args, fmt.Sprintf("vsphere_server=%s", clusterConfig.VsphereServer))
		args = append(args, fmt.Sprintf("-var-file=%s", path.Join(machinePath, TfVarsFilename)))
		args = append(args, "-var")
		args = append(args, fmt.Sprintf("startup_script_path=%s", startupScriptPath))

		_, cmdErr := runTerraformCmd(false, machinePath, args...)
		if cmdErr != nil {
			return errors.New(fmt.Sprintf("could not run terraform to create machine: %s", cmdErr))
		}

		// Get the IP address
		out, cmdErr := runTerraformCmd(false, machinePath, "output", "ip_address")
		if cmdErr != nil {
			return fmt.Errorf("could not obtain 'ip_address' output variable: %s", cmdErr)
		}
		vmIp := strings.TrimSpace(out.String())
		glog.Infof("Machine %s created with ip address %s", machine.ObjectMeta.Name, vmIp)

		// Annotate the machine so that we remember exactly what VM we created for it.
		tfState, _ := vc.GetTfState(machine)
		vc.cleanUpStagingDir(machine)
		vc.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Created", "Created Machine %v", machine.Name)
		return vc.updateAnnotations(machine, vmIp, tfState)
	} else {
		glog.Infof("Skipped creating a VM for machine %s that already exists.", machine.ObjectMeta.Name)
	}

	return nil
}

// Assumes the staging dir (workingDir) is set up correctly. That means have the .tf, .tfstate and .tfvars
// there as needed.
// Set stdout=true to redirect process's standard out to sys stdout.
// Otherwise returns a byte buffer of stdout.
func runTerraformCmd(stdout bool, workingDir string, arg ...string) (bytes.Buffer, error) {
	var out bytes.Buffer

	terraformPath, err := exec.LookPath("terraform")
	if err != nil {
		return bytes.Buffer{}, errors.New("terraform binary not found")
	}
	glog.Infof("terraform path: %s", terraformPath)

	// Check if we need to terraform init
	tfInitExists, e := pathExists(path.Join(workingDir, ".terraform/"))
	if e != nil {
		return bytes.Buffer{}, errors.New(fmt.Sprintf("Could not get the path of .terraform for machine: %+v", e))
	}
	if !tfInitExists {
		glog.Infof("Terraform not initialized. Running terraform init.")
		cmd := exec.Command(terraformPath, "init")
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Dir = workingDir

		initErr := cmd.Run()
		if initErr != nil {
			return bytes.Buffer{}, errors.New(fmt.Sprintf("Could not run terraform: %+v", initErr))
		}
	}

	cmd := exec.Command(terraformPath, arg...)
	// If stdout, only show in stdout
	if stdout {
		cmd.Stdout = os.Stdout
	} else {
		// Otherwise, save to buffer, and to a local log file.
		logFileName := fmt.Sprintf("/tmp/cluster-api-%s.log", util.RandomToken())
		f, _ := os.Create(logFileName)
		glog.Infof("Executing terraform. Logs are saved in %s", logFileName)
		multiWriter := io.MultiWriter(&out, f)
		cmd.Stdout = multiWriter
	}
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Dir = workingDir
	err = cmd.Run()
	if err != nil {
		return out, err
	}

	return out, nil
}

func (vc *VsphereClient) Delete(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	// Check if the instance exists, return if it doesn't
	instance, err := vc.instanceIfExists(machine)
	if err != nil {
		return err
	}
	if instance == nil {
		return nil
	}

	clusterConfig, err := vc.clusterproviderconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return err
	}

	machinePath, err := vc.prepareStageMachineDir(machine, deleteEventAction)

	// destroy it
	args := []string{
		"destroy",
		"-auto-approve",
		"-input=false",
		"-var", fmt.Sprintf("vm_name=%s", machine.ObjectMeta.Name),
		"-var", fmt.Sprintf("vsphere_user=%s", clusterConfig.VsphereUser),
		"-var", fmt.Sprintf("vsphere_password=%s", clusterConfig.VspherePassword),
		"-var", fmt.Sprintf("vsphere_server=%s", clusterConfig.VsphereServer),
		"-var", "startup_script_path=/dev/null",
		"-var-file=variables.tfvars",
	}
	_, err = runTerraformCmd(false, machinePath, args...)
	if err != nil {
		return fmt.Errorf("could not run terraform: %s", err)
	}

	vc.cleanUpStagingDir(machine)

	// Update annotation for the state.
	machine.ObjectMeta.Annotations[StatusMachineTerraformState] = ""
	_, err = vc.machineClient.Update(machine)

	if err == nil {
		vc.eventRecorder.Eventf(machine, corev1.EventTypeNormal, "Killing", "Killing machine %v", machine.Name)
	}

	return err
}

func (vc *VsphereClient) PostDelete(cluster *clusterv1.Cluster) error {
	return nil
}

func (vc *VsphereClient) Update(cluster *clusterv1.Cluster, goalMachine *clusterv1.Machine) error {
	// Check if the annotations we want to track exist, if not, the user likely created a master machine with their own annotation.
	if _, ok := goalMachine.ObjectMeta.Annotations[ControlPlaneVersionAnnotationKey]; !ok {
		ip, _ := vc.DeploymentClient.GetIP(goalMachine)
		glog.Info("Version annotations do not exist. Populating existing state for bootstrapped machine.")
		tfState, _ := vc.GetTfState(goalMachine)
		return vc.updateAnnotations(goalMachine, ip, tfState)
	}

	if util.IsMaster(goalMachine) {
		// Master upgrades
		glog.Info("Upgrade for master machine.. Check if upgrade needed.")

		// If the saved versions and new versions differ, do in-place upgrade.
		if vc.needsMasterUpdate(goalMachine) {
			glog.Infof("Doing in-place upgrade for master from v%s to v%s", goalMachine.Annotations[ControlPlaneVersionAnnotationKey], goalMachine.Spec.Versions.ControlPlane)
			err := vc.updateMasterInPlace(goalMachine)
			if err != nil {
				glog.Errorf("Master in-place upgrade failed: %+v", err)
				return err
			}
		} else {
			glog.Info("UNSUPPORTED MASTER UPDATE.")
		}
	} else {
		if vc.needsNodeUpdate(goalMachine) {
			// Node upgrades
			if err := vc.updateNode(cluster, goalMachine); err != nil {
				glog.Errorf("Node %s update failed: %+v", goalMachine.ObjectMeta.Name, err)
				return err
			}
		} else {
			glog.Info("UNSUPPORTED NODE UPDATE.")
		}
	}

	return nil
}

func (vc *VsphereClient) needsControlPlaneUpdate(machine *clusterv1.Machine) bool {
	return machine.Spec.Versions.ControlPlane != machine.Annotations[ControlPlaneVersionAnnotationKey]
}

func (vc *VsphereClient) needsKubeletUpdate(machine *clusterv1.Machine) bool {
	return machine.Spec.Versions.Kubelet != machine.Annotations[KubeletVersionAnnotationKey]
}

// Returns true if the node is needed to be upgraded.
func (vc *VsphereClient) needsNodeUpdate(machine *clusterv1.Machine) bool {
	return !util.IsMaster(machine) &&
		vc.needsKubeletUpdate(machine)
}

// Returns true if the master is needed to be upgraded.
func (vc *VsphereClient) needsMasterUpdate(machine *clusterv1.Machine) bool {
	return util.IsMaster(machine) &&
		vc.needsControlPlaneUpdate(machine)
	// TODO: we should support kubelet upgrades here as well.
}

func (vc *VsphereClient) updateKubelet(machine *clusterv1.Machine) error {
	if vc.needsKubeletUpdate(machine) {
		// Kubelet packages are versioned 1.10.1-00 and so on.
		kubeletAptVersion := machine.Spec.Versions.Kubelet + "-00"
		cmd := fmt.Sprintf("sudo apt-get install kubelet=%s", kubeletAptVersion)
		if _, err := vc.remoteSshCommand(machine, cmd, "~/.ssh/vsphere_tmp", "ubuntu"); err != nil {
			glog.Errorf("remoteSshCommand while installing new kubelet version: %v", err)
			return err
		}
	}
	return nil
}

func (vc *VsphereClient) updateControlPlane(machine *clusterv1.Machine) error {
	if vc.needsControlPlaneUpdate(machine) {
		// Pull the kudeadm for target version K8s.
		cmd := fmt.Sprintf("curl -sSL https://dl.k8s.io/release/v%s/bin/linux/amd64/kubeadm | sudo tee /usr/bin/kubeadm > /dev/null; "+
			"sudo chmod a+rx /usr/bin/kubeadm", machine.Spec.Versions.ControlPlane)
		if _, err := vc.remoteSshCommand(machine, cmd, "~/.ssh/vsphere_tmp", "ubuntu"); err != nil {
			glog.Infof("remoteSshCommand failed while downloading new kubeadm: %+v", err)
			return err
		}

		// Next upgrade control plane
		cmd = fmt.Sprintf("sudo kubeadm upgrade apply %s -y", "v"+machine.Spec.Versions.ControlPlane)
		if _, err := vc.remoteSshCommand(machine, cmd, "~/.ssh/vsphere_tmp", "ubuntu"); err != nil {
			glog.Infof("remoteSshCommand failed while upgrading control plane: %+v", err)
			return err
		}
	}
	return nil
}

// Update the passed node machine by recreating it.
func (vc *VsphereClient) updateNode(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	if err := vc.Delete(cluster, machine); err != nil {
		return err
	}

	if err := vc.Create(cluster, machine); err != nil {
		return err
	}
	return nil
}

// Assumes that update is needed.
// For now support only K8s control plane upgrades.
func (vc *VsphereClient) updateMasterInPlace(machine *clusterv1.Machine) error {
	// Execute a control plane upgrade.
	if err := vc.updateControlPlane(machine); err != nil {
		return err
	}

	// Update annotation for version.
	machine.ObjectMeta.Annotations[ControlPlaneVersionAnnotationKey] = machine.Spec.Versions.ControlPlane
	if _, err := vc.machineClient.Update(machine); err != nil {
		return err
	}

	return nil
}

func (vc *VsphereClient) remoteSshCommand(m *clusterv1.Machine, cmd, privateKeyPath, sshUser string) (string, error) {
	glog.Infof("Remote SSH execution '%s' on %s", cmd, m.ObjectMeta.Name)

	publicIP, err := vc.DeploymentClient.GetIP(m)
	if err != nil {
		return "", err
	}

	command := fmt.Sprintf("echo STARTFILE; %s", cmd)
	c := exec.Command("ssh", "-i", privateKeyPath, sshUser+"@"+publicIP,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		command)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	result := strings.TrimSpace(string(out))
	parts := strings.Split(result, "STARTFILE")
	glog.Infof("\t Result of command %s ========= %+v", cmd, parts)
	if len(parts) != 2 {
		return "", nil
	}
	// TODO: Check error.
	return strings.TrimSpace(parts[1]), nil
}

func (vc *VsphereClient) Exists(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	i, err := vc.instanceIfExists(machine)
	if err != nil {
		return false, err
	}
	return i != nil, err
}

func (vc *VsphereClient) GetTfState(machine *clusterv1.Machine) (string, error) {
	if machine.ObjectMeta.Annotations != nil {
		if tfStateB64, ok := machine.ObjectMeta.Annotations[StatusMachineTerraformState]; ok {
			glog.Infof("Returning tfstate for machine %s from annotation", machine.ObjectMeta.Name)
			tfState, err := base64.StdEncoding.DecodeString(tfStateB64)
			if err != nil {
				glog.Errorf("error decoding tfstate in annotation. %+v", err)
				return "", err
			}
			return string(tfState), nil
		}
	}

	tfStateBytes, _ := ioutil.ReadFile(path.Join(fmt.Sprintf(MachinePathStageFormat, machine.ObjectMeta.Name), TfStateFilename))
	if tfStateBytes != nil {
		glog.Infof("Returning tfstate for machine %s from staging file", machine.ObjectMeta.Name)
		return string(tfStateBytes), nil
	}

	return "", errors.New("could not get tfstate")
}

// We are storing these as annotations and not in Machine Status because that's intended for
// "Provider-specific status" that will usually be used to detect updates. Additionally,
// Status requires yet another version API resource which is too heavy to store IP and TF state.
func (vc *VsphereClient) updateAnnotations(machine *clusterv1.Machine, vmIp, tfState string) error {
	glog.Infof("Updating annotations for machine %s", machine.ObjectMeta.Name)
	if machine.ObjectMeta.Annotations == nil {
		machine.ObjectMeta.Annotations = make(map[string]string)
	}

	tfStateB64 := base64.StdEncoding.EncodeToString([]byte(tfState))

	machine.ObjectMeta.Annotations[VmIpAnnotationKey] = vmIp
	machine.ObjectMeta.Annotations[ControlPlaneVersionAnnotationKey] = machine.Spec.Versions.ControlPlane
	machine.ObjectMeta.Annotations[KubeletVersionAnnotationKey] = machine.Spec.Versions.Kubelet
	machine.ObjectMeta.Annotations[StatusMachineTerraformState] = tfStateB64

	_, err := vc.machineClient.Update(machine)
	if err != nil {
		return err
	}
	return nil
}

// Returns the machine object if the passed machine exists in terraform state.
func (vc *VsphereClient) instanceIfExists(machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	machinePath := fmt.Sprintf(MachinePathStageFormat, machine.ObjectMeta.Name)
	tfStateFilePath, err := vc.stageTfState(machine)
	if err != nil {
		return nil, err
	}
	glog.Infof("Instance existance checked in directory %+v", tfStateFilePath)
	if tfStateFilePath == "" {
		return nil, nil
	}

	out, tfCmdErr := runTerraformCmd(false, machinePath, "show")
	if tfCmdErr != nil {
		glog.Infof("Ignore terraform error in instanceIfExists: %+v", err)
		return nil, nil
	}
	re := regexp.MustCompile(fmt.Sprintf("\n[[:space:]]*(name = %s)\n", machine.ObjectMeta.Name))
	if re.MatchString(out.String()) {
		return machine, nil
	}
	return nil, nil
}

func (vc *VsphereClient) machineproviderconfig(providerConfig clusterv1.ProviderConfig) (*vsphereconfig.VsphereMachineProviderConfig, error) {
	obj, gvk, err := vc.codecFactory.UniversalDecoder().Decode(providerConfig.Value.Raw, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("machine providerconfig decoding failure: %v", err)
	}

	config, ok := obj.(*vsphereconfig.VsphereMachineProviderConfig)
	if !ok {
		return nil, fmt.Errorf("machine providerconfig failure to cast to vsphere; type: %v", gvk)
	}

	return config, nil
}

func (vc *VsphereClient) clusterproviderconfig(providerConfig clusterv1.ProviderConfig) (*vsphereconfig.VsphereClusterProviderConfig, error) {
	obj, gvk, err := vc.codecFactory.UniversalDecoder().Decode(providerConfig.Value.Raw, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("cluster providerconfig decoding failure: %v", err)
	}

	config, ok := obj.(*vsphereconfig.VsphereClusterProviderConfig)
	if !ok {
		return nil, fmt.Errorf("cluster providerconfig failure to cast to vsphere; type: %v", gvk)
	}

	return config, nil
}

func (vc *VsphereClient) validateMachine(machine *clusterv1.Machine, config *vsphereconfig.VsphereMachineProviderConfig) *apierrors.MachineError {
	if machine.Spec.Versions.Kubelet == "" {
		return apierrors.InvalidMachineConfiguration("spec.versions.kubelet can't be empty")
	}
	return nil
}

func (vc *VsphereClient) validateCluster(cluster *clusterv1.Cluster) error {
	if cluster.Spec.ClusterNetwork.ServiceDomain == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.ServiceDomain")
	}
	if getSubnet(cluster.Spec.ClusterNetwork.Pods) == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.Pods")
	}
	if getSubnet(cluster.Spec.ClusterNetwork.Services) == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.Services")
	}

	clusterConfig, err := vc.clusterproviderconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return err
	}
	if clusterConfig.VsphereUser == "" || clusterConfig.VspherePassword == "" || clusterConfig.VsphereServer == "" {
		return errors.New("vsphere_user, vsphere_password, vsphere_server are required fields in Cluster spec.")
	}
	return nil
}

func getKubeadmToken() (string, error) {
	tokenParams := kubeadm.TokenCreateParams{
		Ttl: KubeadmTokenTtl,
	}
	output, err := kubeadm.New().TokenCreate(tokenParams)
	if err != nil {
		return "", fmt.Errorf("unable to create kubeadm token: %v", err)
	}
	return strings.TrimSpace(output), err
}

// If the VsphereClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (vc *VsphereClient) handleMachineError(machine *clusterv1.Machine, err *apierrors.MachineError, eventAction string) error {
	if vc.machineClient != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		vc.machineClient.UpdateStatus(machine)
	}
	if eventAction != "" {
		vc.eventRecorder.Eventf(machine, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err.Reason)
	}

	glog.Errorf("Machine error: %v", err.Message)
	return err
}

// Just a temporary hack to grab a single range from the config.
func getSubnet(netRange clusterv1.NetworkRanges) string {
	if len(netRange.CIDRBlocks) == 0 {
		return ""
	}
	return netRange.CIDRBlocks[0]
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func cleanUpStartupScript(machineName, fullPath string) {
	glog.Infof("cleaning up startup script for %v: %v", machineName, fullPath)
	if err := os.RemoveAll(path.Dir(fullPath)); err != nil {
		glog.Error(err)
	}
}
