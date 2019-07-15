/*
Copyright 2019 The Kubernetes Authors.

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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/cri"
	"sigs.k8s.io/kind/pkg/exec"
)

// TODO: Generate the RBAC stuff from somewhere instead of copy pasta

const (
	// Important to keep this consistent.
	controlPlaneSet = "controlplane"
)

type machineOptions struct {
	name, namespace, clusterName, set, version *string
}

func (mo *machineOptions) initFlags(fs *flag.FlagSet) {
	mo.name = fs.String("name", "my-machine", "The name of the machine")
	mo.namespace = fs.String("namespace", "my-namespace", "The namespece of the machine")
	mo.clusterName = fs.String("cluster-name", "my-cluster", "The name of the cluster the machine belongs to")
	mo.set = fs.String("set", "worker", "The role of the machine. Valid entries ['worker', 'controlplane']")
	mo.version = fs.String("version", "v1.14.2", "The Kubernetes version to run")
}

type machineDeyploymentOptions struct {
	name, namespace, clusterName, kubeletVersion *string
	replicas                                     *int
}

func (mo *machineDeyploymentOptions) initFlags(fs *flag.FlagSet) {
	mo.name = fs.String("name", "my-machine-deployment", "The name of the machine deployment")
	mo.namespace = fs.String("namespace", "my-namespace", "The namespace of the machine deployment")
	mo.clusterName = fs.String("cluster-name", "my-cluster", "The name of the cluster the machine deployment creates machines for")
	mo.kubeletVersion = fs.String("kubelet-version", "v1.14.2", "The Kubernetes kubelet version to run")
	mo.replicas = fs.Int("replicas", 1, "The number of replicas")
}

func main() {
	setup := flag.NewFlagSet("setup", flag.ExitOnError)
	managementClusterName := setup.String("cluster-name", "management", "The name of the management cluster")
	version := setup.String("capi-version", "v0.1.6", "The CRD versions to pull from CAPI. Does not support < v0.1.6.")
	capdImage := setup.String("capd-image", "gcr.io/kubernetes1-226021/capd-manager:latest", "The capd manager image to run")
	capiImage := setup.String("capi-image", "", "This is normally left blank and filled in automatically. But this will override the generated image name.")

	controlPlane := flag.NewFlagSet("control-plane", flag.ExitOnError)
	controlPlaneOpts := new(machineOptions)
	controlPlaneOpts.initFlags(controlPlane)
	*controlPlaneOpts.set = controlPlaneSet

	worker := flag.NewFlagSet("worker", flag.ExitOnError)
	workerOpts := new(machineOptions)
	workerOpts.initFlags(worker)
	*workerOpts.set = "worker"

	cluster := flag.NewFlagSet("cluster", flag.ExitOnError)
	clusterName := cluster.String("cluster-name", "my-cluster", "The name of the cluster")
	clusterNamespace := cluster.String("namespace", "my-namespace", "The namespace the cluster belongs to")

	machineDeployment := flag.NewFlagSet("machine-deployment", flag.ExitOnError)
	machineDeploymentOpts := new(machineDeyploymentOptions)
	machineDeploymentOpts.initFlags(machineDeployment)

	if len(os.Args) < 2 {
		fmt.Println("At least one subcommand is requied.")
		fmt.Println(usage())
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		setup.Parse(os.Args[2:])
		makeManagementCluster(*managementClusterName, *version, *capdImage, *capiImage)
	case "control-plane":
		controlPlane.Parse(os.Args[2:])
		fmt.Fprintf(os.Stdout, machineYAML(controlPlaneOpts))
	case "worker":
		worker.Parse(os.Args[2:])
		fmt.Fprintf(os.Stdout, machineYAML(workerOpts))
	case "cluster":
		cluster.Parse(os.Args[2:])
		fmt.Fprintf(os.Stdout, clusterYAML(*clusterName, *clusterNamespace))
	case "machine-deployment":
		machineDeployment.Parse(os.Args[2:])
		fmt.Fprint(os.Stdout, machineDeploymentYAML(machineDeploymentOpts))
	case "help":
		fmt.Println(usage())
	default:
		fmt.Println(usage())
		os.Exit(1)
	}
}

func usage() string {
	return `capdctl gets you up and running with capd

subcommands are:

  setup - Create a management cluster
    example: capdctl setup -cluster-name my-management-cluster-name

  crds - Write Cluster API CRDs required to run capd to stdout
    example: capdctl crds | kubectl apply -f -

  capd - Write capd kubernetes components that run necessary managers to stdout
    example: capdctl capd -capd-image gcr.io/kubernetes1-226021/capd-manager:latest -capi-image gcr.io/k8s-cluster-api/cluster-api-controller:0.1.2 | kubectl apply -f -

  control-plane - Write a capd control plane machine to stdout
    example: capdctl control-plane -name my-control-plane -namespace my-namespace -cluster-name my-cluster -version v1.14.1 | kubectl apply -f -

  worker - Write a capd worker machine to stdout
    example: capdctl worker -name my-worker -namespace my-namespace -cluster-name my-cluster -version 1.14.2 | kubectl apply -f -

  cluster - Write a capd cluster object to stdout
    example: capdctl cluster -cluster-name my-cluster -namespace my-namespace | kubectl apply -f -

  machine-deployment - Write a machine deployment object to stdout
    example: capdctl machine-deployment -name my-machine-deployment -cluster-name my-cluster -namespace my-namespace -kubelet-version v1.14.2 -replicas 1 | kubectl apply -f -
`
}

func clusterYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: %s
  namespace: %s
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  providerSpec: {}`, name, namespace)
}

func machineDeploymentYAML(opts *machineDeyploymentOptions) string {
	replicas := int32(*opts.replicas)
	labels := map[string]string{
		"cluster.k8s.io/cluster-name": *opts.clusterName,
		"set":                         "node",
	}
	deployment := v1alpha1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: "cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      *opts.name,
			Namespace: *opts.namespace,
			Labels:    labels,
		},
		Spec: v1alpha1.MachineDeploymentSpec{
			Replicas: &replicas,
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: v1alpha1.MachineTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: v1alpha1.MachineSpec{
					ProviderSpec: v1alpha1.ProviderSpec{},
					Versions: v1alpha1.MachineVersionInfo{
						Kubelet: *opts.kubeletVersion,
					},
				},
			},
		},
	}

	b, err := json.Marshal(deployment)
	// TODO don't panic on the error
	if err != nil {
		panic(err)
	}
	return string(b)
}

func machineYAML(opts *machineOptions) string {
	machine := v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: "cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      *opts.name,
			Namespace: *opts.namespace,
			Labels: map[string]string{
				"cluster.k8s.io/cluster-name": *opts.clusterName,
				"set":                         *opts.set,
			},
		},
		Spec: v1alpha1.MachineSpec{
			ProviderSpec: v1alpha1.ProviderSpec{},
		},
	}
	if *opts.set == controlPlaneSet {
		machine.Spec.Versions.ControlPlane = *opts.version
	}
	if *opts.set == "worker" {
		machine.Spec.Versions.Kubelet = *opts.version
	}
	b, err := json.Marshal(machine)
	// TODO don't panic on the error
	if err != nil {
		panic(err)
	}
	return string(b)
}

func makeManagementCluster(clusterName, capiVersion, capdImage, capiImageOverride string) {
	fmt.Println("Creating a brand new cluster")
	capiImage := fmt.Sprintf("us.gcr.io/k8s-artifacts-prod/cluster-api/cluster-api-controller:%s", capiVersion)
	if capiImageOverride != "" {
		capiImage = capiImageOverride
	}
	elb, err := actions.SetUpLoadBalancer(clusterName)
	if err != nil {
		panic(err)
	}
	lbipv4, _, err := elb.IP()
	if err != nil {
		panic(err)
	}
	cpMounts := []cri.Mount{
		{
			ContainerPath: "/var/run/docker.sock",
			HostPath:      "/var/run/docker.sock",
		},
	}
	cp, err := actions.CreateControlPlane(clusterName, fmt.Sprintf("%s-control-plane", clusterName), lbipv4, "v1.14.2", cpMounts)
	if err != nil {
		panic(err)
	}
	if !nodes.WaitForReady(cp, time.Now().Add(5*time.Minute)) {
		panic(errors.New("control plane was not ready in 5 minutes"))
	}
	f, err := ioutil.TempFile("", "crds")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())
	fmt.Println("Downloading the latest CRDs for CAPI version", capiVersion)
	crds, err := getCRDs(capiVersion, capiImage)
	if err != nil {
		panic(err)
	}
	fmt.Fprintln(f, crds)
	fmt.Fprintln(f, "---")
	fmt.Fprintln(f, capdRBAC)
	fmt.Fprintln(f, "---")
	fmt.Fprintln(f, getCAPDPlane(capdImage))
	fmt.Println("Applying the control plane", f.Name())
	cmd := exec.Command("kubectl", "apply", "-f", f.Name())
	cmd.SetEnv(fmt.Sprintf("KUBECONFIG=%s/.kube/kind-config-%s", os.Getenv("HOME"), clusterName))
	cmd.SetStdout(os.Stdout)
	cmd.SetStderr(os.Stderr)
	if err := cmd.Run(); err != nil {
		out, _ := ioutil.ReadFile(f.Name())
		fmt.Println(out)
		panic(err)
	}
}

func getCAPDPlane(capdImage string) string {
	return fmt.Sprintf(capiPlane, capdImage)
}

var capiPlane = `
apiVersion: v1
kind: Namespace
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: docker-provider-system
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  labels:
    control-plane: controller-manager
  name: docker-provider-controller-manager
  namespace: docker-provider-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  serviceName: docker-provider-controller-manager-service
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - name: capd-manager
        image: %s
        command:
        - capd-manager
        volumeMounts:
        - mountPath: /var/run/docker.sock
          name: dockersock
        - mountPath: /var/lib/docker
          name: dockerlib
        securityContext:
          privileged: true
      volumes:
      - name: dockersock
        hostPath:
          path: /var/run/docker.sock
          type: Socket
      - name: dockerlib
        hostPath:
          path: /var/lib/docker
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      - key: CriticalAddonsOnly
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/notReady
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/unreachable
        operator: Exists
`

// getCRDs should actually use kustomize to correctly build the manager yaml.
// HACK: this is a hacked function
func getCRDs(version, capiImage string) (string, error) {
	crds := []string{"crds", "rbac", "manager"}
	releaseCode := fmt.Sprintf("https://github.com/kubernetes-sigs/cluster-api/archive/%s.tar.gz", version)

	resp, err := http.Get(releaseCode)
	if err != nil {
		return "", errors.WithStack(err)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", errors.WithStack(err)
	}

	tgz := tar.NewReader(gz)
	var buf bytes.Buffer

	for {
		header, err := tgz.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return "", errors.WithStack(err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
			for _, crd := range crds {
				// Skip the kustomization files for now. Would like to use kustomize in future
				if strings.HasSuffix(header.Name, "kustomization.yaml") {
					continue
				}

				// This is a poor person's kustomize
				if strings.HasSuffix(header.Name, "manager.yaml") {
					var managerBuf bytes.Buffer
					io.Copy(&managerBuf, tgz)
					lines := strings.Split(managerBuf.String(), "\n")
					for _, line := range lines {
						if strings.Contains(line, "image:") {
							buf.WriteString(strings.Replace(line, "image: controller:latest", fmt.Sprintf("image: %s", capiImage), 1))
							buf.WriteString("\n")
							continue
						}
						buf.WriteString(line)
						buf.WriteString("\n")
					}
				}

				// These files don't need kustomize at all.
				if strings.Contains(header.Name, fmt.Sprintf("config/%s/", crd)) {
					io.Copy(&buf, tgz)
					fmt.Fprintln(&buf, "---")
				}
			}
		}
	}
	return buf.String(), nil
}

var capdRBAC = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: docker-provider-manager-role
rules:
- apiGroups:
  - cluster.k8s.io
  resources:
  - clusters
  - clusters/status
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - machines
  - machines/status
  - machinedeployments
  - machinedeployments/status
  - machinesets
  - machinesets/status
  - machineclasses
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - clusters
  - clusters/status
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - nodes
  - events
  - secrets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  creationTimestamp: null
  name: docker-provider-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: docker-provider-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: docker-provider-system
`
