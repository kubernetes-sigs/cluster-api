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

package internal

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	"sigs.k8s.io/cluster-api/util/secret"
)

// ManagementCluster defines all behaviors necessary for something to function as a management cluster.
type ManagementCluster interface {
	GetMachinesForCluster(ctx context.Context, filters ...machinefilters.Func) (FilterableMachineCollection, error)
	TargetClusterEtcdIsHealthy(ctx context.Context, controlPlaneName string) error
	TargetClusterControlPlaneIsHealthy(ctx context.Context, controlPlaneName string) error
	GetWorkloadCluster(ctx context.Context) (WorkloadCluster, error)
	SetClusterKeyIfEmpty(clusterKey client.ObjectKey)
}

// Management holds operations on the management cluster.
type Management struct {
	Client     ctrlclient.Client
	ClusterKey client.ObjectKey
}

// SetClusterKeyIfEmpty will set the cluster key if one has not already been set.
func (m *Management) SetClusterKeyIfEmpty(clusterKey client.ObjectKey) {
	if m.ClusterKey.Namespace == "" && m.ClusterKey.Name == "" {
		m.ClusterKey = clusterKey
	}
}

// GetMachinesForCluster returns a list of machines that can be filtered or not.
// If no filter is supplied then all machines associated with the target cluster are returned.
func (m *Management) GetMachinesForCluster(ctx context.Context, filters ...machinefilters.Func) (FilterableMachineCollection, error) {
	selector := map[string]string{
		clusterv1.ClusterLabelName: m.ClusterKey.Name,
	}
	ml := &clusterv1.MachineList{}
	if err := m.Client.List(ctx, ml, client.InNamespace(m.ClusterKey.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}

	machines := NewFilterableMachineCollectionFromMachineList(ml)
	return machines.Filter(filters...), nil
}

// GetWorkloadCluster builds a cluster object.
// The cluster comes with an etcd client generator to connect to any etcd pod living on a managed machine.
func (m *Management) GetWorkloadCluster(ctx context.Context) (WorkloadCluster, error) {
	// TODO(chuckha): Inject this dependency.
	// TODO(chuckha): memoize this function. The workload client only exists as long as a reconciliation loop.
	restConfig, err := remote.RESTConfig(ctx, m.Client, m.ClusterKey)
	if err != nil {
		return nil, err
	}
	restConfig.Timeout = 30 * time.Second

	c, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client for workload cluster %v", m.ClusterKey)
	}

	etcdCASecret := &corev1.Secret{}
	etcdCAObjectKey := ctrlclient.ObjectKey{
		Namespace: m.ClusterKey.Namespace,
		Name:      fmt.Sprintf("%s-etcd", m.ClusterKey.Name),
	}
	if err := m.Client.Get(ctx, etcdCAObjectKey, etcdCASecret); err != nil {
		return nil, errors.Wrapf(err, "failed to get secret; etcd CA bundle %s/%s", etcdCAObjectKey.Namespace, etcdCAObjectKey.Name)
	}
	crtData, ok := etcdCASecret.Data[secret.TLSCrtDataName]
	if !ok {
		return nil, errors.Errorf("etcd tls crt does not exist for cluster %s/%s", m.ClusterKey.Namespace, m.ClusterKey.Name)
	}
	keyData, ok := etcdCASecret.Data[secret.TLSKeyDataName]
	if !ok {
		return nil, errors.Errorf("etcd tls key does not exist for cluster %s/%s", m.ClusterKey.Namespace, m.ClusterKey.Name)
	}

	clientCert, err := generateClientCert(crtData, keyData)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(crtData)
	cfg := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
	}

	return &Workload{
		Client:          c,
		CoreDNSMigrator: &CoreDNSMigrator{},
		etcdClientGenerator: &etcdClientGenerator{
			restConfig: restConfig,
			tlsConfig:  cfg,
		},
	}, nil
}

type healthCheck func(context.Context) (HealthCheckResult, error)

// HealthCheck will run a generic health check function and report any errors discovered.
// In addition to the health check, it also ensures there is a 1;1 match between nodes and machines.
func (m *Management) healthCheck(ctx context.Context, check healthCheck, controlPlaneName string) error {
	var errorList []error
	nodeChecks, err := check(ctx)
	if err != nil {
		errorList = append(errorList, err)
	}
	for nodeName, err := range nodeChecks {
		if err != nil {
			errorList = append(errorList, fmt.Errorf("node %q: %v", nodeName, err))
		}
	}
	if len(errorList) != 0 {
		return kerrors.NewAggregate(errorList)
	}

	// Make sure Cluster API is aware of all the nodes.
	machines, err := m.GetMachinesForCluster(ctx, machinefilters.OwnedControlPlaneMachines(controlPlaneName))
	if err != nil {
		return err
	}

	// This check ensures there is a 1 to 1 correspondence of nodes and machines.
	// If a machine was not checked this is considered an error.
	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			return errors.Errorf("control plane machine %s/%s has no status.nodeRef", machine.Namespace, machine.Name)
		}
		if _, ok := nodeChecks[machine.Status.NodeRef.Name]; !ok {
			return errors.Errorf("machine's (%s/%s) node (%s) was not checked", machine.Namespace, machine.Name, machine.Status.NodeRef.Name)
		}
	}
	if len(nodeChecks) != len(machines) {
		return errors.Errorf("number of nodes and machines in namespace %s did not match: %d nodes %d machines", m.ClusterKey.Namespace, len(nodeChecks), len(machines))
	}
	return nil
}

// TargetClusterControlPlaneIsHealthy checks every node for control plane health.
func (m *Management) TargetClusterControlPlaneIsHealthy(ctx context.Context, controlPlaneName string) error {
	// TODO: add checks for expected taints/labels
	cluster, err := m.GetWorkloadCluster(ctx)
	if err != nil {
		return err
	}
	return m.healthCheck(ctx, cluster.ControlPlaneIsHealthy, controlPlaneName)
}

// TargetClusterEtcdIsHealthy runs a series of checks over a target cluster's etcd cluster.
// In addition, it verifies that there are the same number of etcd members as control plane Machines.
func (m *Management) TargetClusterEtcdIsHealthy(ctx context.Context, controlPlaneName string) error {
	cluster, err := m.GetWorkloadCluster(ctx)
	if err != nil {
		return err
	}
	return m.healthCheck(ctx, cluster.EtcdIsHealthy, controlPlaneName)
}
