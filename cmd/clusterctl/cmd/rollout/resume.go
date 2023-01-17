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

// resumeOptions is the start of the data required to perform the operation.
type resumeOptions struct {
	kubeconfig        string
	kubeconfigContext string
	resources         []string
	namespace         string
}

var resumeOpt = &resumeOptions{}

var (
	resumeLong = templates.LongDesc(`
		Resume a paused cluster-api resource

	        Paused resources will not be reconciled by a controller. By resuming a resource, we allow it to be reconciled again. Currently only MachineDeployments and KubeadmControlPlanes support being resumed.`)

	resumeExample = templates.Examples(`
		# Resume an already paused machinedeployment
		clusterctl alpha rollout resume machinedeployment/my-md-0

		# Resume a kubeadmcontrolplane
		clusterctl alpha rollout resume kubeadmcontrolplane/my-kcp`)
)

// NewCmdRolloutResume returns a Command instance for 'rollout resume' sub command.
func NewCmdRolloutResume(cfgFile string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "resume RESOURCE",
		DisableFlagsInUseLine: true,
		Short:                 "Resume a cluster-api resource",
		Long:                  resumeLong,
		Example:               resumeExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResume(cfgFile, args)
		},
	}
	cmd.Flags().StringVar(&resumeOpt.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	cmd.Flags().StringVar(&resumeOpt.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	cmd.Flags().StringVarP(&resumeOpt.namespace, "namespace", "n", "", "Namespace where the resource(s) reside. If unspecified, the defult namespace will be used.")

	return cmd
}

func runResume(cfgFile string, args []string) error {
	resumeOpt.resources = args

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	return c.RolloutResume(client.RolloutResumeOptions{
		Kubeconfig: client.Kubeconfig{Path: resumeOpt.kubeconfig, Context: resumeOpt.kubeconfigContext},
		Namespace:  resumeOpt.namespace,
		Resources:  resumeOpt.resources,
	})
}
