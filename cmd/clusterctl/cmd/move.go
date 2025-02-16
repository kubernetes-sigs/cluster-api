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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd/internal/templates"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/util/apiwarnings"
)

type moveOptions struct {
	fromKubeconfig        string
	fromKubeconfigContext string
	toKubeconfig          string
	toKubeconfigContext   string
	namespace             string
	fromDirectory         string
	toDirectory           string
	dryRun                bool
	hideAPIWarnings       string
}

var mo = &moveOptions{}

var moveCmd = &cobra.Command{
	Use:     "move",
	GroupID: groupManagement,
	Short:   "Move Cluster API objects and all dependencies between management clusters",
	Long: templates.LongDesc(`
		Move Cluster API objects and all dependencies between management clusters.

		Note: The destination cluster MUST have the required provider components installed.`),

	Example: templates.Examples(`
		Move Cluster API objects and all dependencies between management clusters.
		clusterctl move --to-kubeconfig=target-kubeconfig.yaml

		Write Cluster API objects and all dependencies from a management cluster to directory.
		clusterctl move --to-directory /tmp/backup-directory

		Read Cluster API objects and all dependencies from a directory into a management cluster.
		clusterctl move --from-directory /tmp/backup-directory
	`),
	Args: cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		return runMove()
	},
}

func init() {
	moveCmd.Flags().StringVar(&mo.fromKubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file for the source management cluster. If unspecified, default discovery rules apply.")
	moveCmd.Flags().StringVar(&mo.toKubeconfig, "to-kubeconfig", "",
		"Path to the kubeconfig file to use for the destination management cluster.")
	moveCmd.Flags().StringVar(&mo.fromKubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file for the source management cluster. If empty, current context will be used.")
	moveCmd.Flags().StringVar(&mo.toKubeconfigContext, "to-kubeconfig-context", "",
		"Context to be used within the kubeconfig file for the destination management cluster. If empty, current context will be used.")
	moveCmd.Flags().StringVarP(&mo.namespace, "namespace", "n", "",
		"The namespace where the workload cluster is hosted. If unspecified, the current context's namespace is used.")
	moveCmd.Flags().BoolVar(&mo.dryRun, "dry-run", false,
		"Enable dry run, don't really perform the move actions")
	moveCmd.Flags().StringVar(&mo.toDirectory, "to-directory", "",
		"Write Cluster API objects and all dependencies from a management cluster to directory.")
	moveCmd.Flags().StringVar(&mo.fromDirectory, "from-directory", "",
		"Read Cluster API objects and all dependencies from a directory into a management cluster.")
	moveCmd.Flags().StringVar(&mo.hideAPIWarnings, "hide-api-warnings", "default",
		"Set of API server warnings to hide. Valid sets are \"default\" (includes metadata.finalizer warnings), \"all\" , and \"none\".")

	moveCmd.MarkFlagsMutuallyExclusive("to-directory", "to-kubeconfig")
	moveCmd.MarkFlagsMutuallyExclusive("from-directory", "to-directory")
	moveCmd.MarkFlagsMutuallyExclusive("from-directory", "kubeconfig")

	RootCmd.AddCommand(moveCmd)
}

func runMove() error {
	ctx := context.Background()

	if mo.toDirectory == "" &&
		mo.fromDirectory == "" &&
		mo.toKubeconfig == "" &&
		!mo.dryRun {
		return errors.New("please specify a target cluster using the --to-kubeconfig flag when not using --dry-run, --to-directory or --from-directory")
	}

	configClient, err := config.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	clientOptions := []client.Option{}

	var warningHandler rest.WarningHandler
	switch mo.hideAPIWarnings {
	case "all":
		// Hide all warnings.
		warningHandler = apiwarnings.DiscardAllHandler
	case "default":
		// Hide only the default set of warnings.
		warningHandler = apiwarnings.DefaultHandler(logf.Log.WithName("API Server Warning"))
	case "none":
		// Hide no warnings.
		warningHandler = apiwarnings.LogAllHandler(logf.Log.WithName("API Server Warning"))
	default:
		return fmt.Errorf(
			"set of API warnings %q is unknown; choose \"default\", \"all\", or \"none\"",
			mo.hideAPIWarnings,
		)
	}

	if warningHandler != nil {
		clientOptions = append(clientOptions,
			client.InjectClusterClientFactory(
				func(input client.ClusterClientFactoryInput) (cluster.Client, error) {
					return cluster.New(
						cluster.Kubeconfig(input.Kubeconfig),
						configClient,
						cluster.InjectYamlProcessor(input.Processor),
						cluster.InjectProxy(
							cluster.NewProxy(
								cluster.Kubeconfig(input.Kubeconfig),
								cluster.InjectWarningHandler(
									warningHandler,
								),
							)),
					), nil
				},
			),
			// Ensure that the same configClient used by both the client constructor, and the cluster client factory.
			client.InjectConfig(configClient),
		)
	}

	c, err := client.New(ctx, cfgFile, clientOptions...)
	if err != nil {
		return err
	}

	return c.Move(ctx, client.MoveOptions{
		FromKubeconfig: client.Kubeconfig{Path: mo.fromKubeconfig, Context: mo.fromKubeconfigContext},
		ToKubeconfig:   client.Kubeconfig{Path: mo.toKubeconfig, Context: mo.toKubeconfigContext},
		FromDirectory:  mo.fromDirectory,
		ToDirectory:    mo.toDirectory,
		Namespace:      mo.namespace,
		DryRun:         mo.dryRun,
	})
}
