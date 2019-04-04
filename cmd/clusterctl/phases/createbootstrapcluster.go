/*
Copyright 2018 The Kubernetes Authors.

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
	"github.com/pkg/errors"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
)

func CreateBootstrapCluster(provisioner bootstrap.ClusterProvisioner, cleanupBootstrapCluster bool, clientFactory clusterclient.Factory) (clusterclient.Client, func(), error) {
	klog.Info("Preparing bootstrap cluster")

	cleanupFn := func() {}
	if err := provisioner.Create(); err != nil {
		return nil, cleanupFn, errors.Wrap(err, "could not create bootstrap control plane")
	}

	if cleanupBootstrapCluster {
		cleanupFn = func() {
			klog.Info("Cleaning up bootstrap cluster.")
			provisioner.Delete()
		}
	}

	bootstrapKubeconfig, err := provisioner.GetKubeconfig()
	if err != nil {
		return nil, cleanupFn, errors.Wrap(err, "unable to get bootstrap cluster kubeconfig")
	}
	bootstrapClient, err := clientFactory.NewClientFromKubeconfig(bootstrapKubeconfig)
	if err != nil {
		return nil, cleanupFn, errors.Wrap(err, "unable to create bootstrap client")
	}

	return bootstrapClient, cleanupFn, nil
}
