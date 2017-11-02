package deploy

import (
	"fmt"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"k8s.io/kube-deploy/cluster-api/client"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
	restclient "k8s.io/client-go/rest"
	"github.com/kris-nova/kubicorn/cutil/local"
	machinesv1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	"os"
	"time"
	"github.com/kris-nova/kubicorn/cloud/google/compute/resources"
	"strings"
	"github.com/kris-nova/kubicorn/apis/cluster"
)

const (
	// MasterIPAttempts specifies how many times are allowed to be taken to get the master node IP.
	MasterIPAttempts = 40
	// MasterIPSleepSecondsPerAttempt specifies how much time should pass after a failed attempt to get the master IP.
	MasterIPSleepSecondsPerAttempt = 3
	// DeleteAttempts specifies the amount of retries are allowed when trying to delete instance templates.
	DeleteAttempts = 150
	// RetrySleepSeconds specifies the time to sleep after a failed attempt to delete instance templates.
	DeleteSleepSeconds = 5
	ServerPoolTypeMaster = "master"
	ServerPoolTypeNode   = "node"
)

func CreateMachineCRD(machines []machinesv1.Machine) error {
	config, err := getConfig()
	cs, err := clientset(config)
	if err != nil {
		return err
	}

	if _, err = machinesv1.CreateMachinesCRD(cs); err != nil {
		return fmt.Errorf("Error creating Machines CRD: %v\n", err)
	}

	fmt.Printf("Machines CRD created successfully!\n")
	client, err := client.NewForConfig(config)
	if err != nil {
		return err
	}

	for _, machine := range machines {
		_, err = client.Machines().Create(&machine)
		if err != nil {
			return err
		}
		logger.Info("Added machine [%s]", machine.Name)
	}


	return nil

}

func clientset(config *restclient.Config) (*apiextensionsclient.Clientset, error) {
	clientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func getConfig() (*restclient.Config, error) {
	kubeconfig, err := getKubeConfigPath()
	if err != nil {
		return nil, err
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func getKubeConfigPath() (string, error) {
	localDir := fmt.Sprintf("%s/.kube", local.Home())
	if _, err := os.Stat(localDir); os.IsNotExist(err) {
		if err := os.Mkdir(localDir, 0777); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%s/config", localDir), nil
}


func getMasterIP(cluster *cluster.Cluster) error {
	masterIPPrivate := ""
	masterIPPublic := ""
	found := false
	for i := 0; i < MasterIPAttempts; i++ {
		masterTag := ""
		for _, serverPool := range cluster.ServerPools {
			if serverPool.Type == ServerPoolTypeMaster {
				masterTag = serverPool.Name
			}
		}
		if masterTag == "" {
			return fmt.Errorf("Unable to find master tag")
		}

		instanceGroupManager, err := resources.Sdk.Service.InstanceGroupManagers.ListManagedInstances(cluster.CloudId, cluster.Location, strings.ToLower(masterTag)).Do()
		if err != nil {
			return err
		}

		if err != nil || len(instanceGroupManager.ManagedInstances) == 0 {
			logger.Debug("Hanging for master IP.. (%v)", err)
			time.Sleep(time.Duration(MasterIPSleepSecondsPerAttempt) * time.Second)
			continue
		}

		parts := strings.Split(instanceGroupManager.ManagedInstances[0].Instance, "/")
		instance, err :=resources.Sdk.Service.Instances.Get(cluster.CloudId, cluster.Location, parts[len(parts)-1]).Do()
		if err != nil {
			logger.Debug("Hanging for master IP.. (%v)", err)
			time.Sleep(time.Duration(MasterIPSleepSecondsPerAttempt) * time.Second)
			continue
		}

		for _, networkInterface := range instance.NetworkInterfaces {
			if networkInterface.Name == "nic0" {
				masterIPPrivate = networkInterface.NetworkIP
				for _, accessConfigs := range networkInterface.AccessConfigs {
					masterIPPublic = accessConfigs.NatIP
				}
			}
		}

		if masterIPPublic == "" {
			logger.Debug("Hanging for master IP..")
			time.Sleep(time.Duration(MasterIPSleepSecondsPerAttempt) * time.Second)
			continue
		}

		found = true
		cluster.Values.ItemMap["INJECTEDMASTER"] = fmt.Sprintf("%s:%s", masterIPPrivate, cluster.KubernetesAPI.Port)
		break
	}
	if !found {
		return fmt.Errorf("Unable to find Master IP after defined wait")
	}
	cluster.Values.ItemMap["INJECTEDPORT"] = cluster.KubernetesAPI.Port
	cluster.KubernetesAPI.Endpoint = masterIPPublic

	return nil
}