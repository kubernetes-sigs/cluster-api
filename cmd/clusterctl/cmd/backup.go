/*
Copyright 2021 The Kubernetes Authors.

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

type backupOptions struct {
	fromKubeconfig        string
	fromKubeconfigContext string
	namespace             string
	directory             string
}

var buo = &backupOptions{}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup Cluster API objects and all dependencies from a management cluster.",
	Long: LongDesc(`
		Backup Cluster API objects and all dependencies from a management cluster.`),

	Example: Examples(`
		Backup Cluster API objects and all dependencies from a management cluster.
		clusterctl backup --directory=/tmp/backup-directory`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackup()
	},
}

func init() {
	backupCmd.Flags().StringVar(&buo.fromKubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file for the source management cluster to backup. If unspecified, default discovery rules apply.")
	backupCmd.Flags().StringVar(&buo.fromKubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file for the source management cluster. If empty, current context will be used.")
	backupCmd.Flags().StringVarP(&buo.namespace, "namespace", "n", "",
		"The namespace where the workload cluster is hosted. If unspecified, the current context's namespace is used.")
	backupCmd.Flags().StringVar(&buo.directory, "directory", "",
		"The directory to save Cluster API objects to as yaml files")

	RootCmd.AddCommand(backupCmd)
}

func runBackup() error {
	if buo.directory == "" {
		return errors.New("please specify a directory to backup cluster API objects to using the --directory flag")
	}

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	return c.Backup(client.BackupOptions{
		FromKubeconfig: client.Kubeconfig{Path: buo.fromKubeconfig, Context: buo.fromKubeconfigContext},
		Namespace:      buo.namespace,
		Directory:      buo.directory,
	})
}
