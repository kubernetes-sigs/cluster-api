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

package provider

import (
	clusterv1 "github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	"k8s.io/client-go/kubernetes"
)

// Deployer is a deprecated interface for Provider specific logic. Please do not extend or add. This interface should be removed
// once issues/158 and issues/160 below are fixed.
type Deployer interface {
	// TODO: This requirement can be removed once after: https://github.com/kubernetes-sigs/cluster-api/issues/158
	GetIP(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error)
	// TODO: This requirement can be removed after: https://github.com/kubernetes-sigs/cluster-api/issues/160
	GetKubeConfig(cluster *clusterv1.Cluster, master *clusterv1.Machine) (string, error)
}

// ComponentsStore is an interface for saving and loading Provider Components
type ComponentsStore interface {
	Save(providerComponents string) error
	Load() (string, error)
}

// ComponentsStoreFactory is an interface for creating ComponentsStores
type ComponentsStoreFactory interface {
	NewFromCoreClientset(clientset *kubernetes.Clientset) (ComponentsStore, error)
}
