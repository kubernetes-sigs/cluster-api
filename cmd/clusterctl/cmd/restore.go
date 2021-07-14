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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type restoreOptions struct {
	toKubeconfig        string
	toKubeconfigContext string
	directory           string
}

var ro = &restoreOptions{}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore Cluster API objects from file by glob. Object files are searched in config directory",
	Long: LongDesc(`
		Restore Cluster API objects from file by glob. Object files are searched in the default config directory
		or in the provided directory.`),
	Example: Examples(`
		Restore Cluster API objects from file by glob. Object files are searched in config directory.
		clusterctl restore my-cluster`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRestore()
	},
}

func init() {
	restoreCmd.Flags().StringVar(&ro.toKubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file for the target management cluster to restore objects to. If unspecified, default discovery rules apply.")
	restoreCmd.Flags().StringVar(&ro.toKubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file for the target management cluster. If empty, current context will be used.")
	restoreCmd.Flags().StringVar(&ro.directory, "directory", "",
		"The directory to target when restoring Cluster API object yaml files")

	RootCmd.AddCommand(restoreCmd)
}

func runRestore() error {
	if ro.directory == "" {
		return errors.New("please specify a directory to restore cluster API objects from using the --directory flag")
	}

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	return c.Restore(client.RestoreOptions{
		ToKubeconfig: client.Kubeconfig{Path: ro.toKubeconfig, Context: ro.toKubeconfigContext},
		Directory:    ro.directory,
	})
}
