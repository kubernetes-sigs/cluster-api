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

package cluster

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

// Actuator is responsible for performing cluster reconciliation
type Actuator struct {
	clusterV1alpha1 clusterv1alpha1.ClusterV1alpha1Interface
	recorder        record.EventRecorder
}

// NewClusterActuator creates a new cluster actuator
func NewClusterActuator(clusterV1alpha1 clusterv1alpha1.ClusterV1alpha1Interface, recorder record.EventRecorder) (*Actuator, error) {
	return &Actuator{
		clusterV1alpha1: clusterV1alpha1,
		recorder:        recorder,
	}, nil
}

// Reconcile will create or update the cluster
func (a *Actuator) Reconcile(cluster *clusterv1.Cluster) error {
	a.recorder.Event(cluster, corev1.EventTypeWarning, "cluster-api", "clusteractuator Reconcile invoked")
	return nil
}

// Delete deletes a cluster and is invoked by the Cluster Controller
func (a *Actuator) Delete(cluster *clusterv1.Cluster) error {
	a.recorder.Event(cluster, corev1.EventTypeWarning, "cluster-api", "clusteractuator Delete invoked")
	return nil
}
