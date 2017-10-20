package cmd

import (
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/profiles"
	"github.com/spf13/cobra"
	"io/ioutil"
	"k8s.io/kube-deploy/cluster-api/api"
	"os"
	"os/exec"
)

var RootCmd = &cobra.Command{
	Use:   "cluster-api",
	Short: "Simple kubernetes cluster management",
	Long:  `Simple kubernetes cluster management`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

func init() {
	logger.Level = 4
}

func execCommand(name string, args []string) string {
	cmdOut, _ := exec.Command(name, args...).Output()
	return string(cmdOut)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func convertToKubecornCluster(cluster *api.Cluster) *cluster.Cluster {
	newCluster := profileMapIndexed[cluster.Spec.Cloud](cluster.Name)
	newCluster.Name = cluster.Name
	newCluster.CloudId = cluster.Spec.Project
	newCluster.SSH.User = cluster.Spec.SSH.User
	newCluster.SSH.PublicKeyPath = cluster.Spec.SSH.PublicKeyPath
	return newCluster
}

func readAndValidateYaml(file string) (*api.Cluster, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	clusterSpec := &api.Cluster{}
	err = yaml.Unmarshal(bytes, clusterSpec)
	if err != nil {
		return nil, err
	}

	if _, ok := profileMapIndexed[clusterSpec.Spec.Cloud]; !ok {
		return nil, fmt.Errorf("invalid cloud option [%s]", clusterSpec.Spec.Cloud)
	}
	if clusterSpec.Spec.Cloud == cluster.CloudGoogle && clusterSpec.Spec.Project == "" {
		return nil, fmt.Errorf("CloudID is required for google cloud. Please set it to your project ID")
	}
	return clusterSpec, nil
}

type profileFunc func(name string) *cluster.Cluster

var profileMapIndexed = map[string]profileFunc{
	"azure":        profiles.NewUbuntuAzureCluster,
	"azure-ubuntu": profiles.NewUbuntuAzureCluster,
	"google":       profiles.NewUbuntuGoogleComputeCluster,
	"gcp":          profiles.NewUbuntuGoogleComputeCluster,
}
