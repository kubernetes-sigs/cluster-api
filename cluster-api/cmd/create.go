package cmd

import (
	"github.com/spf13/cobra"
	"fmt"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"os"
	"io/ioutil"
	"github.com/ghodss/yaml"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/profiles"
	"github.com/kris-nova/kubicorn/cutil/initapi"
	"github.com/kris-nova/kubicorn/cutil"
	"github.com/kris-nova/kubicorn/cutil/kubeconfig"
	"strings"
	"k8s.io/kube-deploy/cluster-api/api"
)

var createCmd = &cobra.Command{
	Use:   "create [YAML_FILE]",
	Short: "Simple kubernetes cluster creator",
	Long: `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			logger.Critical("Please provide yaml file for cluster definition.")
			os.Exit(1)
		} else if len(args) > 1 {
			logger.Critical("Too many arguments.")
			os.Exit(1)
		}
		yamlFile := args[0]
		clusterSpec, err :=readAndValidateYaml(yamlFile)
		if err != nil {
			logger.Critical(err.Error())
			os.Exit(1)
		}
		logger.Info("Parsing done [%s]", clusterSpec)

		var newCluster *cluster.Cluster
		newCluster = profileMapIndexed[clusterSpec.Spec.Cloud](clusterSpec.Name)
		newCluster.Name = clusterSpec.Name
		newCluster.CloudId = clusterSpec.Spec.Project
		newCluster.SSH.User = clusterSpec.Spec.SSH.User
		newCluster.SSH.PublicKeyPath = clusterSpec.Spec.SSH.PublicKeyPath
		if err = createCluster(newCluster); err != nil {
			logger.Critical(err.Error())
			os.Exit(1)
		}

	},
}

func createCluster(cluster *cluster.Cluster) error {
	cluster, err := initapi.InitCluster(cluster)
	if err != nil {
		return err
	}
	runtimeParams := &cutil.RuntimeParameters{}
	reconciler, err := cutil.GetReconciler(cluster, runtimeParams)
	if err != nil {
		return fmt.Errorf("Unable to get reconciler: %v", err)
	}

	logger.Info("Query existing resources")
	actual, err := reconciler.Actual(cluster)
	if err != nil {
		return fmt.Errorf("Unable to get actual cluster: %v", err)
	}
	logger.Info("Resolving expected resources")
	expected, err := reconciler.Expected(cluster)
	if err != nil {
		return fmt.Errorf("Unable to get expected cluster: %v", err)
	}

	logger.Info("Reconciling")
	newCluster, err := reconciler.Reconcile(actual, expected)
	if err != nil {
		return fmt.Errorf("Unable to reconcile cluster: %v", err)
	}

	err = kubeconfig.RetryGetConfig(newCluster)
	if err != nil {
		return fmt.Errorf("Unable to write kubeconfig: %v", err)
	}

	logger.Always("The [%s] cluster has applied successfully!", newCluster.Name)
	logger.Always("You can now `kubectl get nodes`")
	privKeyPath := strings.Replace(cluster.SSH.PublicKeyPath, ".pub", "", 1)
	logger.Always("You can SSH into your cluster ssh -i %s %s@%s", privKeyPath, newCluster.SSH.User, newCluster.KubernetesAPI.Endpoint)

	return nil
}

type profileFunc func(name string) *cluster.Cluster

var profileMapIndexed = map[string]profileFunc{
	"azure": profiles.NewUbuntuAzureCluster,
	"azure-ubuntu": profiles.NewUbuntuAzureCluster,
	"google": profiles.NewUbuntuGoogleComputeCluster,
	"gcp": profiles.NewUbuntuGoogleComputeCluster,
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


func init() {
	RootCmd.AddCommand(createCmd)
}