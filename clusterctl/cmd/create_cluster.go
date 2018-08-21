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

package cmd

import (
	"fmt"
	"io/ioutil"

	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer/bootstrap/existing"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer/bootstrap/minikube"
	clustercommon "sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/util"
)

type CreateOptions struct {
	Cluster                       string
	Machine                       string
	ProviderComponents            string
	AddonComponents               string
	CleanupBootstrapCluster       bool
	VmDriver                      string
	Provider                      string
	KubeconfigOutput              string
	ExistingClusterKubeconfigPath string
}

var co = &CreateOptions{}

var createClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Create kubernetes cluster",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if co.Cluster == "" {
			exitWithHelp(cmd, "Please provide yaml file for cluster definition.")
		}
		if co.Machine == "" {
			exitWithHelp(cmd, "Please provide yaml file for machine definition.")
		}
		if co.ProviderComponents == "" {
			exitWithHelp(cmd, "Please provide yaml file for provider component definition.")
		}
		if err := RunCreate(co); err != nil {
			glog.Exit(err)
		}
	},
}

func RunCreate(co *CreateOptions) error {
	c, err := parseClusterYaml(co.Cluster)
	if err != nil {
		return err
	}
	m, err := parseMachinesYaml(co.Machine)
	if err != nil {
		return err
	}

	var bootstrapProvider clusterdeployer.ClusterProvisioner
	if co.ExistingClusterKubeconfigPath != "" {
		bootstrapProvider, err = existing.NewExistingCluster(co.ExistingClusterKubeconfigPath)
		if err != nil {
			return err
		}
	} else {
		bootstrapProvider = minikube.New(co.VmDriver)

	}

	pd, err := getProvider(co.Provider)
	if err != nil {
		return err
	}
	pc, err := ioutil.ReadFile(co.ProviderComponents)
	if err != nil {
		return fmt.Errorf("error loading provider components file '%v': %v", co.ProviderComponents, err)
	}
	var ac []byte
	if co.AddonComponents != "" {
		ac, err = ioutil.ReadFile(co.AddonComponents)
		if err != nil {
			return fmt.Errorf("error loading addons file '%v': %v", co.AddonComponents, err)
		}
	}
	pcsFactory := clusterdeployer.NewProviderComponentsStoreFactory()
	d := clusterdeployer.New(
		bootstrapProvider,
		clusterdeployer.NewClientFactory(),
		string(pc),
		string(ac),
		co.CleanupBootstrapCluster)
	return d.Create(c, m, pd, co.KubeconfigOutput, pcsFactory)
}

func init() {
	// Required flags
	createClusterCmd.Flags().StringVarP(&co.Cluster, "cluster", "c", "", "A yaml file containing cluster object definition")
	createClusterCmd.Flags().StringVarP(&co.Machine, "machines", "m", "", "A yaml file containing machine object definition(s)")
	createClusterCmd.Flags().StringVarP(&co.ProviderComponents, "provider-components", "p", "", "A yaml file containing cluster api provider controllers and supporting objects")
	// TODO: Remove as soon as code allows https://github.com/kubernetes-sigs/cluster-api/issues/157
	createClusterCmd.Flags().StringVarP(&co.Provider, "provider", "", "", "Which provider deployment logic to use (google/vsphere/azure)")

	// Optional flags
	createClusterCmd.Flags().StringVarP(&co.AddonComponents, "addon-components", "a", "", "A yaml file containing cluster addons to apply to the internal cluster")
	createClusterCmd.Flags().BoolVarP(&co.CleanupBootstrapCluster, "cleanup-bootstrap-cluster", "", true, "Whether to cleanup the bootstrap cluster after bootstrap")
	createClusterCmd.Flags().StringVarP(&co.VmDriver, "vm-driver", "", "", "Which vm driver to use for minikube")
	createClusterCmd.Flags().StringVarP(&co.KubeconfigOutput, "kubeconfig-out", "", "kubeconfig", "Where to output the kubeconfig for the provisioned cluster")
	createClusterCmd.Flags().StringVarP(&co.ExistingClusterKubeconfigPath, "existing-bootstrap-cluster-kubeconfig", "", "", "Path to an existing cluster's kubeconfig for bootstrapping (intead of using minikube)")

	createCmd.AddCommand(createClusterCmd)
}

func parseClusterYaml(file string) (*clusterv1.Cluster, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	cluster := &clusterv1.Cluster{}
	err = yaml.Unmarshal(bytes, cluster)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func parseMachinesYaml(file string) ([]*clusterv1.Machine, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	list := &clusterv1.MachineList{}
	err = yaml.Unmarshal(bytes, &list)
	if err != nil {
		return nil, err
	}

	if list == nil {
		return []*clusterv1.Machine{}, nil
	}

	return util.MachineP(list.Items), nil
}

func getProvider(name string) (clusterdeployer.ProviderDeployer, error) {
	provisioner, err := clustercommon.ClusterProvisioner(name)
	if err != nil {
		return nil, err
	}
	provider, ok := provisioner.(clusterdeployer.ProviderDeployer)
	if !ok {
		return nil, fmt.Errorf("provider for %s does not implement ProviderDeployer interface", name)
	}
	return provider, nil
}
