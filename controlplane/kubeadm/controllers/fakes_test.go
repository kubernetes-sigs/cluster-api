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

	"github.com/blang/semver"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeManagementCluster struct {
	// TODO: once all client interactions are moved to the Management cluster this can go away
	Management   *internal.Management
	Machines     collections.Machines
	MachinePools *expv1.MachinePoolList
	Workload     fakeWorkloadCluster
	Reader       client.Reader
}

func (f *fakeManagementCluster) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return f.Reader.Get(ctx, key, obj)
}

func (f *fakeManagementCluster) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return f.Reader.List(ctx, list, opts...)
}

func (f *fakeManagementCluster) GetWorkloadCluster(_ context.Context, _ client.ObjectKey) (internal.WorkloadCluster, error) {
	return f.Workload, nil
}

func (f *fakeManagementCluster) GetMachinesForCluster(c context.Context, cluster *clusterv1.Cluster, filters ...collections.Func) (collections.Machines, error) {
	if f.Management != nil {
		return f.Management.GetMachinesForCluster(c, cluster, filters...)
	}
	return f.Machines, nil
}

func (f *fakeManagementCluster) GetMachinePoolsForCluster(c context.Context, cluster *clusterv1.Cluster) (*expv1.MachinePoolList, error) {
	if f.Management != nil {
		return f.Management.GetMachinePoolsForCluster(c, cluster)
	}
	return f.MachinePools, nil
}

type fakeWorkloadCluster struct {
	*internal.Workload
	Status            internal.ClusterStatus
	EtcdMembersResult []string
}

func (f fakeWorkloadCluster) ForwardEtcdLeadership(_ context.Context, _ *clusterv1.Machine, _ *clusterv1.Machine) error {
	return nil
}

func (f fakeWorkloadCluster) ReconcileEtcdMembers(ctx context.Context, nodeNames []string, version semver.Version) ([]string, error) {
	return nil, nil
}

func (f fakeWorkloadCluster) ClusterStatus(_ context.Context) (internal.ClusterStatus, error) {
	return f.Status, nil
}

func (f fakeWorkloadCluster) AllowBootstrapTokensToGetNodes(ctx context.Context) error {
	return nil
}

func (f fakeWorkloadCluster) ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) UpdateEtcdVersionInKubeadmConfigMap(ctx context.Context, imageRepository, imageTag string, version semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error {
	return nil
}

func (f fakeWorkloadCluster) RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine, version semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) EtcdMembers(_ context.Context) ([]string, error) {
	return f.EtcdMembersResult, nil
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
