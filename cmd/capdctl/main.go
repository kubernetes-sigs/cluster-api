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
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions/resource"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api-provider-docker/kind/controlplane"
	"sigs.k8s.io/cluster-api-provider-docker/objects"
	_ "sigs.k8s.io/cluster-api-provider-docker/objects"
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

type machineDeploymentOptions struct {
	name, namespace, clusterName, kubeletVersion *string
	replicas                                     *int
}

func (mo *machineDeploymentOptions) initFlags(fs *flag.FlagSet) {
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
	machineDeploymentOpts := new(machineDeploymentOptions)
	machineDeploymentOpts.initFlags(machineDeployment)

	kflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(kflags)

	if len(os.Args) < 2 {
		fmt.Println("At least one subcommand is requied.")
		fmt.Println(usage())
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		setup.Parse(os.Args[2:])
		makeManagementCluster(*managementClusterName, *version, *capdImage, *capiImage)
	case "apply":
		kflags.Parse(os.Args[2:])
		applyControlPlane(*managementClusterName, *version, *capiImage, *capdImage)
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

	klog.Flush()
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

func clusterYAML(clusterName, namespace string) string {
	cluster := objects.GetCluster(clusterName, namespace)
	return marshal(&cluster)
}

func machineYAML(opts *machineOptions) string {
	machine := objects.GetMachine(*opts.name, *opts.namespace, *opts.clusterName, *opts.set, *opts.version)
	return marshal(&machine)
}

func machineDeploymentYAML(opts *machineDeploymentOptions) string {
	machineDeploy := objects.GetMachineDeployment(*opts.name, *opts.namespace, *opts.clusterName, *opts.kubeletVersion, int32(*opts.replicas))
	return marshal(&machineDeploy)

}

func marshal(obj runtime.Object) string {
	b, err := json.Marshal(obj)
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

	if err := controlplane.CreateKindCluster(capiImage, clusterName); err != nil {
		panic(err)
	}

	applyControlPlane(clusterName, capiVersion, capiImage, capdImage)
}

func applyControlPlane(clusterName, capiVersion, capiImage, capdImage string) {
	fmt.Println("Downloading the latest CRDs for CAPI version", capiVersion)
	objects, err := objects.GetManegementCluster(capiVersion, capiImage, capdImage)
	if err != nil {
		panic(err)
	}

	fmt.Println("Applying the control plane")

	cfg, err := controlplane.GetKubeconfig(clusterName)
	if err != nil {
		panic(err)
	}

	helper, err := NewAPIHelper(cfg)
	if err != nil {
		panic(err)
	}

	for _, obj := range objects {
		if helper.Create(obj); err != nil {
			panic(err)
		}
	}
}

type APIHelper struct {
	cfg    *rest.Config
	mapper meta.RESTMapper
}

func NewAPIHelper(cfg *rest.Config) (*APIHelper, error) {
	discover, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "could not create discovery client")
	}

	groupResources, err := restmapper.GetAPIGroupResources(discover)
	if err != nil {
		return nil, errors.Wrap(err, "could not get api group resources")
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	return &APIHelper{
		cfg,
		mapper,
	}, nil
}

func (a *APIHelper) Create(obj runtime.Object) error {
	gvk := obj.GetObjectKind().GroupVersionKind()

	a.cfg.ContentConfig = resource.UnstructuredPlusDefaultContentConfig()

	client, err := rest.UnversionedRESTClientFor(a.cfg)
	if err != nil {
		return errors.Wrap(err, "couldn't create REST client")
	}

	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrap(err, "couldn't create accessor")
	}

	mapping, err := a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)

	if err != nil {
		return errors.Wrapf(err, "failed to retrieve mapping for %s %s", gvk.String(), accessor.GetName())
	}

	fmt.Printf("Creating %s %s in %q\n", gvk.String(), accessor.GetName(), accessor.GetNamespace())

	result := client.
		Post().
		AbsPath(makeURLSegments(mapping.Resource, "", accessor.GetNamespace())...).
		Body(obj).
		Do()

	return errors.Wrapf(result.Error(), "failed to create object %q", accessor.GetName())
}

func makeURLSegments(resource schema.GroupVersionResource, name, namespace string) []string {
	url := []string{}
	if len(resource.Group) == 0 {
		url = append(url, "api")
	} else {
		url = append(url, "apis", resource.Group)
	}
	url = append(url, resource.Version)

	if len(namespace) > 0 {
		url = append(url, "namespaces", namespace)
	}
	url = append(url, resource.Resource)

	if len(name) > 0 {
		url = append(url, name)
	}

	return url
}
