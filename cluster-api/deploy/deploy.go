package deploy

import (
	"github.com/kris-nova/kubicorn/cutil/initapi"
	"github.com/kris-nova/kubicorn/cutil/agent"
	"github.com/kris-nova/kubicorn/cutil"
	"fmt"
	"github.com/kris-nova/kubicorn/cutil/logger"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/cutil/kubeconfig"
	"strings"
	"k8s.io/kube-deploy/cluster-api/api"
	"k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"

	"github.com/kris-nova/kubicorn/profiles/azure"
	"github.com/kris-nova/kubicorn/cutil/kubeadm"
	"github.com/kris-nova/kubicorn/cutil/task"
)


// CreateCluster uses kubicorn API to create cluster
func CreateCluster(c *api.Cluster, machines []v1alpha1.Machine) error {
	cluster, err := convertToKubecornCluster(c)
	if err != nil {
		return err
	}
	newCluster, err := initapi.InitCluster(cluster)
	if err != nil {
		return err
	}

	runtimeParams := &cutil.RuntimeParameters{}
	reconciler, err := cutil.GetReconciler(newCluster, runtimeParams)
	if err != nil {
		return fmt.Errorf("Unable to get reconciler: %v", err)
	}

	logger.Info("Query existing resources")
	actual, err := reconciler.Actual(newCluster)
	if err != nil {
		return fmt.Errorf("Unable to get actual cluster: %v", err)
	}
	logger.Info("Resolving expected resources")
	expected, err := reconciler.Expected(newCluster)
	if err != nil {
		return fmt.Errorf("Unable to get expected cluster: %v", err)
	}

	logger.Info("Reconciling")
	newCluster, err = reconciler.Reconcile(actual, expected)
	if err != nil {
		return fmt.Errorf("Unable to reconcile cluster: %v", err)
	}

	if err = getMasterIP(newCluster); err != nil {
		return fmt.Errorf("Unable to get master IP: %v", err)
	}

	agent := agent.NewAgent()
	if err = kubeconfig.RetryGetConfig(newCluster, agent); err != nil {
		return fmt.Errorf("Unable to write kubeconfig: %v", err)
	}

	if err = CreateMachineCRD(machines); err != nil {
		return err
	}

	logger.Always("The [%s] cluster has applied successfully!", newCluster.Name)
	logger.Always("You can now `kubectl get nodes`")
	privKeyPath := strings.Replace(newCluster.SSH.PublicKeyPath, ".pub", "", 1)
	logger.Always("You can SSH into your cluster ssh -i %s %s@%s", privKeyPath, newCluster.SSH.User, newCluster.KubernetesAPI.Endpoint)

	return nil
}


func DeleteCluster(c *api.Cluster) error {
	cluster, err := convertToKubecornCluster(c)
	if err!= nil {
		return err
	}

	newCluster, err := initapi.InitCluster(cluster)
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

func convertToKubecornCluster(cluster *api.Cluster) (*cluster.Cluster, error) {

	if _, ok := profileMapIndexed[cluster.Spec.Cloud]; !ok {
		return nil, fmt.Errorf("invalid cloud option [%s]", cluster.Spec.Cloud)
	}
	if cluster.Spec.Cloud == "google" && cluster.Spec.Project == "" {
		return nil, fmt.Errorf("CloudID is required for google cloud. Please set it to your project ID")
	}
	newCluster := profileMapIndexed[cluster.Spec.Cloud](cluster.Name)
	newCluster.Name = cluster.Name
	newCluster.CloudId = cluster.Spec.Project
	newCluster.SSH.User = cluster.Spec.SSH.User
	newCluster.SSH.PublicKeyPath = cluster.Spec.SSH.PublicKeyPath
	//newCluster.SSH.PublicKeyData = []byte(cluster.Spec.SSH.PublicKeyData)
	//newCluster.SSH.PublicKeyFingerprint = publicKeyFingerprint(newCluster.SSH.PublicKeyData)
	//fmt.Println(newCluster.SSH.PublicKeyFingerprint)
	return newCluster, nil
}


type profileFunc func(name string) *cluster.Cluster

var profileMapIndexed = map[string]profileFunc{
	"azure":        azure.NewUbuntuCluster,
	"google":       NewUbuntuGoogleComputeCluster,
	"gcp":          NewUbuntuGoogleComputeCluster,
}


// NewUbuntuGoogleComputeCluster creates a basic Ubuntu Google Compute cluster.
func NewUbuntuGoogleComputeCluster(name string) *cluster.Cluster {
	return &cluster.Cluster{
		Name:     name,
		CloudId:  "example-id",
		Cloud:    cluster.CloudGoogle,
		Location: "us-central1-a",
		SSH: &cluster.SSH{
			PublicKeyPath: "~/.ssh/id_rsa.pub",
			User:          "ubuntu",
		},
		KubernetesAPI: &cluster.KubernetesAPI{
			Port: "443",
		},
		Values: &cluster.Values{
			ItemMap: map[string]string{
				"INJECTEDTOKEN": kubeadm.GetRandomToken(),
			},
		},
		ServerPools: []*cluster.ServerPool{
			{
				Type:     cluster.ServerPoolTypeMaster,
				Name:     fmt.Sprintf("%s-master", name),
				MaxCount: 1,
				Image:    "ubuntu-1604-xenial-v20170811",
				Size:     "n1-standard-1",
				BootstrapScripts: []string{
					"bootstrap/google_compute_k8s_ubuntu_16.04_master.sh",
				},
			},
		},
	}
}
