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
)

type deleteOptions struct {
	kubeconfig           string
	targetNamespace      string
	forceDeleteNamespace bool
	forceDeleteCRD       bool
	deleteAll            bool
}

var dd = &deleteOptions{}

var deleteCmd = &cobra.Command{
	Use:   "delete [providers]",
	Short: "Deletes one or more providers from the management cluster",
	Long: LongDesc(`
		Deletes one or more providers from the management cluster.`),

	Example: Examples(`
		# Deletes the AWS provider
		# Please note that this implies the deletion of all provider components except the hosting namespace
		# and the CRDs.
		clusterctl delete aws
 
		# Deletes the instance of the AWS provider hosted in the "foo" namespace
		# Please note, if there are multiple instances of the AWS provider installed in the cluster,
		# global/shared resources (e.g. ClusterRoles), are not deleted in order to preserve
		# the functioning of the remaining instances.
		clusterctl delete aws --namespace=foo

		# Deletes all the providers
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# Cluster API Providers are orphaned and there might be ongoing costs incurred as a result of this.
		clusterctl delete --all

		# Delete the AWS provider and related CRDs. Please note that this forces deletion of 
		# all the related objects (e.g. AWSClusters, AWSMachines etc.).
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# the AWS infrastructure provider are orphaned and there might be ongoing costs incurred as a result of this.
		clusterctl delete aws --delete-crd

		# Delete the AWS provider and its hosting Namespace. Please note that this forces deletion of 
		# all objects existing in the namespace. 
		# Important! As a consequence of this operation, all the corresponding resources managed by
		# Cluster API Providers are orphaned and there might be ongoing costs incurred as a result of this.
		clusterctl delete aws --delete-namespace

		# Reset the management cluster to its original state
		# Important! As a consequence of this operation all the corresponding resources on target clouds
		# are "orphaned" and thus there may be ongoing costs incurred as a result of this.
		clusterctl delete --all --delete-crd  --delete-namespace`),

	RunE: func(cmd *cobra.Command, args []string) error {
		if dd.deleteAll && len(args) > 0 {
			return errors.New("The --all flag can't be used in combination with the list of providers")
		}

		if !dd.deleteAll && len(args) == 0 {
			return errors.New("At least one provider should be specified or the --all flag should be set")
		}

		return runDelete(args)
	},
}

func init() {
	deleteCmd.Flags().StringVarP(&dd.kubeconfig, "kubeconfig", "", "", "Path to the kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig discovery will be used")
	deleteCmd.Flags().StringVarP(&dd.targetNamespace, "namespace", "", "", "The namespace where the provider to be deleted lives. If not specified, the namespace name will be inferred from the current configuration")

	deleteCmd.Flags().BoolVarP(&dd.forceDeleteNamespace, "delete-namespace", "n", false, "Forces the deletion of the namespace where the providers are hosted (and of all the contained objects)")
	deleteCmd.Flags().BoolVarP(&dd.forceDeleteCRD, "delete-crd", "c", false, "Forces the deletion of the provider's CRDs (and of all the related objects)")
	deleteCmd.Flags().BoolVarP(&dd.deleteAll, "all", "", false, "Force deletion of all the providers")

	RootCmd.AddCommand(deleteCmd)
}

func runDelete(args []string) error {
	return nil
}
