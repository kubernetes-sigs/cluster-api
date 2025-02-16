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

package cmd

import (
	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd/internal/templates"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/cmd/rollout"
)

var (
	rolloutLong = templates.LongDesc(`
		Manage the rollout of a cluster-api resource.
		Valid resource types include:

		   * machinedeployment
		   * kubeadmcontrolplane
		`)

	rolloutExample = templates.Examples(`
		# Force an immediate rollout of machinedeployment or kubeadmcontrolplane
		clusterctl alpha rollout restart machinedeployment/my-md-0
		clusterctl alpha rollout restart kubeadmcontrolplane/my-kcp

		# Mark the machinedeployment or kubeadmcontrolplane as paused
		clusterctl alpha rollout pause machinedeployment/my-md-0
		clusterctl alpha rollout pause kubeadmcontrolplane/my-kcp

		# Resume an already paused machinedeployment or kubeadmcontrolplane
		clusterctl alpha rollout resume machinedeployment/my-md-0
		clusterctl alpha rollout resume kubeadmcontrolplane/my-kcp

		# Rollback a machinedeployment
		clusterctl alpha rollout undo machinedeployment/my-md-0 --to-revision=3`)

	rolloutCmd = &cobra.Command{
		Use:     "rollout SUBCOMMAND",
		Short:   "Manage the rollout of a cluster-api resource",
		Long:    rolloutLong,
		Example: rolloutExample,
	}
)

func init() {
	// subcommands
	rolloutCmd.AddCommand(rollout.NewCmdRolloutRestart(cfgFile))
	rolloutCmd.AddCommand(rollout.NewCmdRolloutPause(cfgFile))
	rolloutCmd.AddCommand(rollout.NewCmdRolloutResume(cfgFile))
	rolloutCmd.AddCommand(rollout.NewCmdRolloutUndo(cfgFile))
}
