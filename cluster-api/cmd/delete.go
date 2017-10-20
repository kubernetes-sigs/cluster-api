package cmd

import (
	"fmt"
	"github.com/kris-nova/kubicorn/cutil"
	"github.com/kris-nova/kubicorn/cutil/initapi"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/cutil/task"
	"github.com/spf13/cobra"
	"k8s.io/kube-deploy/cluster-api/api"
	"os"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [YAML_FILE]",
	Short: "Simple kubernetes cluster manager",
	Long:  `Delete a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			logger.Critical("Please provide yaml file for cluster definition.")
			os.Exit(1)
		} else if len(args) > 1 {
			logger.Critical("Too many arguments.")
			os.Exit(1)
		}
		yamlFile := args[0]
		cluster, err := readAndValidateYaml(yamlFile)
		if err != nil {
			logger.Critical(err.Error())
			os.Exit(1)
		}
		logger.Info("Parsing done [%s]", cluster)

		if err = deleteCluster(cluster); err != nil {
			logger.Critical(err.Error())
			os.Exit(1)
		}

	},
}

// deleteCluster uses kubicorn API to delete cluster.
func deleteCluster(cluster *api.Cluster) error {
	newCluster := convertToKubecornCluster(cluster)

	newCluster, err := initapi.InitCluster(newCluster)
	if err != nil {
		return err
	}
	runtimeParams := &cutil.RuntimeParameters{}
	reconciler, err := cutil.GetReconciler(newCluster, runtimeParams)
	if err != nil {
		return fmt.Errorf("Unable to get reconciler: %v", err)
	}

	var deleteClusterTask = func() error {
		_, err = reconciler.Destroy()
		return err
	}

	err = task.RunAnnotated(deleteClusterTask, fmt.Sprintf("\nDestroying resources for cluster [%s]:\n", newCluster.Name), "")
	if err != nil {
		return fmt.Errorf("Unable to destroy resources for cluster [%s]: %v", newCluster.Name, err)
	}

	return nil
}

func init() {
	RootCmd.AddCommand(deleteCmd)
}
