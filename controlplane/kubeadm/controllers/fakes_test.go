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

package controllers

import (
	"context"
	"errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeManagementCluster struct {
	// TODO: once all client interactions are moved to the Management cluster this can go away
	Management          *internal.Management
	ControlPlaneHealthy bool
	EtcdHealthy         bool
	Machines            internal.FilterableMachineCollection
	Workload            fakeWorkloadCluster
}

func (f *fakeManagementCluster) GetWorkloadCluster(_ context.Context, _ client.ObjectKey) (internal.WorkloadCluster, error) {
	return f.Workload, nil
}

func (f *fakeManagementCluster) GetMachinesForCluster(c context.Context, n client.ObjectKey, filters ...internal.MachineFilter) (internal.FilterableMachineCollection, error) {
	if f.Management != nil {
		return f.Management.GetMachinesForCluster(c, n, filters...)
	}
	return f.Machines, nil
}

func (f *fakeManagementCluster) TargetClusterControlPlaneIsHealthy(_ context.Context, _ client.ObjectKey, _ string) error {
	if !f.ControlPlaneHealthy {
		return errors.New("control plane is not healthy")
	}
	return nil
}

func (f *fakeManagementCluster) TargetClusterEtcdIsHealthy(_ context.Context, _ client.ObjectKey, _ string) error {
	if !f.EtcdHealthy {
		return errors.New("etcd is not healthy")
	}
	return nil
}

type fakeWorkloadCluster struct {
	*internal.Workload
	Status internal.ClusterStatus
}

func (f fakeWorkloadCluster) ForwardEtcdLeadership(_ context.Context, _ *clusterv1.Machine) error {
	return nil
}

func (f fakeWorkloadCluster) ClusterStatus(_ context.Context) (internal.ClusterStatus, error) {
	return f.Status, nil
}

type fakeMigrator struct {
	migrateCalled    bool
	migrateErr       error
	migratedCorefile string
}

func (m *fakeMigrator) Migrate(current, to, corefile string, deprecations bool) (string, error) {
	m.migrateCalled = true
	if m.migrateErr != nil {
		return "", m.migrateErr
	}
	return m.migratedCorefile, nil
}
