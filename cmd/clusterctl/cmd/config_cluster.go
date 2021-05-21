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
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

/*
NOTE: This command is deprecated in favor of `clusterctl generate cluster`. The source code is located at `cmd/clusterctl/cmd/generate_cluster.go`.
This file will be removed in 0.5.x. Do not make any changes to this file.
*/

type configClusterOptions struct {
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
}

var cc = &configClusterOptions{}

var configClusterClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Generate templates for creating workload clusters.",
	Long: LongDesc(`
		Generate templates for creating workload clusters.

		clusterctl ships with a list of known providers; if necessary, edit
		$HOME/.cluster-api/clusterctl.yaml to add new provider or to customize existing ones.

		Each provider configuration links to a repository; clusterctl uses this information
		to fetch templates when creating a new cluster.`),

	Example: Examples(`
		# Generates a configuration file for creating workload clusters using
		# the pre-installed infrastructure and bootstrap providers.
		clusterctl config cluster my-cluster

		# Generates a configuration file for creating workload clusters using
		# a specific version of the AWS infrastructure provider.
		clusterctl config cluster my-cluster --infrastructure=aws:v0.4.1

		# Generates a configuration file for creating workload clusters in a custom namespace.
		clusterctl config cluster my-cluster --target-namespace=foo

		# Generates a configuration file for creating workload clusters with a specific Kubernetes version.
		clusterctl config cluster my-cluster --kubernetes-version=v1.19.1

		# Generates a configuration file for creating workload clusters with a
		# custom number of nodes (if supported by the provider's templates).
		clusterctl config cluster my-cluster --control-plane-machine-count=3 --worker-machine-count=10

		# Generates a configuration file for creating workload clusters using a template stored in a ConfigMap.
		clusterctl config cluster my-cluster --from-config-map MyTemplates

		# Generates a configuration file for creating workload clusters using a template from a specific URL.
		clusterctl config cluster my-cluster --from https://github.com/foo-org/foo-repository/blob/master/cluster-template.yaml

		# Generates a configuration file for creating workload clusters using a template stored locally.
		clusterctl config cluster my-cluster --from ~/workspace/cluster-template.yaml`),

	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetClusterTemplate(cmd, args[0])
	},
	Deprecated: "use `clusterctl generate cluster` instead",
}

func init() {
	configClusterClusterCmd.Flags().StringVar(&cc.kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig file to use for the management cluster. If empty, default discovery rules apply.")
	configClusterClusterCmd.Flags().StringVar(&cc.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")

	// flags for the template variables
	configClusterClusterCmd.Flags().StringVarP(&cc.targetNamespace, "target-namespace", "n", "",
		"The namespace to use for the workload cluster. If unspecified, the current namespace will be used.")
	configClusterClusterCmd.Flags().StringVar(&cc.kubernetesVersion, "kubernetes-version", "",
		"The Kubernetes version to use for the workload cluster. If unspecified, the value from OS environment variables or the .cluster-api/clusterctl.yaml config file will be used.")
	configClusterClusterCmd.Flags().Int64Var(&cc.controlPlaneMachineCount, "control-plane-machine-count", 1,
		"The number of control plane machines for the workload cluster.")
	configClusterClusterCmd.Flags().Int64Var(&cc.workerMachineCount, "worker-machine-count", 0,
		"The number of worker machines for the workload cluster.")

	// flags for the repository source
	configClusterClusterCmd.Flags().StringVarP(&cc.infrastructureProvider, "infrastructure", "i", "",
		"The infrastructure provider to read the workload cluster template from. If unspecified, the default infrastructure provider will be used.")
	configClusterClusterCmd.Flags().StringVarP(&cc.flavor, "flavor", "f", "",
		"The workload cluster template variant to be used when reading from the infrastructure provider repository. If unspecified, the default cluster template will be used.")

	// flags for the url source
	configClusterClusterCmd.Flags().StringVar(&cc.url, "from", "",
		"The URL to read the workload cluster template from. If unspecified, the infrastructure provider repository URL will be used")

	// flags for the config map source
	configClusterClusterCmd.Flags().StringVar(&cc.configMapName, "from-config-map", "",
		"The ConfigMap to read the workload cluster template from. This can be used as alternative to read from the provider repository or from an URL")
	configClusterClusterCmd.Flags().StringVar(&cc.configMapNamespace, "from-config-map-namespace", "",
		"The namespace where the ConfigMap exists. If unspecified, the current namespace will be used")
	configClusterClusterCmd.Flags().StringVar(&cc.configMapDataKey, "from-config-map-key", "",
		fmt.Sprintf("The ConfigMap.Data key where the workload cluster template is hosted. If unspecified, %q will be used", client.DefaultCustomTemplateConfigMapKey))

	// other flags
	configClusterClusterCmd.Flags().BoolVar(&cc.listVariables, "list-variables", false,
		"Returns the list of variables expected by the template instead of the template yaml")

	configCmd.AddCommand(configClusterClusterCmd)
}

func runGetClusterTemplate(cmd *cobra.Command, name string) error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	templateOptions := client.GetClusterTemplateOptions{
		Kubeconfig:        client.Kubeconfig{Path: cc.kubeconfig, Context: cc.kubeconfigContext},
		ClusterName:       name,
		TargetNamespace:   cc.targetNamespace,
		KubernetesVersion: cc.kubernetesVersion,
		ListVariablesOnly: cc.listVariables,
	}

	if cmd.Flags().Changed("control-plane-machine-count") {
		templateOptions.ControlPlaneMachineCount = &cc.controlPlaneMachineCount
	}
	if cmd.Flags().Changed("worker-machine-count") {
		templateOptions.WorkerMachineCount = &cc.workerMachineCount
	}

	if cc.url != "" {
		templateOptions.URLSource = &client.URLSourceOptions{
			URL: cc.url,
		}
	}

	if cc.configMapNamespace != "" || cc.configMapName != "" || cc.configMapDataKey != "" {
		templateOptions.ConfigMapSource = &client.ConfigMapSourceOptions{
			Namespace: cc.configMapNamespace,
			Name:      cc.configMapName,
			DataKey:   cc.configMapDataKey,
		}
	}

	if cc.infrastructureProvider != "" || cc.flavor != "" {
		templateOptions.ProviderRepositorySource = &client.ProviderRepositorySourceOptions{
			InfrastructureProvider: cc.infrastructureProvider,
			Flavor:                 cc.flavor,
		}
	}

	template, err := c.GetClusterTemplate(templateOptions)
	if err != nil {
		return err
	}

	if cc.listVariables {
		return templateListVariablesOutput(template)
	}

	return templateYAMLOutput(template)
}

func templateListVariablesOutput(template client.Template) error {
	if len(template.Variables()) > 0 {
		fmt.Println("Variables:")
		for _, v := range template.Variables() {
			fmt.Printf("  - %s\n", v)
		}
	}
	fmt.Println()
	return nil
}

func templateYAMLOutput(template client.Template) error {
	yaml, err := template.Yaml()
	if err != nil {
		return err
	}
	yaml = append(yaml, '\n')

	if _, err := os.Stdout.Write(yaml); err != nil {
		return errors.Wrap(err, "failed to write yaml to Stdout")
	}
	return nil
}
