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
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

type fakeManagementCluster struct {
	// TODO: once all client interactions are moved to the Management cluster this can go away
	Management   *internal.Management
	Machines     collections.Machines
	MachinePools *expv1.MachinePoolList
	Workload     *fakeWorkloadCluster
	WorkloadErr  error
	Reader       client.Reader
}

func (f *fakeManagementCluster) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return f.Reader.Get(ctx, key, obj, opts...)
}

func (f *fakeManagementCluster) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return f.Reader.List(ctx, list, opts...)
}

func (f *fakeManagementCluster) GetWorkloadCluster(_ context.Context, _ client.ObjectKey) (internal.WorkloadCluster, error) {
	return f.Workload, f.WorkloadErr
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
	Status                     internal.ClusterStatus
	EtcdMembersResult          []string
	APIServerCertificateExpiry *time.Time

	forwardEtcdLeadershipCalled      int
	removeEtcdMemberForMachineCalled int
}

func (f *fakeWorkloadCluster) ForwardEtcdLeadership(_ context.Context, _ *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error {
	f.forwardEtcdLeadershipCalled++
	if leaderCandidate == nil {
		return errors.New("leaderCandidate is nil")
	}
	return nil
}

func (f *fakeWorkloadCluster) ReconcileEtcdMembersAndControlPlaneNodes(_ context.Context, _ []*etcd.Member, _ []string) ([]string, error) {
	return nil, nil
}

func (f *fakeWorkloadCluster) ClusterStatus(_ context.Context) (internal.ClusterStatus, error) {
	return f.Status, nil
}

func (f *fakeWorkloadCluster) GetAPIServerCertificateExpiry(_ context.Context, _ *bootstrapv1.KubeadmConfig, _ string) (*time.Time, error) {
	return f.APIServerCertificateExpiry, nil
}

func (f *fakeWorkloadCluster) AllowBootstrapTokensToGetNodes(_ context.Context) error {
	return nil
}

func (f *fakeWorkloadCluster) AllowClusterAdminPermissions(_ context.Context, _ semver.Version) error {
	return nil
}

func (f *fakeWorkloadCluster) ReconcileKubeletRBACRole(_ context.Context, _ semver.Version) error {
	return nil
}

func (f *fakeWorkloadCluster) ReconcileKubeletRBACBinding(_ context.Context, _ semver.Version) error {
	return nil
}

func (f *fakeWorkloadCluster) UpdateKubernetesVersionInKubeadmConfigMap(semver.Version) func(*bootstrapv1.ClusterConfiguration) {
	return nil
}

func (f *fakeWorkloadCluster) UpdateEtcdLocalInKubeadmConfigMap(*bootstrapv1.LocalEtcd) func(*bootstrapv1.ClusterConfiguration) {
	return nil
}

func (f *fakeWorkloadCluster) UpdateKubeletConfigMap(_ context.Context, _ semver.Version) error {
	return nil
}

func (f *fakeWorkloadCluster) RemoveEtcdMemberForMachine(_ context.Context, _ *clusterv1.Machine) error {
	f.removeEtcdMemberForMachineCalled++
	return nil
}

func (f *fakeWorkloadCluster) EtcdMembers(_ context.Context) ([]string, error) {
	return f.EtcdMembersResult, nil
}

func (f *fakeWorkloadCluster) UpdateClusterConfiguration(context.Context, semver.Version, ...func(*bootstrapv1.ClusterConfiguration)) error {
	return nil
}

type fakeMigrator struct {
	migrateCalled    bool
	migrateErr       error
	migratedCorefile string
}

func (m *fakeMigrator) Migrate(string, string, string, bool) (string, error) {
	m.migrateCalled = true
	if m.migrateErr != nil {
		return "", m.migrateErr
	}
	return m.migratedCorefile, nil
}
