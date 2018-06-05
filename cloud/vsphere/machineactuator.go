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
	"os/user"
	"path"
	"regexp"
	"strings"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"encoding/base64"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/cloud/vsphere/namedmachines"
	vsphereconfig "sigs.k8s.io/cluster-api/cloud/vsphere/vsphereproviderconfig"
	vsphereconfigv1 "sigs.k8s.io/cluster-api/cloud/vsphere/vsphereproviderconfig/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
)

const (
	MasterIpAnnotationKey            = "master-ip"
	ControlPlaneVersionAnnotationKey = "control-plane-version"
	KubeletVersionAnnotationKey      = "kubelet-version"

	// Filename in which named machines are saved using a ConfigMap (in master).
	NamedMachinesFilename = "vsphere_named_machines.yaml"

	// The contents of the tfstate file for a machine.
	StatusMachineTerraformState = "tf-state"
)

const (
	StageDir               = "/tmp/cluster-api/machines"
	MachinePathStageFormat = "/tmp/cluster-api/machines/%s/"
	TfConfigFilename       = "terraform.tf"
	TfVarsFilename         = "variables.tfvars"
	TfStateFilename        = "terraform.tfstate"
	MasterIpFilename       = "master-ip"
)

type VsphereClient struct {
	scheme            *runtime.Scheme
	codecFactory      *serializer.CodecFactory
	kubeadmToken      string
	machineClient     client.MachineInterface
	namedMachineWatch *namedmachines.ConfigWatch
}

func NewMachineActuator(kubeadmToken string, machineClient client.MachineInterface, namedMachinePath string) (*VsphereClient, error) {
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
		kubeadmToken:      kubeadmToken,
		machineClient:     machineClient,
		namedMachineWatch: nmWatch,
	}, nil
}

// DEPRECATED: Remove when vsphere-deployer is deleted.
func (vc *VsphereClient) CreateMachineController(cluster *clusterv1.Cluster, initialMachines []*clusterv1.Machine, clientSet kubernetes.Clientset) error {
	if err := CreateExtApiServerRoleBinding(); err != nil {
		return err
	}

	// Create the named machines ConfigMap.
	// After pivot-based bootstrap is done, the named machine should be a ConfigMap and this logic will be removed.
	namedMachines, err := vc.namedMachineWatch.NamedMachines()

	if err != nil {
		return err
	}
	yaml, err := namedMachines.GetYaml()
	if err != nil {
		return err
	}
	nmConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "named-machines"},
		Data: map[string]string{
			NamedMachinesFilename: yaml,
		},
	}
	configMaps := clientSet.CoreV1().ConfigMaps(corev1.NamespaceDefault)
	if _, err := configMaps.Create(&nmConfigMap); err != nil {
		return err
	}

	if err := CreateApiServerAndController(vc.kubeadmToken); err != nil {
		return err
	}
	return nil
}

func saveFile(contents, path string, perm os.FileMode) error {
	return ioutil.WriteFile(path, []byte(contents), perm)
}

// Currently only works for unix systems.
// TODO: Consider using https://github.com/mitchellh/go-homedir.
func getHomeDir() (string, error) {
	// Prefer $HOME.
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// Fallback to user's profile.
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return usr.HomeDir, nil
}

// Stage the machine for running terraform.
// Return: machine's staging dir path, error
func (vc *VsphereClient) prepareStageMachineDir(machine *clusterv1.Machine) (string, error) {
	err := vc.cleanUpStagingDir(machine)
	if err != nil {
		return "", err
	}

	machineName := machine.ObjectMeta.Name
	config, err := vc.machineproviderconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return "", vc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err))
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
	if err := saveFile(strings.Join(config.TerraformVariables, "\n"), tfVarsPath, 0644); err != nil {
		return "", err
	}

	// Save the tfstate file (if not bootstrapping).
	_, err = vc.stageTfState(machine)
	if err != nil {
		return "", err
	}

	return machinePath, nil
}

// Returns the path to the tfstate file.
func (vc *VsphereClient) stageTfState(machine *clusterv1.Machine) (string, error) {
	machinePath := fmt.Sprintf(MachinePathStageFormat, machine.ObjectMeta.Name)
	tfStateFilePath := path.Join(machinePath, TfStateFilename)

	// Ensure machinePath exists
	exists, err := pathExists(machinePath)
	if err != nil {
		return "", err
	}
	if !exists {
		err = os.Mkdir(machinePath, os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	// Check if the tfstate file already exists.
	exists, err = pathExists(tfStateFilePath)
	if err != nil {
		return "", err
	}
	if exists {
		return tfStateFilePath, nil
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
func (vc *VsphereClient) saveStartupScript(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	preloaded := false
	var startupScript string

	if util.IsMaster(machine) {
		if machine.Spec.Versions.ControlPlane == "" {
			return vc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
				"invalid master configuration: missing Machine.Spec.Versions.ControlPlane"))
		}
		var err error
		startupScript, err = getMasterStartupScript(
			templateParams{
				Token:     vc.kubeadmToken,
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
		startupScript, err = getNodeStartupScript(
			templateParams{
				Token:     vc.kubeadmToken,
				Cluster:   cluster,
				Machine:   machine,
				Preloaded: preloaded,
			},
		)
		if err != nil {
			return err
		}
	}

	// Save the startup script.
	if err := saveFile(startupScript, path.Join("/tmp", "machine-startup.sh"), 0644); err != nil {
		return errors.New(fmt.Sprintf("Could not write startup script %s", err))
	}

	return nil
}

func (vc *VsphereClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	config, err := vc.machineproviderconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return vc.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err))
	}

	clusterConfig, err := vc.clusterproviderconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return err
	}

	if verr := vc.validateMachine(machine, config); verr != nil {
		return vc.handleMachineError(machine, verr)
	}

	if verr := vc.validateCluster(cluster); verr != nil {
		return verr
	}

	machinePath, err := vc.prepareStageMachineDir(machine)
	if err != nil {
		return errors.New(fmt.Sprintf("error while staging machine: %+v", err))
	}

	// Save the startup script.
	err = vc.saveStartupScript(cluster, machine)
	if err != nil {
		return errors.New(fmt.Sprintf("could not write startup script %+v", err))
	}

	instance, err := vc.instanceIfExists(machine)
	if err != nil {
		return err
	}

	if instance == nil {
		glog.Infof("machine instance does not exist. will create %s", machine.ObjectMeta.Name)

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

		_, cmdErr := runTerraformCmd(false, machinePath, args...)
		if cmdErr != nil {
			return errors.New(fmt.Sprintf("could not run terraform to create machine: %s", cmdErr))
		}

		// Get the IP address
		out, cmdErr := runTerraformCmd(false, machinePath, "output", "ip_address")
		if cmdErr != nil {
			return fmt.Errorf("could not obtain 'ip_address' output variable: %s", cmdErr)
		}
		masterEndpointIp := strings.TrimSpace(out.String())
		glog.Infof("Master created with ip address %s", masterEndpointIp)

		// Annotate the machine so that we remember exactly what VM we created for it.
		tfState, _ := vc.GetTfState(machine)
		vc.cleanUpStagingDir(machine)
		return vc.updateAnnotations(machine, masterEndpointIp, tfState)
	} else {
		glog.Infof("Skipped creating a VM that already exists.\n")
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

func (vc *VsphereClient) Delete(_ *clusterv1.Cluster, machine *clusterv1.Machine) error {
	// Check if the instance exists, return if it doesn't
	instance, err := vc.instanceIfExists(machine)
	if err != nil {
		return err
	}

	if instance == nil {
		return nil
	}

	homedir, err := getHomeDir()
	if err != nil {
		return err
	}
	machinePath := path.Join(homedir, fmt.Sprintf(".terraform.d/kluster/machines/%s", machine.ObjectMeta.Name))

	// destroy it
	args := []string{
		"destroy",
		"-auto-approve",
		"-input=false",
		"-var", fmt.Sprintf("vm_name=%s", machine.ObjectMeta.Name),
		"-var-file=variables.tfvars",
	}
	_, err = runTerraformCmd(false, machinePath, args...)
	if err != nil {
		return fmt.Errorf("could not run terraform: %s", err)
	}

	// remove finalizer
	if vc.machineClient != nil {
		machine.ObjectMeta.Finalizers = util.Filter(machine.ObjectMeta.Finalizers, clusterv1.MachineFinalizer)
		_, err = vc.machineClient.Update(machine)
	}

	return err
}

func (vc *VsphereClient) PostDelete(cluster *clusterv1.Cluster, machines []*clusterv1.Machine) error {
	return nil
}

func (vc *VsphereClient) Update(cluster *clusterv1.Cluster, goalMachine *clusterv1.Machine) error {
	// Try to get the annotations for the versions. If they don't exist, update the annotation and return.
	// This can only happen right after bootstrapping.
	if goalMachine.ObjectMeta.Annotations == nil {
		ip, _ := vc.GetIP(goalMachine)
		glog.Info("Annotations do not exist. This happens for a newly bootstrapped machine.")
		tfState, _ := vc.GetTfState(goalMachine)
		return vc.updateAnnotations(goalMachine, ip, tfState)
	}

	// Check if the annotations we want to track exist, if not, the user likely created a master machine with their own annotation.
	if _, ok := goalMachine.ObjectMeta.Annotations[ControlPlaneVersionAnnotationKey]; !ok {
		ip, _ := vc.GetIP(goalMachine)
		glog.Info("Version annotations do not exist. Populating existing state for bootstrapped machine.")
		tfState, _ := vc.GetTfState(goalMachine)
		return vc.updateAnnotations(goalMachine, ip, tfState)
	}

	if util.IsMaster(goalMachine) {
		glog.Info("Upgrade for master machine.. Check if upgrade needed.")
		// Get the versions for the new object.
		goalMachineControlPlaneVersion := goalMachine.Spec.Versions.ControlPlane
		goalMachineKubeletVersion := goalMachine.Spec.Versions.Kubelet

		// If the saved versions and new versions differ, do in-place upgrade.
		if goalMachineControlPlaneVersion != goalMachine.Annotations[ControlPlaneVersionAnnotationKey] ||
			goalMachineKubeletVersion != goalMachine.Annotations[KubeletVersionAnnotationKey] {
			glog.Infof("Doing in-place upgrade for master from %s to %s", goalMachine.Annotations[ControlPlaneVersionAnnotationKey], goalMachineControlPlaneVersion)
			err := vc.updateMasterInPlace(goalMachine)
			if err != nil {
				glog.Errorf("Master inplace update failed: %+v", err)
			}
		} else {
			glog.Info("UPDATE TYPE NOT SUPPORTED")
			return nil
		}
	} else {
		glog.Info("NODE UPDATES NOT SUPPORTED")
		return nil
	}

	return nil
}

// Assumes that update is needed.
// For now support only K8s control plane upgrades.
func (vc *VsphereClient) updateMasterInPlace(goalMachine *clusterv1.Machine) error {
	goalMachineControlPlaneVersion := goalMachine.Spec.Versions.ControlPlane
	goalMachineKubeletVersion := goalMachine.Spec.Versions.Kubelet

	currentControlPlaneVersion := goalMachine.Annotations[ControlPlaneVersionAnnotationKey]
	currentKubeletVersion := goalMachine.Annotations[KubeletVersionAnnotationKey]

	// Control plane upgrade
	if goalMachineControlPlaneVersion != currentControlPlaneVersion {
		// Pull the kudeadm for target version K8s.
		cmd := fmt.Sprintf("curl -sSL https://dl.k8s.io/release/v%s/bin/linux/amd64/kubeadm | sudo tee /usr/bin/kubeadm > /dev/null; "+
			"sudo chmod a+rx /usr/bin/kubeadm", goalMachineControlPlaneVersion)
		_, err := vc.remoteSshCommand(goalMachine, cmd, "~/.ssh/id_rsa", "ubuntu")
		if err != nil {
			glog.Infof("remoteSshCommand failed while downloading new kubeadm: %+v", err)
			return err
		}

		// Next upgrade control plane
		cmd = fmt.Sprintf("sudo kubeadm upgrade apply %s -y", "v"+goalMachine.Spec.Versions.ControlPlane)
		_, err = vc.remoteSshCommand(goalMachine, cmd, "~/.ssh/id_rsa", "ubuntu")
		if err != nil {
			glog.Infof("remoteSshCommand failed while upgrading control plane: %+v", err)
			return err
		}
	}

	// Upgrade kubelet
	if goalMachineKubeletVersion != currentKubeletVersion {
		// Upgrade kubelet to desired version.
		cmd := fmt.Sprintf("sudo apt-get install kubelet=%s", goalMachine.Spec.Versions.Kubelet+"-00")
		_, err := vc.remoteSshCommand(goalMachine, cmd, "~/.ssh/id_rsa", "ubuntu")
		if err != nil {
			glog.Infof("remoteSshCommand while installing new kubelet version: %v", err)
			return err
		}
	}

	return nil
}

func (vc *VsphereClient) remoteSshCommand(m *clusterv1.Machine, cmd, privateKeyPath, sshUser string) (string, error) {
	glog.Infof("Remote SSH execution '%s' on %s", cmd, m.ObjectMeta.Name)

	publicIP, err := vc.GetIP(m)
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

func (vc *VsphereClient) GetIP(machine *clusterv1.Machine) (string, error) {
	if machine.ObjectMeta.Annotations != nil {
		if ip, ok := machine.ObjectMeta.Annotations[MasterIpAnnotationKey]; ok {
			glog.Infof("Returning IP from machine annotation %s", ip)
			return ip, nil
		}
	}

	ipBytes, _ := ioutil.ReadFile(path.Join(fmt.Sprintf(MachinePathStageFormat, machine.ObjectMeta.Name), MasterIpFilename))
	if ipBytes != nil {
		glog.Infof("Returning IP from file %s", string(ipBytes))
		return string(ipBytes), nil
	}

	return "", errors.New("could not get IP")
}

func (vc *VsphereClient) GetTfState(machine *clusterv1.Machine) (string, error) {
	if machine.ObjectMeta.Annotations != nil {
		if tfStateB64, ok := machine.ObjectMeta.Annotations[StatusMachineTerraformState]; ok {
			glog.Infof("Returning tfstate from annotation")
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
		glog.Infof("Returning tfstate from staging file")
		return string(tfStateBytes), nil
	}

	return "", errors.New("could not get tfstate")
}

func (vc *VsphereClient) GetKubeConfig(master *clusterv1.Machine) (string, error) {
	ip, err := vc.GetIP(master)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	cmd := exec.Command(
		"ssh", "-i", "~/.ssh/vsphere_tmp",
		"-q",
		"-o", "StrictHostKeyChecking no",
		"-o", "UserKnownHostsFile /dev/null",
		fmt.Sprintf("ubuntu@%s", ip),
		"sudo cat /etc/kubernetes/admin.conf")
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	cmd.Run()
	result := strings.TrimSpace(out.String())
	return result, nil
}

// DEPRECATED. Remove after vsphere-deployer is deleted.
// This method exists because during bootstrap some artifacts need to be transferred to the
// remote master. These include the IP address for the master, terraform state file.
// In GCE, we set this kind of data as GCE metadata. Unfortunately, vSphere does not have
// a comparable API.
func (vc *VsphereClient) SetupRemoteMaster(master *clusterv1.Machine) error {
	machineName := master.ObjectMeta.Name
	glog.Infof("Setting up the remote master[%s] with terraform config.", machineName)

	ip, err := vc.GetIP(master)
	if err != nil {
		return err
	}

	glog.Infof("Copying the staging directory to master.")
	machinePath := fmt.Sprintf(MachinePathStageFormat, machineName)
	_, err = vc.remoteSshCommand(master, fmt.Sprintf("mkdir -p %s", machinePath), "~/.ssh/vsphere_tmp", "ubuntu")
	if err != nil {
		glog.Infof("remoteSshCommand failed while creating remote machine stage: %+v", err)
		return err
	}

	cmd := exec.Command(
		"scp", "-i", "~/.ssh/vsphere_tmp",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-r",
		machinePath,
		fmt.Sprintf("ubuntu@%s:%s", ip, StageDir))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	return nil
}

// We are storing these as annotations and not in Machine Status because that's intended for
// "Provider-specific status" that will usually be used to detect updates. Additionally,
// Status requires yet another version API resource which is too heavy to store IP and TF state.
func (vc *VsphereClient) updateAnnotations(machine *clusterv1.Machine, masterEndpointIp, tfState string) error {
	glog.Infof("Updating annotations for machine %s", machine.ObjectMeta.Name)
	if machine.ObjectMeta.Annotations == nil {
		machine.ObjectMeta.Annotations = make(map[string]string)
	}

	tfStateB64 := base64.StdEncoding.EncodeToString([]byte(tfState))

	machine.ObjectMeta.Annotations[MasterIpAnnotationKey] = masterEndpointIp
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

// If the VsphereClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (vc *VsphereClient) handleMachineError(machine *clusterv1.Machine, err *apierrors.MachineError) error {
	if vc.machineClient != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		vc.machineClient.UpdateStatus(machine)
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

// TODO: We need to change this when we create dedicated service account for apiserver/controller
// pod.
func CreateExtApiServerRoleBinding() error {
	return run("kubectl", "create", "rolebinding",
		"-n", "kube-system", "machine-controller", "--role=extension-apiserver-authentication-reader",
		"--serviceaccount=default:default")
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
