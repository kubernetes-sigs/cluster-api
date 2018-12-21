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

	"k8s.io/api/core/v1"
	tcmd "k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap/existing"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap/minikube"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/providercomponents"

	"github.com/spf13/cobra"
	"k8s.io/klog"
)

type DeleteOptions struct {
	KubeconfigPath                string
	ProviderComponents            string
	ClusterNamespace              string
	CleanupBootstrapCluster       bool
	MiniKube                      []string
	VmDriver                      string
	ExistingClusterKubeconfigPath string
	KubeconfigOverrides           tcmd.ConfigOverrides
}

var do = &DeleteOptions{}

var deleteClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Delete kubernetes cluster",
	Long:  `Delete a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if do.KubeconfigPath == "" {
			exitWithHelp(cmd, "Please provide kubeconfig file for cluster to delete.")
		}
		if do.ProviderComponents == "" {
			exitWithHelp(cmd, "Please provide yaml file for provider component definition.")
		}
		if err := RunDelete(); err != nil {
			klog.Exit(err)
		}
	},
}

func init() {
	// Required flags
	deleteClusterCmd.Flags().StringVarP(&do.KubeconfigPath, "kubeconfig", "", "", "Path to the kubeconfig file to use for connecting to the cluster to be deleted, if empty, the default KUBECONFIG load path is used.")
	deleteClusterCmd.Flags().StringVarP(&do.ProviderComponents, "provider-components", "p", "", "A yaml file containing cluster api provider controllers and supporting objects, if empty the value is loaded from the cluster's configuration store.")

	// Optional flags
	deleteClusterCmd.Flags().StringVarP(&do.ClusterNamespace, "cluster-namespace", "", v1.NamespaceDefault, "Namespace where the cluster to be deleted resides")
	deleteClusterCmd.Flags().BoolVarP(&do.CleanupBootstrapCluster, "cleanup-bootstrap-cluster", "", true, "Whether to cleanup the bootstrap cluster after bootstrap")
	deleteClusterCmd.Flags().StringSliceVarP(&do.MiniKube, "minikube", "", []string{}, "Minikube options")
	deleteClusterCmd.Flags().StringVarP(&do.VmDriver, "vm-driver", "", "", "Which vm driver to use for minikube")
	deleteClusterCmd.Flags().StringVarP(&do.ExistingClusterKubeconfigPath, "existing-bootstrap-cluster-kubeconfig", "e", "", "Path to an existing cluster's kubeconfig for bootstrapping (intead of using minikube)")
	// BindContextFlags will bind the flags cluster, namespace, and user
	tcmd.BindContextFlags(&do.KubeconfigOverrides.Context, deleteClusterCmd.Flags(), tcmd.RecommendedContextOverrideFlags(""))
	deleteCmd.AddCommand(deleteClusterCmd)
}

func RunDelete() error {
	providerComponents, err := loadProviderComponents()
	if err != nil {
		return err
	}
	clusterClient, err := clusterclient.NewFromDefaultSearchPath(do.KubeconfigPath, do.KubeconfigOverrides)
	if err != nil {
		return fmt.Errorf("error when creating cluster client: %v", err)
	}
	defer clusterClient.Close()

	var bootstrapProvider bootstrap.ClusterProvisioner
	if do.ExistingClusterKubeconfigPath != "" {
		bootstrapProvider, err = existing.NewExistingCluster(do.ExistingClusterKubeconfigPath)
		if err != nil {
			return err
		}
	} else {
		if do.VmDriver != "" {
			do.MiniKube = append(do.MiniKube, fmt.Sprintf("vm-driver=%s", do.VmDriver))
		}

		bootstrapProvider = minikube.WithOptions(do.MiniKube)
	}

	deployer := clusterdeployer.New(
		bootstrapProvider,
		clusterclient.NewFactory(),
		providerComponents,
		"",
		do.CleanupBootstrapCluster)
	return deployer.Delete(clusterClient, do.ClusterNamespace)
}

func loadProviderComponents() (string, error) {
	coreClients, err := clientcmd.NewCoreClientSetForDefaultSearchPath(do.KubeconfigPath, do.KubeconfigOverrides)
	if err != nil {
		return "", fmt.Errorf("error creating core clients: %v", err)
	}
	pcStore := providercomponents.Store{
		ExplicitPath: do.ProviderComponents,
		ConfigMap:    coreClients.CoreV1().ConfigMaps(v1.NamespaceDefault),
	}
	providerComponents, err := pcStore.Load()
	if err != nil {
		return "", fmt.Errorf("error when loading provider components: %v", err)
	}
	return providerComponents, nil
}
