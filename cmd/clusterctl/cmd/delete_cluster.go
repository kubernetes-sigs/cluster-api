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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	tcmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/providercomponents"
)

type DeleteOptions struct {
	KubeconfigPath      string
	ProviderComponents  string
	KubeconfigOverrides tcmd.ConfigOverrides
	BootstrapFlags      bootstrap.Options
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

	// BindContextFlags will bind the flags cluster, namespace, and user
	tcmd.BindContextFlags(&do.KubeconfigOverrides.Context, deleteClusterCmd.Flags(), tcmd.RecommendedContextOverrideFlags(""))

	do.BootstrapFlags.AddFlags(deleteClusterCmd.Flags())
	deleteCmd.AddCommand(deleteClusterCmd)
}

func RunDelete() error {
	providerComponents, err := loadProviderComponents()
	if err != nil {
		return err
	}
	clusterClient, err := clusterclient.NewFromDefaultSearchPath(do.KubeconfigPath, do.KubeconfigOverrides)
	if err != nil {
		return errors.Wrap(err, "error when creating cluster client")
	}
	defer clusterClient.Close()

	bootstrapProvider, err := bootstrap.Get(do.BootstrapFlags)
	if err != nil {
		return err
	}

	deployer := clusterdeployer.New(
		bootstrapProvider,
		clusterclient.NewFactory(),
		providerComponents,
		"",
		"",
		do.BootstrapFlags.Cleanup)

	return deployer.Delete(clusterClient)
}

func loadProviderComponents() (string, error) {
	coreClients, err := clientcmd.NewCoreClientSetForDefaultSearchPath(do.KubeconfigPath, do.KubeconfigOverrides)
	if err != nil {
		return "", errors.Wrap(err, "error creating core clients")
	}
	pcStore := providercomponents.Store{
		ExplicitPath: do.ProviderComponents,
		ConfigMap:    coreClients.CoreV1().ConfigMaps(v1.NamespaceDefault),
	}
	providerComponents, err := pcStore.Load()
	if err != nil {
		return "", errors.Wrap(err, "error when loading provider components")
	}
	return providerComponents, nil
}
