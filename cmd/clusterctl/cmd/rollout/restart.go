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
	"context"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

// restartOptions is the start of the data required to perform the operation.
type restartOptions struct {
	kubeconfig        string
	kubeconfigContext string
	resources         []string
	namespace         string
}

var restartOpt = &restartOptions{}

var (
	restartLong = templates.LongDesc(`
		Restart of cluster-api resources.

	        Resources will be rollout restarted.`)

	restartExample = templates.Examples(`
		# Restart a machinedeployment
		clusterctl alpha rollout restart machinedeployment/my-md-0

		# Restart a kubeadmcontrolplane
		clusterctl alpha rollout restart kubeadmcontrolplane/my-kcp`)
)

// NewCmdRolloutRestart returns a Command instance for 'rollout restart' sub command.
func NewCmdRolloutRestart(cfgFile string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "restart RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 "Restart a cluster-api resource",
		Long:                  restartLong,
		Example:               restartExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestart(cfgFile, cmd, args)
		},
	}
	cmd.Flags().StringVar(&restartOpt.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	cmd.Flags().StringVar(&restartOpt.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	cmd.Flags().StringVarP(&restartOpt.namespace, "namespace", "n", "", "Namespace where the resource(s) reside. If unspecified, the defult namespace will be used.")

	return cmd
}

func runRestart(cfgFile string, _ *cobra.Command, args []string) error {
	restartOpt.resources = args

	ctx := context.Background()

	c, err := client.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	return c.RolloutRestart(ctx, client.RolloutRestartOptions{
		Kubeconfig: client.Kubeconfig{Path: restartOpt.kubeconfig, Context: restartOpt.kubeconfigContext},
		Namespace:  restartOpt.namespace,
		Resources:  restartOpt.resources,
	})
}
