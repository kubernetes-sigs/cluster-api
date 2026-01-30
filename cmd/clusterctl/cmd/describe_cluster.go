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
	"context"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd/internal/templates"
	cmdtree "sigs.k8s.io/cluster-api/internal/util/tree"
)

type describeClusterOptions struct {
	kubeconfig              string
	kubeconfigContext       string
	namespace               string
	showOtherConditions     string
	showMachineSets         bool
	showClusterResourceSets bool
	showTemplates           bool
	echo                    bool
	grouping                bool
	v1beta2                 bool
	color                   bool
}

var dc = &describeClusterOptions{}

var describeClusterClusterCmd = &cobra.Command{
	Use:   "cluster NAME",
	Short: "Describe workload clusters",
	Long: templates.LongDesc(`
		Provide an "at glance" view of a Cluster API cluster designed to help the user in quickly
		understanding if there are problems and where.
		.`),

	Example: templates.Examples(`
		# Describe the cluster named test-1.
		clusterctl describe cluster test-1

		# Describe the cluster named test-1 showing all the conditions for the KubeadmControlPlane object kind.
		clusterctl describe cluster test-1 --show-conditions KubeadmControlPlane

		# Describe the cluster named test-1 showing all the conditions for a specific machine.
		clusterctl describe cluster test-1 --show-conditions Machine/m1

		# Describe the cluster named test-1 disabling automatic grouping of objects with the same ready condition
		# e.g. un-group all the machines with Ready=true instead of showing a single group node.
		clusterctl describe cluster test-1 --grouping=false

		# Describe the cluster named test-1 showing the MachineInfrastructure and BootstrapConfig objects
		# also when their status is the same as the status of the corresponding machine object.
		clusterctl describe cluster test-1 --echo`),

	Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("please specify a cluster name")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDescribeCluster(cmd, args[0])
	},
}

func init() {
	describeClusterClusterCmd.Flags().StringVar(&dc.kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig file to use for the management cluster. If empty, default discovery rules apply.")
	describeClusterClusterCmd.Flags().StringVar(&dc.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	describeClusterClusterCmd.Flags().StringVarP(&dc.namespace, "namespace", "n", "",
		"The namespace where the workload cluster is located. If unspecified, the current namespace will be used.")

	describeClusterClusterCmd.Flags().StringVar(&dc.showOtherConditions, "show-conditions", "",
		fmt.Sprintf("list of comma separated kind or kind/name for which the command should show all the object's conditions (use 'all' to show conditions for everything, use the %s suffix to show only non-zero conditions).", tree.ShowNonZeroConditionsSuffix))
	describeClusterClusterCmd.Flags().BoolVar(&dc.showMachineSets, "show-machinesets", false,
		"Show MachineSet objects.")
	describeClusterClusterCmd.Flags().BoolVar(&dc.showClusterResourceSets, "show-resourcesets", false,
		"Show cluster resource sets.")
	describeClusterClusterCmd.Flags().BoolVar(&dc.showTemplates, "show-templates", false,
		"Show infrastructure and bootstrap config templates associated with the cluster.")

	describeClusterClusterCmd.Flags().BoolVar(&dc.echo, "echo", false, ""+
		"Show MachineInfrastructure and BootstrapConfig when ready condition is true or it has the Status, Severity and Reason of the machine's object.")
	describeClusterClusterCmd.Flags().BoolVar(&dc.grouping, "grouping", true,
		"Groups machines when ready condition has the same Status, Severity and Reason.")
	describeClusterClusterCmd.Flags().BoolVar(&dc.v1beta2, "v1beta2", true,
		"Use V1Beta2 conditions..")
	_ = describeClusterClusterCmd.Flags().MarkDeprecated("v1beta2",
		"this field will be removed when v1beta1 will be dropped.")
	describeClusterClusterCmd.Flags().BoolVarP(&dc.color, "color", "c", false, "Enable or disable color output; if not set color is enabled by default only if using tty. The flag is overridden by the NO_COLOR env variable if set.")

	// completions
	describeClusterClusterCmd.ValidArgsFunction = resourceNameCompletionFunc(
		describeClusterClusterCmd.Flags().Lookup("kubeconfig"),
		describeClusterClusterCmd.Flags().Lookup("kubeconfig-context"),
		describeClusterClusterCmd.Flags().Lookup("namespace"),
		clusterv1.GroupVersion.String(),
		"cluster",
	)

	describeCmd.AddCommand(describeClusterClusterCmd)
}

func runDescribeCluster(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	c, err := client.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	tree, err := c.DescribeCluster(ctx, client.DescribeClusterOptions{
		Kubeconfig:              client.Kubeconfig{Path: dc.kubeconfig, Context: dc.kubeconfigContext},
		Namespace:               dc.namespace,
		ClusterName:             name,
		ShowOtherConditions:     dc.showOtherConditions,
		ShowClusterResourceSets: dc.showClusterResourceSets,
		ShowTemplates:           dc.showTemplates,
		ShowMachineSets:         dc.showMachineSets,
		AddTemplateVirtualNode:  true,
		Echo:                    dc.echo,
		Grouping:                dc.grouping,
		V1Beta1:                 !dc.v1beta2,
	})
	if err != nil {
		return err
	}

	if cmd.Flags().Changed("color") {
		color.NoColor = !dc.color
	}

	switch dc.v1beta2 {
	case true:
		if err := cmdtree.PrintObjectTree(tree, os.Stdout); err != nil {
			return errors.Wrap(err, "failed to print object tree")
		}
	default:
		if err := cmdtree.PrintObjectTreeV1Beta1(tree); err != nil {
			return errors.Wrap(err, "failed to print object tree v1beta1")
		}
	}

	return nil
}
