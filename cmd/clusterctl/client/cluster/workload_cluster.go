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
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	utilkubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"
	utilsecret "sigs.k8s.io/cluster-api/util/secret"
)

// WorkloadCluster has methods for fetching kubeconfig of workload cluster from management cluster.
type WorkloadCluster interface {
	// GetKubeconfig returns the kubeconfig of the workload cluster.
	GetKubeconfig(workloadClusterName string, namespace string, userKubeconfig bool) (string, error)
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

func (p *workloadCluster) GetKubeconfig(workloadClusterName string, namespace string, userKubeconfig bool) (string, error) {
	cs, err := p.proxy.NewClient()
	if err != nil {
		return "", err
	}

	obj := client.ObjectKey{
		Namespace: namespace,
		Name:      workloadClusterName,
	}
	dataBytes, err := utilkubeconfig.FromSecret(ctx, cs, obj, userKubeconfig)
	if err != nil {
		errSecretName := utilsecret.Name(workloadClusterName, utilsecret.Kubeconfig)
		if userKubeconfig {
			errUserSecretName := utilsecret.Name(workloadClusterName, utilsecret.UserKubeconfig)
			// fallback to Kubeconfig if UserKubeconfig is not found
			klog.Warningf("unable to find secret %q; finding %q instead", errUserSecretName, errSecretName)
			dataBytes, err = utilkubeconfig.FromSecret(ctx, cs, obj, false)
			if err != nil {
				return "", errors.Wrapf(err, "%q not found in namespace %q", errUserSecretName, namespace)
			}
			return string(dataBytes), nil
		}
		return "", errors.Wrapf(err, "%q not found in namespace %q", errSecretName, namespace)
	}
	return string(dataBytes), nil
}
