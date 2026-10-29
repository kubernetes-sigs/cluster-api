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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"

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
	watch                   bool
	watchMode               string
	watchInterval           time.Duration
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
		clusterctl describe cluster test-1 --echo

		# Describe the cluster named test-1 and refresh the output every time one of its objects changes.
		clusterctl describe cluster test-1 --watch

		# Describe the cluster named test-1 and refresh the output every 10 seconds.
		clusterctl describe cluster test-1 --watch --watch-mode poll --watch-interval 10s`),

	Args: func(cmd *cobra.Command, args []string) error {
		if err := exactArgsWithMessage(1, "please specify a cluster name")(cmd, args); err != nil {
			return err
		}
		if err := validateShowConditions(dc.showOtherConditions); err != nil {
			return err
		}
		return validateWatch(dc.watchMode, dc.watchInterval)
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

	describeClusterClusterCmd.Flags().BoolVarP(&dc.watch, "watch", "w", false,
		"Watch the cluster and refresh the output when it changes, until interrupted.")
	describeClusterClusterCmd.Flags().StringVar(&dc.watchMode, "watch-mode", string(client.WatchClusterModeInformer),
		fmt.Sprintf("How --watch detects changes: %q refreshes when an object of the cluster changes (requires list and watch permissions), %q refreshes every --watch-interval.", client.WatchClusterModeInformer, client.WatchClusterModePoll))
	describeClusterClusterCmd.Flags().DurationVar(&dc.watchInterval, "watch-interval", 5*time.Second,
		fmt.Sprintf("Interval between refreshes when using --watch-mode=%s.", client.WatchClusterModePoll))

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

func validateShowConditions(showConditions string) error {
	for _, filter := range strings.Split(showConditions, ",") {
		if strings.TrimSpace(filter) != filter {
			return pkgerrors.Errorf("invalid --show-conditions value %q: whitespace is not allowed", showConditions)
		}
	}
	return nil
}

func validateWatch(watchMode string, watchInterval time.Duration) error {
	switch client.WatchClusterMode(watchMode) {
	case client.WatchClusterModeInformer, client.WatchClusterModePoll:
	default:
		return pkgerrors.Errorf("invalid --watch-mode value %q: must be one of %q or %q", watchMode, client.WatchClusterModeInformer, client.WatchClusterModePoll)
	}
	if watchInterval <= 0 {
		return pkgerrors.Errorf("invalid --watch-interval value %s: must be greater than zero", watchInterval)
	}
	return nil
}

func runDescribeCluster(cmd *cobra.Command, name string) error {
	ctx := context.Background()

	c, err := client.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	options := client.DescribeClusterOptions{
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
	}

	if cmd.Flags().Changed("color") {
		color.NoColor = !dc.color
	}

	if dc.watch {
		return watchDescribeCluster(ctx, c, options)
	}

	objectTree, err := c.DescribeCluster(ctx, options)
	if err != nil {
		return err
	}

	return printObjectTree(objectTree, os.Stdout)
}

func printObjectTree(objectTree *tree.ObjectTree, w io.Writer) error {
	switch dc.v1beta2 {
	case true:
		if err := cmdtree.PrintObjectTree(objectTree, w); err != nil {
			return pkgerrors.Wrap(err, "failed to print object tree")
		}
	default:
		if err := cmdtree.PrintObjectTreeV1Beta1ToWriter(objectTree, w); err != nil {
			return pkgerrors.Wrap(err, "failed to print object tree v1beta1")
		}
	}

	return nil
}

func watchDescribeCluster(ctx context.Context, c client.Client, options client.DescribeClusterOptions) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// When writing to a terminal, redraw the object tree in place, like watch(1);
	// otherwise, e.g. when redirecting to a file, append each refresh to the output.
	clearScreen := term.IsTerminal(int(os.Stdout.Fd()))
	refreshes := 0

	return c.WatchCluster(ctx, client.WatchClusterOptions{
		DescribeClusterOptions: options,
		Mode:                   client.WatchClusterMode(dc.watchMode),
		Interval:               dc.watchInterval,
		OnChange: func(objectTree *tree.ObjectTree, err error) error {
			// Render to a buffer first, so the screen is redrawn at once without flickering.
			buf := &bytes.Buffer{}
			switch {
			case clearScreen:
				buf.WriteString("\033[H\033[2J")
			case refreshes > 0:
				buf.WriteString("\n")
			}
			refreshes++

			fmt.Fprintf(buf, "%s: clusterctl describe cluster %s    %s\n\n", watchHeader(), options.ClusterName, time.Now().Format(time.DateTime))
			if err != nil {
				fmt.Fprintf(buf, "Error: %v\n", err)
			} else if err := printObjectTree(objectTree, buf); err != nil {
				return err
			}

			_, err = os.Stdout.Write(buf.Bytes())
			return err
		},
	})
}

// watchHeader describes when the output is refreshed, e.g. "Every 5s".
func watchHeader() string {
	if client.WatchClusterMode(dc.watchMode) == client.WatchClusterModePoll {
		return fmt.Sprintf("Every %s", dc.watchInterval)
	}
	return "On change"
}
