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

package cmd

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type deleteOptions struct {
	kubeconfig              string
	kubeconfigContext       string
	coreProvider            string
	bootstrapProviders      []string
	controlPlaneProviders   []string
	infrastructureProviders []string
	includeNamespace        bool
	includeCRDs             bool
	deleteAll               bool
}

var dd = &deleteOptions{}

var deleteCmd = &cobra.Command{
	Use:   "delete [providers]",
	Short: "Delete one or more providers from the management cluster.",
	Long: LongDesc(`
		Delete one or more providers from the management cluster.`),

	Example: Examples(`
		# Deletes the AWS provider
		# Please note that this implies the deletion of all provider components except the hosting namespace
		# and the CRDs.
		clusterctl delete --infrastructure aws

		# Deletes all the providers
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# Cluster API Providers are orphaned and there might be ongoing costs incurred as a result of this.
		clusterctl delete --all

		# Delete the AWS infrastructure provider and Core provider. This will leave behind Bootstrap and ControlPlane
		# providers
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# the AWS infrastructure provider and Cluster API Providers are orphaned and there might be
		# ongoing costs incurred as a result of this.
		clusterctl delete --core cluster-api --infrastructure aws

		# Delete the AWS infrastructure provider and related CRDs. Please note that this forces deletion of
		# all the related objects (e.g. AWSClusters, AWSMachines etc.).
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# the AWS infrastructure provider are orphaned and there might be ongoing costs incurred as a result of this.
		clusterctl delete --infrastructure aws --include-crd

		# Delete the AWS infrastructure provider and its hosting Namespace. Please note that this forces deletion of
		# all objects existing in the namespace.
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# Cluster API Providers are orphaned and there might be ongoing costs incurred as a result of this.
		clusterctl delete --infrastructure aws --include-namespace

		# Reset the management cluster to its original state
		# Important! As a consequence of this operation all the corresponding resources on target clouds
		# are "orphaned" and thus there may be ongoing costs incurred as a result of this.
		clusterctl delete --all --include-crd  --include-namespace`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDelete()
	},
}

func init() {
	deleteCmd.Flags().StringVar(&dd.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	deleteCmd.Flags().StringVar(&dd.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")

	deleteCmd.Flags().BoolVar(&dd.includeNamespace, "include-namespace", false,
		"Forces the deletion of the namespace where the providers are hosted (and of all the contained objects)")
	deleteCmd.Flags().BoolVar(&dd.includeCRDs, "include-crd", false,
		"Forces the deletion of the provider's CRDs (and of all the related objects)")

	deleteCmd.Flags().StringVar(&dd.coreProvider, "core", "",
		"Core provider version (e.g. cluster-api:v0.3.0) to delete from the management cluster")
	deleteCmd.Flags().StringSliceVarP(&dd.infrastructureProviders, "infrastructure", "i", nil,
		"Infrastructure providers and versions (e.g. aws:v0.5.0) to delete from the management cluster")
	deleteCmd.Flags().StringSliceVarP(&dd.bootstrapProviders, "bootstrap", "b", nil,
		"Bootstrap providers and versions (e.g. kubeadm:v0.3.0) to delete from the management cluster")
	deleteCmd.Flags().StringSliceVarP(&dd.controlPlaneProviders, "control-plane", "c", nil,
		"ControlPlane providers and versions (e.g. kubeadm:v0.3.0) to delete from the management cluster")

	deleteCmd.Flags().BoolVar(&dd.deleteAll, "all", false,
		"Force deletion of all the providers")

	RootCmd.AddCommand(deleteCmd)
}

func runDelete() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	hasProviderNames := (dd.coreProvider != "") ||
		(len(dd.bootstrapProviders) > 0) ||
		(len(dd.controlPlaneProviders) > 0) ||
		(len(dd.infrastructureProviders) > 0)

	if dd.deleteAll && hasProviderNames {
		return errors.New("The --all flag can't be used in combination with --core, --bootstrap, --control-plane, --infrastructure")
	}

	if !dd.deleteAll && !hasProviderNames {
		return errors.New("At least one of --core, --bootstrap, --control-plane, --infrastructure should be specified or the --all flag should be set")
	}

	return c.Delete(client.DeleteOptions{
		Kubeconfig:              client.Kubeconfig{Path: dd.kubeconfig, Context: dd.kubeconfigContext},
		IncludeNamespace:        dd.includeNamespace,
		IncludeCRDs:             dd.includeCRDs,
		CoreProvider:            dd.coreProvider,
		BootstrapProviders:      dd.bootstrapProviders,
		InfrastructureProviders: dd.infrastructureProviders,
		ControlPlaneProviders:   dd.controlPlaneProviders,
		DeleteAll:               dd.deleteAll,
	})
}
