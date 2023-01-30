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

// Package rollout implements the clusterctl rollout command.
package rollout

import (
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

// pauseOptions is the start of the data required to perform the operation.
type pauseOptions struct {
	kubeconfig        string
	kubeconfigContext string
	resources         []string
	namespace         string
}

var pauseOpt = &pauseOptions{}

var (
	pauseLong = templates.LongDesc(`
		Mark the provided cluster-api resource as paused.

	        Paused resources will not be reconciled by a controller. Use "clusterctl alpha rollout resume" to resume a paused resource. Currently only MachineDeployments and KubeadmControlPlanes support being paused.`)

	pauseExample = templates.Examples(`
		# Mark the machinedeployment as paused.
		clusterctl alpha rollout pause machinedeployment/my-md-0

		# Mark the KubeadmControlPlane as paused.
		clusterctl alpha rollout pause kubeadmcontrolplane/my-kcp`)
)

// NewCmdRolloutPause returns a Command instance for 'rollout pause' sub command.
func NewCmdRolloutPause(cfgFile string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "pause RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 "Pause a cluster-api resource",
		Long:                  pauseLong,
		Example:               pauseExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPause(cfgFile, args)
		},
	}
	cmd.Flags().StringVar(&pauseOpt.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	cmd.Flags().StringVar(&pauseOpt.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	cmd.Flags().StringVarP(&pauseOpt.namespace, "namespace", "n", "", "Namespace where the resource(s) reside. If unspecified, the defult namespace will be used.")

	return cmd
}

func runPause(cfgFile string, args []string) error {
	pauseOpt.resources = args

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	return c.RolloutPause(client.RolloutPauseOptions{
		Kubeconfig: client.Kubeconfig{Path: pauseOpt.kubeconfig, Context: pauseOpt.kubeconfigContext},
		Namespace:  pauseOpt.namespace,
		Resources:  pauseOpt.resources,
	})
}
