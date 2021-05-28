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

package rollout

import (
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

// undoOptions is the start of the data required to perform the operation.
type undoOptions struct {
	kubeconfig        string
	kubeconfigContext string
	resources         []string
	namespace         string
	toRevision        int64
}

var undoOpt = &undoOptions{}

var (
	undoLong = templates.LongDesc(`
		Rollback to a previous rollout.`)

	undoExample = templates.Examples(`
		# Rollback to the previous deployment
		clusterctl alpha rollout undo machinedeployment/my-md-0

		# Rollback to previous machinedeployment --to-revision=3
		clusterctl alpha rollout undo machinedeployment/my-md-0 --to-revision=3`)
)

// NewCmdRolloutUndo returns a Command instance for 'rollout undo' sub command.
func NewCmdRolloutUndo(cfgFile string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "undo RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 "Undo a cluster-api resource",
		Long:                  undoLong,
		Example:               undoExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUndo(cfgFile, args)
		},
	}
	cmd.Flags().StringVar(&undoOpt.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	cmd.Flags().StringVar(&undoOpt.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	cmd.Flags().StringVar(&undoOpt.namespace, "namespace", "", "Namespace where the resource(s) reside. If unspecified, the defult namespace will be used.")
	cmd.Flags().Int64Var(&undoOpt.toRevision, "to-revision", undoOpt.toRevision, "The revision to rollback to. Default to 0 (last revision).")

	return cmd
}

func runUndo(cfgFile string, args []string) error {
	undoOpt.resources = args

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	return c.RolloutUndo(client.RolloutOptions{
		Kubeconfig: client.Kubeconfig{Path: undoOpt.kubeconfig, Context: undoOpt.kubeconfigContext},
		Namespace:  undoOpt.namespace,
		Resources:  undoOpt.resources,
		ToRevision: undoOpt.toRevision,
	})
}
