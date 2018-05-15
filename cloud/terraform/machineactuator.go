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

package terraform

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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/cloud/terraform/namedmachines"
	terraformconfig "sigs.k8s.io/cluster-api/cloud/terraform/terraformproviderconfig"
	terraformconfigv1 "sigs.k8s.io/cluster-api/cloud/terraform/terraformproviderconfig/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
)

const (
	MasterIpAnnotationKey            = "master-ip"
	TerraformConfigAnnotationKey     = "tf-config"
	ControlPlaneVersionAnnotationKey = "control-plane-version"
	KubeletVersionAnnotationKey      = "kubelet-version"

	// Filename in which named machines are saved using a ConfigMap (in master).
	NamedMachinesFilename = "vsphere_named_machines.yaml"
)

type TerraformClient struct {
	scheme            *runtime.Scheme
	codecFactory      *serializer.CodecFactory
	kubeadmToken      string
	machineClient     client.MachineInterface
	namedMachineWatch *namedmachines.ConfigWatch
}

func NewMachineActuator(kubeadmToken string, machineClient client.MachineInterface, namedMachinePath string) (*TerraformClient, error) {
	scheme, codecFactory, err := terraformconfigv1.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}

	var nmWatch *namedmachines.ConfigWatch
	nmWatch, err = namedmachines.NewConfigWatch(namedMachinePath)
	if err != nil {
		glog.Errorf("error creating named machine config watch: %+v", err)
	}
	return &TerraformClient{
		scheme:            scheme,
		codecFactory:      codecFactory,
		kubeadmToken:      kubeadmToken,
		machineClient:     machineClient,
		namedMachineWatch: nmWatch,
	}, nil
}

func (tf *TerraformClient) CreateMachineController(cluster *clusterv1.Cluster, initialMachines []*clusterv1.Machine, clientSet kubernetes.Clientset) error {
	if err := CreateExtApiServerRoleBinding(); err != nil {
		return err
	}

	// Create the named machines ConfigMap.
	// After pivot-based bootstrap is done, the named machine should be a ConfigMap and this logic will be removed.
	namedMachines, err := tf.namedMachineWatch.NamedMachines()

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

	if err := CreateApiServerAndController(tf.kubeadmToken); err != nil {
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

func (tf *TerraformClient) Create(cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	config, err := tf.providerconfig(machine.Spec.ProviderConfig)
	if err != nil {
		return tf.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
			"Cannot unmarshal providerConfig field: %v", err))
	}

	machineName := machine.ObjectMeta.Name

	// Set up the file system.
	// Create ~/.terraform.d/plugins and ~/.terraform.d/cluster/machines/$machineName directories
	homedir, err := getHomeDir()
	if err != nil {
		return err
	}
	pluginsPath := path.Join(homedir, ".terraform.d/plugins")
	machinePath := path.Join(homedir, fmt.Sprintf(".terraform.d/kluster/machines/%s", machineName))
	if _, err := os.Stat(pluginsPath); os.IsNotExist(err) {
		os.MkdirAll(pluginsPath, 0755)
	}
	if _, err := os.Stat(machinePath); os.IsNotExist(err) {
		os.MkdirAll(machinePath, 0755)
	}
	// Save the config file and variables file to the machinePath
	tfConfigPath := path.Join(machinePath, "terraform.tf")
	tfVarsPath := path.Join(machinePath, "variables.tfvars")
	namedMachines, err := tf.namedMachineWatch.NamedMachines()
	if err != nil {
		return err
	}
	matchedMachine, err := namedMachines.MatchMachine(config.TerraformMachine)
	if err != nil {
		return err
	}
	if err := saveFile(matchedMachine.MachineHcl, tfConfigPath, 0644); err != nil {
		return err
	}
	if err := saveFile(strings.Join(config.TerraformVariables, "\n"), tfVarsPath, 0644); err != nil {
		return err
	}

	if verr := tf.validateMachine(machine, config); verr != nil {
		return tf.handleMachineError(machine, verr)
	}

	if verr := tf.validateCluster(cluster); verr != nil {
		return verr
	}

	preloaded := false
	var startupScript string

	if util.IsMaster(machine) {
		if machine.Spec.Versions.ControlPlane == "" {
			return tf.handleMachineError(machine, apierrors.InvalidMachineConfiguration(
				"invalid master configuration: missing Machine.Spec.Versions.ControlPlane"))
		}
		var err error
		startupScript, err = getMasterStartupScript(
			templateParams{
				Token:     tf.kubeadmToken,
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
				Token:     tf.kubeadmToken,
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

	instance, err := tf.instanceIfExists(machine)
	if err != nil {
		return err
	}

	if instance == nil {
		glog.Infof("Will start creating machine %s", machine.ObjectMeta.Name)
		tfConfigDir := machinePath

		// Check if we need to terraform init
		tfInitExists, e := pathExists(path.Join(tfConfigDir, ".terraform/"))
		if e != nil {
			return errors.New(fmt.Sprintf("Could not get the path of the file: %+v", e))
		}
		if !tfInitExists {
			glog.Infof("Terraform not initialized.. Running terraform init.")
			_, initErr := runTerraformCmd(true, tfConfigDir, "init")
			if initErr != nil {
				return errors.New(fmt.Sprintf("Could not run terraform: %+v", initErr))
			}
		}

		// Now create the machine
		var args []string
		args = append(args, "apply")
		args = append(args, "-auto-approve")
		args = append(args, "-input=false")
		args = append(args, "-var")
		args = append(args, fmt.Sprintf("vm_name=%s", machine.ObjectMeta.Name))
		args = append(args, fmt.Sprintf("-var-file=%s", tfVarsPath))

		_, cmdErr := runTerraformCmd(false, tfConfigDir, args...)
		if cmdErr != nil {
			return errors.New(fmt.Sprintf("Could not run terraform: %s", cmdErr))
		}

		// Get the IP address
		out, cmdErr := runTerraformCmd(false, tfConfigDir, "output", "ip_address")
		if cmdErr != nil {
			return fmt.Errorf("could not obtain 'ip_address' output variable: %s", cmdErr)
		}
		masterEndpointIp := strings.TrimSpace(out.String())
		glog.Infof("Master created with ip address %s", masterEndpointIp)

		// If we have a machineClient, then annotate the machine so that we
		// remember exactly what VM we created for it.
		if tf.machineClient != nil {
			return tf.updateAnnotations(machine, masterEndpointIp)
		} else {
			masterIpFile := path.Join(machinePath, "master-ip")
			glog.Infof("Since we are bootstrapping, saving IP address to %s", masterIpFile)
			if err := saveFile(masterEndpointIp, masterIpFile, 0644); err != nil {
				return errors.New(fmt.Sprintf("Could not write master IP to file %s", err))
			}
		}
	} else {
		glog.Infof("Skipped creating a VM that already exists.\n")
	}

	return nil
}

// Get the directory that holds the file at the passed path.
func getDirForFile(path string) (string, error) {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return dir, nil
}

// Set stdout=true to redirect process's standard out to sys stdout.
// Otherwise returns a byte buffer of stdout.
func runTerraformCmd(stdout bool, workingDir string, arg ...string) (bytes.Buffer, error) {
	var out bytes.Buffer

	// Lookup terraform binary where it is copied in the controller.
	homedir, err := getHomeDir()
	if err != nil {
		return bytes.Buffer{}, err
	}
	terraformPath := path.Join(homedir, ".terraform.d/bin/terraform")
	if _, err := os.Stat(terraformPath); os.IsNotExist(err) {
		glog.Infof("terraform binary not found in .terraform.d. Looking in PATH.")
		tfPath, err := exec.LookPath("terraform")
		if err != nil {
			return bytes.Buffer{}, errors.New("terraform binary not found.")
		}
		terraformPath = tfPath
		glog.Infof("terraform path: %s", terraformPath)
	}
	cmd := exec.Command(terraformPath, arg...)
	// If stdout, only show in stdout
	if stdout {
		cmd.Stdout = os.Stdout
	} else {
		// Otherwise, save to buffer, and to a local log file.
		logFileName := fmt.Sprintf("/tmp/cluster-api-%s.log", util.RandomToken())
		f, _ := os.Create(logFileName)
		glog.Infof("Running terraform. Check for logs in %s", logFileName)
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

func (tf *TerraformClient) Delete(machine *clusterv1.Machine) error {
	glog.Infof("TERRAFORM DELETE.\n")
	return nil
}

func (tf *TerraformClient) PostDelete(cluster *clusterv1.Cluster, machines []*clusterv1.Machine) error {
	return nil
}

func (tf *TerraformClient) Update(cluster *clusterv1.Cluster, goalMachine *clusterv1.Machine) error {
	// Try to get the annotations for the versions. If they don't exist, update the annotation and return.
	// This can only happen right after bootstrapping.
	if goalMachine.ObjectMeta.Annotations == nil {
		ip, _ := tf.GetIP(goalMachine)
		glog.Info("Annotations do not exist. Populating existing state for bootstrapped machine.")
		return tf.updateAnnotations(goalMachine, ip)
	}

	// Check if the annotations we want to track exist, if not, the user likely created a master machine with their own annotation.
	if _, ok := goalMachine.ObjectMeta.Annotations[ControlPlaneVersionAnnotationKey]; !ok {
		ip, _ := tf.GetIP(goalMachine)
		glog.Info("Version annotations do not exist. Populating existing state for bootstrapped machine.")
		return tf.updateAnnotations(goalMachine, ip)
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
			err := tf.updateMasterInPlace(goalMachine)
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
func (tf *TerraformClient) updateMasterInPlace(goalMachine *clusterv1.Machine) error {
	goalMachineControlPlaneVersion := goalMachine.Spec.Versions.ControlPlane
	goalMachineKubeletVersion := goalMachine.Spec.Versions.Kubelet

	currentControlPlaneVersion := goalMachine.Annotations[ControlPlaneVersionAnnotationKey]
	currentKubeletVersion := goalMachine.Annotations[KubeletVersionAnnotationKey]

	// Control plane upgrade
	if goalMachineControlPlaneVersion != currentControlPlaneVersion {
		// Pull the kudeadm for target version K8s.
		cmd := fmt.Sprintf("curl -sSL https://dl.k8s.io/release/v%s/bin/linux/amd64/kubeadm | sudo tee /usr/bin/kubeadm > /dev/null; " +
			"sudo chmod a+rx /usr/bin/kubeadm", goalMachineControlPlaneVersion)
		_, err := tf.remoteSshCommand(goalMachine, cmd, "~/.ssh/id_rsa", "ubuntu")
		if err != nil {
			glog.Infof("remoteSshCommand failed while downloading new kubeadm: %+v", err)
			return err
		}

		// Next upgrade control plane
		cmd = fmt.Sprintf("sudo kubeadm upgrade apply %s -y", "v"+goalMachine.Spec.Versions.ControlPlane)
		_, err = tf.remoteSshCommand(goalMachine, cmd, "~/.ssh/id_rsa", "ubuntu")
		if err != nil {
			glog.Infof("remoteSshCommand failed while upgrading control plane: %+v", err)
			return err
		}
	}

	// Upgrade kubelet
	if goalMachineKubeletVersion != currentKubeletVersion {
		// Upgrade kubelet to desired version.
		cmd := fmt.Sprintf("sudo apt-get install kubelet=%s", goalMachine.Spec.Versions.Kubelet+"-00")
		_, err := tf.remoteSshCommand(goalMachine, cmd, "~/.ssh/id_rsa", "ubuntu")
		if err != nil {
			glog.Infof("remoteSshCommand while installing new kubelet version: %v", err)
			return err
		}
	}

	return nil
}

func (tf *TerraformClient) remoteSshCommand(m *clusterv1.Machine, cmd, privateKeyPath, sshUser string) (string, error) {
	glog.Infof("Remote SSH execution '%s' on %s", cmd, m.ObjectMeta.Name)

	publicIP, err := tf.GetIP(m)
	if err != nil {
		return "", err
	}

	command := fmt.Sprintf("echo STARTFILE; %s", cmd)
	c := exec.Command("ssh", "-i", privateKeyPath, sshUser+"@"+publicIP,
		"-o", "StrictHostKeyChecking no",
		"-o", "UserKnownHostsFile /dev/null",
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

func (tf *TerraformClient) Exists(machine *clusterv1.Machine) (bool, error) {
	i, err := tf.instanceIfExists(machine)
	if err != nil {
		return false, err
	}
	return (i != nil), err
}

func (tf *TerraformClient) GetIP(machine *clusterv1.Machine) (string, error) {
	if machine.ObjectMeta.Annotations != nil {
		if ip, ok := machine.ObjectMeta.Annotations[MasterIpAnnotationKey]; ok {
			glog.Infof("Retuning IP from metadata %s", ip)
			return ip, nil
		}
	}

	homedir, err := getHomeDir()
	if err != nil {
		return "", err
	}
	ipBytes, _ := ioutil.ReadFile(path.Join(homedir, fmt.Sprintf(".terraform.d/kluster/machines/%s/%s", machine.ObjectMeta.Name, "master-ip")))
	if ipBytes != nil {
		glog.Infof("Retuning IP from file %s", string(ipBytes))
		return string(ipBytes), nil
	}

	return "", errors.New("Could not get IP")
}

func (tf *TerraformClient) GetKubeConfig(master *clusterv1.Machine) (string, error) {
	ip, err := tf.GetIP(master)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	cmd := exec.Command(
		// TODO: this is taking my private key and username for now.
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

// After master created, move the plugins folder from local to
// master's .terraform.d/plugins.
// Currently each init will download the plugin, so for a large number of machines,
// this will eat up a lot of space.
func (tf *TerraformClient) SetupRemoteMaster(master *clusterv1.Machine) error {
	// Because bootstrapping machine and the remote machine can be different platform,
	// we copy over the master from local to remote, then terraform init for that platform,
	// and copy the plugins to the remote's .terraform.d directory.
	machineName := master.ObjectMeta.Name
	glog.Infof("Setting up the remote master[%s] with terraform config and plugin.", machineName)

	ip, err := tf.GetIP(master)
	if err != nil {
		return err
	}

	glog.Infof("Copying .terraform.d directory to master.")
	homedir, err := getHomeDir()
	if err != nil {
		return err
	}
	cmd := exec.Command(
		"scp", "-i", "~/.ssh/vsphere_tmp",
		"-o", "StrictHostKeyChecking no",
		"-o", "UserKnownHostsFile /dev/null",
		"-r",
		path.Join(homedir, ".terraform.d"),
		fmt.Sprintf("ubuntu@%s:~/", ip))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	glog.Infof("Setting up terraform on remote master.")
	cmd = exec.Command(
		// TODO: this is taking my private key and username for now.
		"ssh", "-i", "~/.ssh/vsphere_tmp",
		"-o", "StrictHostKeyChecking no",
		"-o", "UserKnownHostsFile /dev/null",
		fmt.Sprintf("ubuntu@%s", ip),
		fmt.Sprintf("source ~/.profile; cd ~/.terraform.d/kluster/machines/%s; ~/.terraform.d/bin/terraform init; cp -r ~/.terraform.d/kluster/machines/%s/.terraform/plugins/* ~/.terraform.d/plugins/", machineName, machineName))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	return nil
}

func (tf *TerraformClient) updateAnnotations(machine *clusterv1.Machine, masterEndpointIp string) error {
	if machine.ObjectMeta.Annotations == nil {
		machine.ObjectMeta.Annotations = make(map[string]string)
	}
	machine.ObjectMeta.Annotations[MasterIpAnnotationKey] = masterEndpointIp
	machine.ObjectMeta.Annotations[ControlPlaneVersionAnnotationKey] = machine.Spec.Versions.ControlPlane
	machine.ObjectMeta.Annotations[KubeletVersionAnnotationKey] = machine.Spec.Versions.Kubelet

	_, err := tf.machineClient.Update(machine)
	if err != nil {
		return err
	}
	//err = tf.updateInstanceStatus(machine)
	return nil
}

func (tf *TerraformClient) instanceIfExists(machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	homedir, err := getHomeDir()
	if err != nil {
		return nil, err
	}
	tfConfigDir := path.Join(homedir, fmt.Sprintf(".terraform.d/kluster/machines/%s", machine.ObjectMeta.Name))
	out, tfCmdErr := runTerraformCmd(false, tfConfigDir, "show")
	if tfCmdErr != nil {
		glog.Infof("Ignore terrafrom error in instanceIfExists: %s", err)
		return nil, nil
	}
	re := regexp.MustCompile(fmt.Sprintf("\n[[:space:]]*(name = %s)\n", machine.ObjectMeta.Name))
	if re.MatchString(out.String()) {
		return machine, nil
	}
	return nil, nil
}

func (tf *TerraformClient) providerconfig(providerConfig clusterv1.ProviderConfig) (*terraformconfig.TerraformProviderConfig, error) {
	obj, gvk, err := tf.codecFactory.UniversalDecoder().Decode(providerConfig.Value.Raw, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decoding failure: %v", err)
	}

	config, ok := obj.(*terraformconfig.TerraformProviderConfig)
	if !ok {
		return nil, fmt.Errorf("failure to cast to tf; type: %v", gvk)
	}

	return config, nil
}

func (tf *TerraformClient) validateMachine(machine *clusterv1.Machine, config *terraformconfig.TerraformProviderConfig) *apierrors.MachineError {
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

func (tf *TerraformClient) validateCluster(cluster *clusterv1.Cluster) error {
	if cluster.Spec.ClusterNetwork.ServiceDomain == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.ServiceDomain")
	}
	if getSubnet(cluster.Spec.ClusterNetwork.Pods) == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.Pods")
	}
	if getSubnet(cluster.Spec.ClusterNetwork.Services) == "" {
		return errors.New("invalid cluster configuration: missing Cluster.Spec.ClusterNetwork.Services")
	}
	return nil
}

// If the TerraformClient has a client for updating Machine objects, this will set
// the appropriate reason/message on the Machine.Status. If not, such as during
// cluster installation, it will operate as a no-op. It also returns the
// original error for convenience, so callers can do "return handleMachineError(...)".
func (tf *TerraformClient) handleMachineError(machine *clusterv1.Machine, err *apierrors.MachineError) error {
	if tf.machineClient != nil {
		reason := err.Reason
		message := err.Message
		machine.Status.ErrorReason = &reason
		machine.Status.ErrorMessage = &message
		tf.machineClient.UpdateStatus(machine)
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
//
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
