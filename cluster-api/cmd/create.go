package cmd

import (
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/spf13/cobra"
	"k8s.io/kube-deploy/cluster-api/deploy"
	_ "k8s.io/kube-deploy/cluster-api/deploy"
	"os"
)

type CreateOptions struct {
	Cluster                 string
	Machine                 string
	EnableMachineController bool
}

var co = &CreateOptions{}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Simple kubernetes cluster creator",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if co.Cluster == "" {
			logger.Critical("Please provide yaml file for cluster definition.")
			os.Exit(1)
		}
		if co.Machine == "" {
			logger.Critical("Please provide yaml file for machine definition.")
			os.Exit(1)
		}
		if err := RunCreate(co); err != nil {
			logger.Critical(err.Error())
			os.Exit(1)
		}
	},
}

func RunCreate(co *CreateOptions) error {
	logger.Info("start parsing")

	cluster, err := parseClusterYaml(co.Cluster)
	if err != nil {
		return err
	}
	//logger.Info("Parsing done cluster: [%s]", cluster)

	machines, err := parseMachinesYaml(co.Machine)
	if err != nil {
		return err
	}

	//logger.Info("Parsing done [%s]", machines)

	if err = deploy.CreateCluster(cluster, machines, co.EnableMachineController); err != nil {
		return err
	}
	return nil

}
func init() {
	createCmd.Flags().StringVarP(&co.Cluster, "cluster", "c", "", "cluster yaml file")
	createCmd.Flags().StringVarP(&co.Machine, "machines", "m", "", "machine yaml file")
	createCmd.Flags().BoolVarP(&co.EnableMachineController, "enable-machine-controller", "e", false, "whether or not to start the machine controller")

	RootCmd.AddCommand(createCmd)
}
