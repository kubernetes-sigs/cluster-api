/*
Copyright 2022 The Kubernetes Authors.

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
	"context"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

var (
	historyLong = templates.LongDesc(`
		View previous rollout revisions and configurations.`)

	historyExample = templates.Examples(`
		# View the rollout history of a machine deployment
		clusterctl alpha rollout history machinedeployment/my-md-0

		# View the details of machine deployment revision 3
		clusterctl alpha rollout history machinedeployment/my-md-0 --revision=3`)
)

// historyOptions is the start of the data required to perform the operation.
type historyOptions struct {
	kubeconfig        string
	kubeconfigContext string
	resources         []string
	namespace         string
	revision          int64
}

var historyOpt = historyOptions{}

// NewCmdRolloutHistory returns a Command instance for 'rollout history' sub command.
func NewCmdRolloutHistory(cfgFile string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "history RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 "History of a cluster-api resource",
		Long:                  historyLong,
		Example:               historyExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(cfgFile, args)
		},
	}
	cmd.Flags().StringVar(&historyOpt.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	cmd.Flags().StringVar(&historyOpt.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	cmd.Flags().StringVarP(&historyOpt.namespace, "namespace", "n", "", "Namespace where the resource(s) reside. If unspecified, the defult namespace will be used.")
	cmd.Flags().Int64Var(&historyOpt.revision, "revision", historyOpt.revision, "See the details, including machineTemplate of the revision specified.")

	return cmd
}

func runHistory(cfgFile string, args []string) error {
	historyOpt.resources = args

	ctx := context.Background()

	c, err := client.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	return c.RolloutHistory(ctx, client.RolloutHistoryOptions{
		Kubeconfig: client.Kubeconfig{Path: historyOpt.kubeconfig, Context: historyOpt.kubeconfigContext},
		Namespace:  historyOpt.namespace,
		Resources:  historyOpt.resources,
		Revision:   historyOpt.revision,
	})
}
