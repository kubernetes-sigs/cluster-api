/*
Copyright 2020 The Kubernetes Authors.

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

package cluster

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	utilkubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"
	utilsecret "sigs.k8s.io/cluster-api/util/secret"
)

// WorkloadCluster has methods for fetching kubeconfig of workload cluster from management cluster.
type WorkloadCluster interface {
	// GetKubeconfig returns the kubeconfig of the workload cluster.
	GetKubeconfig(workloadClusterName string, namespace string) (string, error)
}

// workloadCluster implements WorkloadCluster.
type workloadCluster struct {
	proxy Proxy
}

// newWorkloadCluster returns a workloadCluster.
func newWorkloadCluster(proxy Proxy) *workloadCluster {
	return &workloadCluster{
		proxy: proxy,
	}
}

func (p *workloadCluster) GetKubeconfig(workloadClusterName string, namespace string) (string, error) {
	cs, err := p.proxy.NewClient()
	if err != nil {
		return "", err
	}

	obj := client.ObjectKey{
		Namespace: namespace,
		Name:      workloadClusterName,
	}
	dataBytes, err := utilkubeconfig.FromSecret(ctx, cs, obj)
	if err != nil {
		errSecretName := utilsecret.Name(workloadClusterName, utilsecret.Kubeconfig)
		return "", errors.Wrapf(err, "%q not found in namespace %q", errSecretName, namespace)
	}
	return string(dataBytes), nil
}

// GetUserKubeconfig returns the kubeconfig of the user as provided by the provider.
// Note: This has not been converted to a method yet to prevent API change.
func GetUserKubeconfig(proxy Proxy, workloadClusterName string, namespace string) (string, error) {
	cs, err := proxy.NewClient()
	if err != nil {
		return "", err
	}

	obj := client.ObjectKey{
		Namespace: namespace,
		Name:      workloadClusterName,
	}
	dataBytes, err := utilkubeconfig.FromUserSecret(ctx, cs, obj)

	if err != nil {
		errUserSecretName := utilsecret.Name(workloadClusterName, utilsecret.UserKubeconfig)
		return "", errors.Wrapf(err, "%q not found in namespace %q", errUserSecretName, namespace)
	}

	return string(dataBytes), nil
}
