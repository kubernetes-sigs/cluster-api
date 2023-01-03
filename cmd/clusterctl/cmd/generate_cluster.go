/*
Copyright 2021 The Kubernetes Authors.

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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type generateClusterOptions struct {
	kubeconfig             string
	kubeconfigContext      string
	flavor                 string
	infrastructureProvider string

	targetNamespace          string
	kubernetesVersion        string
	controlPlaneMachineCount int64
	workerMachineCount       int64

	url                string
	configMapNamespace string
	configMapName      string
	configMapDataKey   string

	listVariables bool

	output string
}

var gc = &generateClusterOptions{}

var generateClusterClusterCmd = &cobra.Command{
	Use:   "cluster NAME",
	Short: "Generate templates for creating workload clusters",
	Long: LongDesc(`
		Generate templates for creating workload clusters.

		clusterctl ships with a list of known providers; if necessary, edit
		$HOME/.cluster-api/clusterctl.yaml to add new provider or to customize existing ones.

		Each provider configuration links to a repository; clusterctl uses this information
		to fetch templates when creating a new cluster.`),

	Example: Examples(`
		# Generates a yaml file for creating workload clusters using
		# the pre-installed infrastructure and bootstrap providers.
		clusterctl generate cluster my-cluster

		# Generates a yaml file for creating workload clusters using
		# a specific version of the AWS infrastructure provider.
		clusterctl generate cluster my-cluster --infrastructure=aws:v0.4.1

		# Generates a yaml file for creating workload clusters in a custom namespace.
		clusterctl generate cluster my-cluster --target-namespace=foo

		# Generates a yaml file for creating workload clusters with a specific Kubernetes version.
		clusterctl generate cluster my-cluster --kubernetes-version=v1.19.1

		# Generates a yaml file for creating workload clusters with a
		# custom number of nodes (if supported by the provider's templates).
		clusterctl generate cluster my-cluster --control-plane-machine-count=3 --worker-machine-count=10

		# Generates a yaml file for creating workload clusters using a template stored in a ConfigMap.
		clusterctl generate cluster my-cluster --from-config-map MyTemplates

		# Generates a yaml file for creating workload clusters using a template from a specific URL.
		clusterctl generate cluster my-cluster --from https://github.com/foo-org/foo-repository/blob/main/cluster-template.yaml

		# Generates a yaml file for creating workload clusters using a template stored locally.
		clusterctl generate cluster my-cluster --from ~/workspace/cluster-template.yaml

		# Prints the list of variables required by the yaml file for creating workload cluster.
		clusterctl generate cluster my-cluster --list-variables`),

	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("please specify a cluster name")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenerateClusterTemplate(cmd, args[0])
	},
}

func init() {
	generateClusterClusterCmd.Flags().StringVar(&gc.kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig file to use for the management cluster. If empty, default discovery rules apply.")
	generateClusterClusterCmd.Flags().StringVar(&gc.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")

	// flags for the template variables
	generateClusterClusterCmd.Flags().StringVarP(&gc.targetNamespace, "target-namespace", "n", "",
		"The namespace to use for the workload cluster. If unspecified, the current namespace will be used.")
	generateClusterClusterCmd.Flags().StringVar(&gc.kubernetesVersion, "kubernetes-version", "",
		"The Kubernetes version to use for the workload cluster. If unspecified, the value from OS environment variables or the .cluster-api/clusterctl.yaml config file will be used.")
	generateClusterClusterCmd.Flags().Int64Var(&gc.controlPlaneMachineCount, "control-plane-machine-count", 1,
		"The number of control plane machines for the workload cluster.")
	generateClusterClusterCmd.Flags().Int64Var(&gc.workerMachineCount, "worker-machine-count", 0,
		"The number of worker machines for the workload cluster.")

	// flags for the repository source
	generateClusterClusterCmd.Flags().StringVarP(&gc.infrastructureProvider, "infrastructure", "i", "",
		"The infrastructure provider to read the workload cluster template from. If unspecified, the default infrastructure provider will be used.")
	generateClusterClusterCmd.Flags().StringVarP(&gc.flavor, "flavor", "f", "",
		"The workload cluster template variant to be used when reading from the infrastructure provider repository. If unspecified, the default cluster template will be used.")

	// flags for the url source
	generateClusterClusterCmd.Flags().StringVar(&gc.url, "from", "",
		"The URL to read the workload cluster template from. If unspecified, the infrastructure provider repository URL will be used. If set to '-', the workload cluster template is read from stdin.")

	// flags for the config map source
	generateClusterClusterCmd.Flags().StringVar(&gc.configMapName, "from-config-map", "",
		"The ConfigMap to read the workload cluster template from. This can be used as alternative to read from the provider repository or from an URL")
	generateClusterClusterCmd.Flags().StringVar(&gc.configMapNamespace, "from-config-map-namespace", "",
		"The namespace where the ConfigMap exists. If unspecified, the current namespace will be used")
	generateClusterClusterCmd.Flags().StringVar(&gc.configMapDataKey, "from-config-map-key", "",
		fmt.Sprintf("The ConfigMap.Data key where the workload cluster template is hosted. If unspecified, %q will be used", client.DefaultCustomTemplateConfigMapKey))

	// other flags
	generateClusterClusterCmd.Flags().BoolVar(&gc.listVariables, "list-variables", false,
		"Returns the list of variables expected by the template instead of the template yaml")
	generateClusterClusterCmd.Flags().StringVar(&gc.output, "write-to", "", "Specify the output file to write the template to, defaults to STDOUT if the flag is not set")

	generateCmd.AddCommand(generateClusterClusterCmd)
}

func runGenerateClusterTemplate(cmd *cobra.Command, name string) error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	templateOptions := client.GetClusterTemplateOptions{
		Kubeconfig:        client.Kubeconfig{Path: gc.kubeconfig, Context: gc.kubeconfigContext},
		ClusterName:       name,
		TargetNamespace:   gc.targetNamespace,
		KubernetesVersion: gc.kubernetesVersion,
		ListVariablesOnly: gc.listVariables,
	}

	if cmd.Flags().Changed("control-plane-machine-count") {
		templateOptions.ControlPlaneMachineCount = &gc.controlPlaneMachineCount
	}
	if cmd.Flags().Changed("worker-machine-count") {
		templateOptions.WorkerMachineCount = &gc.workerMachineCount
	}

	if gc.url != "" {
		templateOptions.URLSource = &client.URLSourceOptions{
			URL: gc.url,
		}
	}

	if gc.configMapNamespace != "" || gc.configMapName != "" || gc.configMapDataKey != "" {
		templateOptions.ConfigMapSource = &client.ConfigMapSourceOptions{
			Namespace: gc.configMapNamespace,
			Name:      gc.configMapName,
			DataKey:   gc.configMapDataKey,
		}
	}

	if gc.infrastructureProvider != "" || gc.flavor != "" {
		templateOptions.ProviderRepositorySource = &client.ProviderRepositorySourceOptions{
			InfrastructureProvider: gc.infrastructureProvider,
			Flavor:                 gc.flavor,
		}
	}

	template, err := c.GetClusterTemplate(templateOptions)
	if err != nil {
		return err
	}

	if gc.listVariables {
		return printVariablesOutput(template, templateOptions)
	}

	return printYamlOutput(template, gc.output)
}
