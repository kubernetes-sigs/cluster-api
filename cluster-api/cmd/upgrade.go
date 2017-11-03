package cmd

import (
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"github.com/spf13/cobra"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"k8s.io/kube-deploy/cluster-api/deploy"
)

type UpgradeOptions struct {
	KubernetesVersion string
}

var uo = &UpgradeOptions{}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Kubernetes cluster.",
	Long:  `Upgrade the kubernetes control plan and nodes to the specified version.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if uo.KubernetesVersion == "" {
			logger.Critical("Please provide new kubernetes version.")
			os.Exit(1)
		}

		if err := RunUpgrade(uo); err != nil {
			logger.Critical(err.Error())
			os.Exit(1)
		}
	},
}

func RunUpgrade(uo *UpgradeOptions) error {
	if err := deploy.UpgradeCluster(uo.KubernetesVersion, KubeConfig); err != nil {
		logger.Critical(err.Error())
		os.Exit(1)
	}
	return nil
}

func init() {
	upgradeCmd.Flags().StringVarP(&uo.KubernetesVersion, "version", "v", "", "Kubernetes Version")
	RootCmd.AddCommand(upgradeCmd)
}

