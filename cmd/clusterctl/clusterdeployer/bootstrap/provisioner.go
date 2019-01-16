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

package bootstrap

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap/existing"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap/minikube"
)

// Can provision a kubernetes cluster
type ClusterProvisioner interface {
	Create() error
	Delete() error
	GetKubeconfig() (string, error)
}

func Get(o Options) (ClusterProvisioner, error) {
	switch o.Type {
	case "minikube":
		return minikube.WithOptions(o.ExtraFlags), nil
	default:
		if o.KubeConfig != "" {
			return existing.NewExistingCluster(o.KubeConfig)
		}

		return nil, errors.New("Cannot get bootstrapper, specify `--bootstrap-cluster-kubeconfig` to use an existing Kubernetes cluster or specify `--bootstrap-type` to use a built-in ephemeral cluster.")
	}
}
