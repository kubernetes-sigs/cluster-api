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

package phases

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/provider"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/util"
)

const (
	retryKubeConfigReady   = 10 * time.Second
	timeoutKubeconfigReady = 20 * time.Minute
)

// GetKubeconfig returns a kubeconfig for the target cluster
func GetKubeconfig(bootstrapClient clusterclient.Client, provider provider.Deployer, kubeconfigOutput string, clusterName, namespace string) (string, error) {
	cluster, controlPlane, _, err := clusterclient.GetClusterAPIObject(bootstrapClient, clusterName, namespace)
	if err != nil {
		return "", err
	}

	klog.V(1).Info("Getting target cluster kubeconfig.")
	targetKubeconfig, err := waitForKubeconfigReady(provider, cluster, controlPlane)
	if err != nil {
		return "", fmt.Errorf("unable to get target cluster kubeconfig: %v", err)
	}

	if err := writeKubeconfig(targetKubeconfig, kubeconfigOutput); err != nil {
		return "", err
	}

	return targetKubeconfig, nil
}

func waitForKubeconfigReady(provider provider.Deployer, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {
	kubeconfig := ""
	err := util.PollImmediate(retryKubeConfigReady, timeoutKubeconfigReady, func() (bool, error) {
		klog.V(2).Infof("Waiting for kubeconfig on %v to become ready...", machine.Name)
		k, err := provider.GetKubeConfig(cluster, machine)
		if err != nil {
			klog.V(4).Infof("error getting kubeconfig: %v", err)
			return false, nil
		}
		if k == "" {
			return false, nil
		}
		kubeconfig = k
		return true, nil
	})

	return kubeconfig, err
}

func writeKubeconfig(kubeconfig string, kubeconfigOutput string) error {
	const fileMode = 0660
	os.Remove(kubeconfigOutput)
	return ioutil.WriteFile(kubeconfigOutput, []byte(kubeconfig), fileMode)
}
