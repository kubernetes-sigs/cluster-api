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
	"strconv"
	"time"

	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/util"
)

const (
	TimeoutMachineReadyEnv        = "CLUSTER_API_KUBECONFIG_READY_TIMEOUT"
	defaultTimeoutKubeconfigReady = 20 * time.Minute
	retryKubeConfigReady          = 10 * time.Second
)

// GetKubeconfig returns a kubeconfig for the target cluster
func GetKubeconfig(bootstrapClient clusterclient.Client, kubeconfigOutput string, clusterName, namespace string) (string, error) {
	klog.V(1).Info("Getting target cluster kubeconfig.")
	targetKubeconfig, err := waitForKubeconfigReady(bootstrapClient, clusterName, namespace)
	if err != nil {
		return "", fmt.Errorf("unable to get target cluster kubeconfig: %v", err)
	}

	if err := writeKubeconfig(targetKubeconfig, kubeconfigOutput); err != nil {
		return "", err
	}

	return targetKubeconfig, nil
}

func waitForKubeconfigReady(bootstrapClient clusterclient.Client, clusterName, namespace string) (string, error) {
	timeout := defaultTimeoutKubeconfigReady
	if v := os.Getenv(TimeoutMachineReadyEnv); v != "" {
		t, err := strconv.Atoi(v)
		if err == nil {
			timeout = time.Duration(t) * time.Minute
			klog.V(4).Infof("Setting KubeConfig timeout to %v", timeout)
		}
	}

	kubeconfig := ""
	err := util.PollImmediate(retryKubeConfigReady, timeout, func() (bool, error) {
		klog.V(2).Infof("Waiting for kubeconfig from Secret %q-kubeconfig in namespace %q to become available...", clusterName, namespace)
		k, err := bootstrapClient.GetKubeconfigFromSecret(namespace, clusterName)
		if err != nil {
			klog.V(4).Infof("error getting kubeconfig: %v", err)
			return false, nil
		}
		if k == "" {
			klog.V(4).Info("error getting kubeconfig: secret value is empty")
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
