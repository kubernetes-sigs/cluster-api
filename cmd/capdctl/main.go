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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cluster-api-provider-docker/cmd/versioninfo"
	"sigs.k8s.io/cluster-api-provider-docker/kind/controlplane"
	"sigs.k8s.io/cluster-api-provider-docker/objects"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"
)

const (
	// Important to keep this consistent.
	controlPlaneSet = "controlplane"
	capdctl         = "capdctl"
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

type platformOptions struct {
	capiImage, capdImage, capiVersion *string
}

func (po *platformOptions) initFlags(fs *flag.FlagSet) {
	po.capiVersion = fs.String("capi-version", "v0.1.8", "The CRD versions to pull from CAPI. Does not support < v0.1.8.")
	po.capdImage = fs.String("capd-image", "gcr.io/kubernetes1-226021/capd-manager:latest", "The capd manager image to run")
	po.capiImage = fs.String("capi-image", "", "This is normally left blank and filled in automatically. But this will override the generated image name.")
}

func addClusterName(fs *flag.FlagSet) *string {
	return fs.String("cluster-name", "management", "The name of the management cluster")
}

func checkErr(err error) {
	if err != nil {
		fmt.Printf("%+v\n", err)
		os.Exit(1)
	}
}

func main() {
	setup := flag.NewFlagSet("setup", flag.ExitOnError)
	managementClusterName := addClusterName(setup)
	setupPlatformOpts := new(platformOptions)
	setupPlatformOpts.initFlags(setup)

	kindSet := flag.NewFlagSet("kind", flag.ExitOnError)
	kindClusterName := addClusterName(kindSet)

	platform := flag.NewFlagSet("platform", flag.ExitOnError)
	platformOpts := new(platformOptions)
	platformOpts.initFlags(platform)

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

	if len(os.Args) < 2 {
		fmt.Println("At least one subcommand is requied.")
		fmt.Println(usage())
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		if err := setup.Parse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "%+v\n", err)
			os.Exit(1)
		}
		if err := makeManagementCluster(
			*managementClusterName,
			*setupPlatformOpts.capiVersion,
			*setupPlatformOpts.capdImage,
			*setupPlatformOpts.capiImage); err != nil {
			fmt.Printf("%+v\n", err)
			os.Exit(1)
		}
	case "kind":
		checkErr(kindSet.Parse(os.Args[2:]))
		checkErr(controlplane.CreateKindCluster(*kindClusterName))
		fmt.Printf("to use your new cluster:\nexport KUBECONFIG=%s\n", kindcluster.NewContext(*kindClusterName).KubeConfigPath())
	case "platform":
		checkErr(platform.Parse(os.Args[2:]))
		objs, err := getControlPlaneObjects(*platformOpts.capiVersion, *platformOpts.capdImage, *platformOpts.capiImage)
		checkErr(err)
		checkErr(printAll(objs))
	case "control-plane":
		checkErr(controlPlane.Parse(os.Args[2:]))
		m, err := machineYAML(controlPlaneOpts)
		checkErr(err)
		fmt.Fprintf(os.Stdout, m)
	case "worker":
		checkErr(worker.Parse(os.Args[2:]))
		m, err := machineYAML(workerOpts)
		checkErr(err)
		fmt.Fprintf(os.Stdout, m)
	case "cluster":
		checkErr(cluster.Parse(os.Args[2:]))
		c, err := clusterYAML(*clusterName, *clusterNamespace)
		checkErr(err)
		fmt.Fprintf(os.Stdout, c)
	case "machine-deployment":
		checkErr(machineDeployment.Parse(os.Args[2:]))
		md, err := machineDeploymentYAML(machineDeploymentOpts)
		checkErr(err)
		fmt.Fprint(os.Stdout, md)
	case "version":
		fmt.Print(versioninfo.VersionInfo(capdctl))
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

  kind - Create a kind cluster with docker directories mounted
    example: capdctl kind -cluster-name my-management-cluster-name

  platform - Write capd kubernetes components that run necessary managers and all CAPI crds to stdout
    example: capdctl platform -capd-image gcr.io/kubernetes1-226021/capd-manager:latest -capi-image gcr.io/k8s-cluster-api/cluster-api-controller:0.1.2 | kubectl apply -f -

  control-plane - Write a capd control plane machine to stdout
    example: capdctl control-plane -name my-control-plane -namespace my-namespace -cluster-name my-cluster -version v1.14.1 | kubectl apply -f -

  worker - Write a capd worker machine to stdout
    example: capdctl worker -name my-worker -namespace my-namespace -cluster-name my-cluster -version 1.14.2 | kubectl apply -f -

  cluster - Write a capd cluster object to stdout
    example: capdctl cluster -cluster-name my-cluster -namespace my-namespace | kubectl apply -f -

  machine-deployment - Write a machine deployment object to stdout
    example: capdctl machine-deployment -name my-machine-deployment -cluster-name my-cluster -namespace my-namespace -kubelet-version v1.14.2 -replicas 1 | kubectl apply -f -

  version - Print version information for capdctl
    example: capdctl version
`
}

func clusterYAML(clusterName, namespace string) (string, error) {
	cluster := objects.GetCluster(clusterName, namespace)
	b, err := json.Marshal(&cluster)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(b), nil
}

func machineYAML(opts *machineOptions) (string, error) {
	machine := objects.GetMachine(*opts.name, *opts.namespace, *opts.clusterName, *opts.set, *opts.version)
	b, err := json.Marshal(&machine)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(b), nil
}

func machineDeploymentYAML(opts *machineDeploymentOptions) (string, error) {
	machineDeploy := objects.GetMachineDeployment(*opts.name, *opts.namespace, *opts.clusterName, *opts.kubeletVersion, int32(*opts.replicas))
	b, err := json.Marshal(&machineDeploy)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(b), nil

}

func getControlPlaneObjects(capiVersion, capdImage, capiImageOverride string) ([]runtime.Object, error) {
	capiImage := fmt.Sprintf("us.gcr.io/k8s-artifacts-prod/cluster-api/cluster-api-controller:%s", capiVersion)
	if capiImageOverride != "" {
		capiImage = capiImageOverride
	}

	objs, err := objects.GetManegementCluster(capiVersion, capiImage, capdImage)
	if err != nil {
		return nil, err
	}

	return objs, nil
}

func makeManagementCluster(clusterName, capiVersion, capdImage, capiImageOverride string) error {
	fmt.Println("Creating a brand new cluster")
	if err := controlplane.CreateKindCluster(clusterName); err != nil {
		return err
	}

	cfg, err := controlplane.GetKubeconfig(clusterName)
	if err != nil {
		return err
	}

	objs, err := getControlPlaneObjects(capiVersion, capdImage, capiImageOverride)
	if err != nil {
		return err
	}

	return apply(cfg, objs)
}

func apply(cfg *rest.Config, objs []runtime.Object) error {
	client, err := crclient.New(cfg, crclient.Options{})
	if err != nil {
		return err
	}

	for _, obj := range objs {
		accessor, err := meta.Accessor(obj)
		if err != nil {
			return err
		}
		fmt.Printf("creating %q %q\n", obj.GetObjectKind().GroupVersionKind().String(), accessor.GetName())

		if err := client.Create(context.Background(), obj); err != nil {
			return err
		}
	}
	return nil
}

func printAll(objs []runtime.Object) error {
	// Stolen from https://github.com/kubernetes/kubernetes/blob/664edf832777cb7d6d00d38ccbcd4acba1497dc1/staging/src/k8s.io/kubectl/pkg/scheme/scheme.go#L37
	encoder := unstructured.JSONFallbackEncoder{Encoder: scheme.Codecs.LegacyCodec(scheme.Scheme.PrioritizedVersionsAllGroups()...)}

	for _, obj := range objs {
		if err := encoder.Encode(obj, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}
