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
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client"
)

type configClusterOptions struct {
	kubeconfig             string
	flavor                 string
	infrastructureProvider string

	targetNamespace          string
	kubernetesVersion        string
	controlPlaneMachineCount int
	workerMachineCount       int

	url                string
	configMapNamespace string
	configMapName      string
	configMapDataKey   string

	listVariables bool
}

var cc = &configClusterOptions{}

var configClusterClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Generate templates for creating a Cluster API workload clusters",
	Long: LongDesc(`
		clusterctl ships with a list of well know providers; if necessary, edit
		the $HOME/.cluster-api/clusterctl.yaml file to add new provider configurations or to customize existing ones.
		
		Each provider configuration links to a repository, and clusterctl will fetch the template
		from creating a new cluster from there.`),

	Example: Examples(`
		# Generates a yaml file for creating a Cluster API workload cluster using
		# default infrastructure provider and default bootstrap provider installed in the cluster.
		clusterctl config cluster my-cluster
 
		# Generates a yaml file for creating a Cluster API workload cluster using
		# specified version of the AWS infrastructure provider
		clusterctl config cluster my-cluster --infrastructure=aws:v0.4.1

		# Generates a yaml file for creating a Cluster API workload cluster in the "foo" namespace.
		clusterctl config cluster my-cluster --target-namespace=foo
		
		# Generates a yaml file for creating a Cluster API workload cluster with
		# nodes using Kubernetes v1.16.0
		clusterctl config cluster my-cluster --kubernetes-version=v1.16.0

		# Generates a yaml file for creating a Cluster API workload cluster with
		# custom number of nodes (if supported by provider's templates)
		clusterctl config cluster my-cluster --control-plane-machine-count=3 --worker-machine-count=10

		# Generates a yaml file for creating a Cluster API workload cluster using a template hosted on a ConfigMap
		# instead of using the cluster templates hosted in the provider's repository.
		clusterctl config cluster my-cluster --from-config-map MyTemplates

		# Generates a yaml file for creating a Cluster API workload cluster using a template hosted on specific URL
		# instead of using the cluster templates hosted in the provider's repository.
		clusterctl config cluster my-cluster --from https://github.com/foo-org/foo-repository/blob/master/cluster-template.yaml

		# Generates a yaml file for creating a Cluster API workload cluster using a template hosted on the local file system
		clusterctl config cluster my-cluster --from ~/workspace/cluster-template.yaml`),

	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetClusterTemplate(args[0])
	},
}

func init() {
	configClusterClusterCmd.Flags().StringVarP(&cc.kubeconfig, "kubeconfig", "", "", "Path to the kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig discovery will be used")

	// flags for the template variables
	configClusterClusterCmd.Flags().StringVarP(&cc.targetNamespace, "target-namespace", "n", "", "The namespace where the objects describing the workload cluster should be deployed. If not specified, the current namespace will be used")
	configClusterClusterCmd.Flags().StringVarP(&cc.kubernetesVersion, "kubernetes-version", "", "", "The Kubernetes version to use for the workload cluster. By default (empty), the value from os env variables or the .cluster-api/clusterctl.yaml config file will be used")
	configClusterClusterCmd.Flags().IntVarP(&cc.controlPlaneMachineCount, "control-plane-machine-count", "", 1, "The number of control plane machines to be added to the workload cluster.")
	configClusterClusterCmd.Flags().IntVarP(&cc.workerMachineCount, "worker-machine-count", "", 0, "The number of worker machines to be added to the workload cluster.")

	// flags for the repository source
	configClusterClusterCmd.Flags().StringVarP(&cc.infrastructureProvider, "infrastructure", "i", "", "The infrastructure provider to read the workload cluster template from. By default (empty), the default infrastructure provider will be used if no other source is specified")
	configClusterClusterCmd.Flags().StringVarP(&cc.flavor, "flavor", "f", "", "The workload cluster template variant to be used when reading from the infrastructure provider repository. By default (empty), the default cluster template will be used")

	// flags for the url source
	configClusterClusterCmd.Flags().StringVarP(&cc.url, "from", "", "", "The URL to read the workload cluster template from. By default (empty), the infrastructure provider repository URL will be used")

	// flags for the config map source
	configClusterClusterCmd.Flags().StringVarP(&cc.configMapName, "from-config-map", "", "", "The ConfigMap to read the workload cluster template from. This can be used as alternative to read from the provider repository or from an URL")
	configClusterClusterCmd.Flags().StringVarP(&cc.configMapNamespace, "from-config-map-namespace", "", "", "The namespace where the ConfigMap exists. By default (empty), the current namespace will be used")
	configClusterClusterCmd.Flags().StringVarP(&cc.configMapDataKey, "from-config-map-key", "", "", fmt.Sprintf("The ConfigMap.Data key where the workload cluster template is hosted. By default (empty), %q will be used", client.DefaultCustomTemplateConfigMapKey))

	// other flags
	configClusterClusterCmd.Flags().BoolVarP(&cc.listVariables, "list-variables", "", false, "Returns the list of variables expected by the template instead of the template yaml")

	configCmd.AddCommand(configClusterClusterCmd)
}

func runGetClusterTemplate(name string) error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	templateOptions := client.GetClusterTemplateOptions{
		Kubeconfig:               cc.kubeconfig,
		ClusterName:              name,
		TargetNamespace:          cc.targetNamespace,
		KubernetesVersion:        cc.kubernetesVersion,
		ControlPlaneMachineCount: cc.controlPlaneMachineCount,
		WorkerMachineCount:       cc.workerMachineCount,
		ListVariablesOnly:        cc.listVariables,
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
