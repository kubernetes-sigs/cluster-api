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

// statusOptions is the start of the data required to perform the operation.
type statusOptions struct {
	kubeconfig        string
	kubeconfigContext string
	resources         []string
	namespace         string
}

var statusOpt = &statusOptions{}

var (
	statusLong = templates.LongDesc(`
		Show the status of the rollout.`)

	statusExample = templates.Examples(`
		# Watch the rollout status of a deployment
		clusterctl alpha rollout status machinedeployment/my-md-0`)
)

// NewCmdRolloutStatus returns a Command instance for 'rollout status' sub command.
func NewCmdRolloutStatus(cfgFile string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "status RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 "Status a cluster-api resource",
		Long:                  statusLong,
		Example:               statusExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cfgFile, args)
		},
	}
	cmd.Flags().StringVar(&statusOpt.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	cmd.Flags().StringVar(&statusOpt.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	cmd.Flags().StringVarP(&statusOpt.namespace, "namespace", "n", "", "Namespace where the resource(s) reside. If unspecified, the defult namespace will be used.")

	return cmd
}

func runStatus(cfgFile string, args []string) error {
	statusOpt.resources = args

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	return c.RolloutStatus(client.RolloutStatusOptions{
		Kubeconfig: client.Kubeconfig{Path: statusOpt.kubeconfig, Context: statusOpt.kubeconfigContext},
		Namespace:  statusOpt.namespace,
		Resources:  statusOpt.resources,
	})
}
