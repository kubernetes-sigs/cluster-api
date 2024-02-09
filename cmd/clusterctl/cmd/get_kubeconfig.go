/*
Copyright 2020 The Kubernetes Authors.

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
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type getKubeconfigOptions struct {
	kubeconfig        string
	kubeconfigContext string
	namespace         string
}

var gk = &getKubeconfigOptions{}

var getKubeconfigCmd = &cobra.Command{
	Use:   "kubeconfig NAME",
	Short: "Gets the kubeconfig file for accessing a workload cluster",
	Long: LongDesc(`
		Gets the kubeconfig file for accessing a workload cluster`),

	Example: Examples(`
		# Get the workload cluster's kubeconfig.
		clusterctl get kubeconfig <name of workload cluster>

		# Get the workload cluster's kubeconfig in a particular namespace.
		clusterctl get kubeconfig <name of workload cluster> --namespace foo`),

	Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("please specify a workload cluster name")
		}
		return nil
	},
	RunE: func(_ *cobra.Command, args []string) error {
		return runGetKubeconfig(args[0])
	},
}

func init() {
	getKubeconfigCmd.Flags().StringVarP(&gk.namespace, "namespace", "n", "",
		"Namespace where the workload cluster exist.")
	getKubeconfigCmd.Flags().StringVar(&gk.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	getKubeconfigCmd.Flags().StringVar(&gk.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")

	// completions
	getKubeconfigCmd.ValidArgsFunction = resourceNameCompletionFunc(
		getKubeconfigCmd.Flags().Lookup("kubeconfig"),
		getKubeconfigCmd.Flags().Lookup("kubeconfig-context"),
		getKubeconfigCmd.Flags().Lookup("namespace"),
		clusterv1.GroupVersion.String(),
		"cluster",
	)

	getCmd.AddCommand(getKubeconfigCmd)
}

func runGetKubeconfig(workloadClusterName string) error {
	ctx := context.Background()

	c, err := client.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	options := client.GetKubeconfigOptions{
		Kubeconfig:          client.Kubeconfig{Path: gk.kubeconfig, Context: gk.kubeconfigContext},
		WorkloadClusterName: workloadClusterName,
		Namespace:           gk.namespace,
	}

	out, err := c.GetKubeconfig(ctx, options)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}
