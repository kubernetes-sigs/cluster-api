/*
Copyright 2023 The Kubernetes Authors.

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

package etcd

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/client"
	cmanager "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/runtime/manager"
)

// ResourceGroupResolver defines a func that can identify which workloadCluster/resourceGroup a
// request targets.
type ResourceGroupResolver func(host string) (string, error)

// NewEtcdServerHandler returns an http.Handler for fake etcd members.
func NewEtcdServerHandler(manager cmanager.Manager, log logr.Logger, resolver ResourceGroupResolver) http.Handler {
	svr := grpc.NewServer()

	baseSvr := &baseServer{
		manager:               manager,
		log:                   log,
		resourceGroupResolver: resolver,
	}

	clusterServerSrv := &clusterServerServer{
		baseServer: baseSvr,
	}
	pb.RegisterClusterServer(svr, clusterServerSrv)

	maintenanceSrv := &maintenanceServer{
		baseServer: baseSvr,
	}
	pb.RegisterMaintenanceServer(svr, maintenanceSrv)

	return svr
}

// clusterServerServer implements the MaintenanceServer grpc server.
type maintenanceServer struct {
	*baseServer
}

func (m *maintenanceServer) Alarm(ctx context.Context, _ *pb.AlarmRequest) (*pb.AlarmResponse, error) {
	resourceGroup, etcdMember, err := m.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}

	m.log.Info("Etcd: Alarm", "resourceGroup", resourceGroup, "etcdMember", etcdMember)

	return &pb.AlarmResponse{}, nil
}

func (m *maintenanceServer) Status(ctx context.Context, _ *pb.StatusRequest) (*pb.StatusResponse, error) {
	resourceGroup, etcdMember, err := m.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}
	cloudClient := m.manager.GetResourceGroup(resourceGroup).GetClient()

	m.log.Info("Etcd: Status", "resourceGroup", resourceGroup, "etcdMember", etcdMember)
	_, statusResponse, err := m.inspectEtcd(ctx, cloudClient, etcdMember)
	if err != nil {
		return nil, err
	}

	return statusResponse, nil
}

func (m *maintenanceServer) Defragment(_ context.Context, _ *pb.DefragmentRequest) (*pb.DefragmentResponse, error) {
	panic("implement me")
}

func (m *maintenanceServer) Hash(_ context.Context, _ *pb.HashRequest) (*pb.HashResponse, error) {
	panic("implement me")
}

func (m *maintenanceServer) HashKV(_ context.Context, _ *pb.HashKVRequest) (*pb.HashKVResponse, error) {
	panic("implement me")
}

func (m *maintenanceServer) Snapshot(_ *pb.SnapshotRequest, _ pb.Maintenance_SnapshotServer) error {
	panic("implement me")
}

func (m *maintenanceServer) MoveLeader(_ context.Context, _ *pb.MoveLeaderRequest) (*pb.MoveLeaderResponse, error) {
	panic("implement me")
}

func (m *maintenanceServer) Downgrade(_ context.Context, _ *pb.DowngradeRequest) (*pb.DowngradeResponse, error) {
	panic("implement me")
}

// clusterServerServer implements the ClusterServer grpc server.
type clusterServerServer struct {
	*baseServer
}

func (c *clusterServerServer) MemberAdd(_ context.Context, _ *pb.MemberAddRequest) (*pb.MemberAddResponse, error) {
	panic("implement me")
}

func (c *clusterServerServer) MemberRemove(_ context.Context, _ *pb.MemberRemoveRequest) (*pb.MemberRemoveResponse, error) {
	panic("implement me")
}

func (c *clusterServerServer) MemberUpdate(_ context.Context, _ *pb.MemberUpdateRequest) (*pb.MemberUpdateResponse, error) {
	panic("implement me")
}

func (c *clusterServerServer) MemberList(ctx context.Context, _ *pb.MemberListRequest) (*pb.MemberListResponse, error) {
	resourceGroup, etcdMember, err := c.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}
	cloudClient := c.manager.GetResourceGroup(resourceGroup).GetClient()

	c.log.Info("Etcd: MemberList", "resourceGroup", resourceGroup, "etcdMember", etcdMember)
	memberList, _, err := c.inspectEtcd(ctx, cloudClient, etcdMember)
	if err != nil {
		return nil, err
	}

	return memberList, nil
}

func (c *clusterServerServer) MemberPromote(_ context.Context, _ *pb.MemberPromoteRequest) (*pb.MemberPromoteResponse, error) {
	panic("implement me")
}

type baseServer struct {
	manager               cmanager.Manager
	log                   logr.Logger
	resourceGroupResolver ResourceGroupResolver
}

func (b *baseServer) getResourceGroupAndMember(ctx context.Context) (resourceGroup string, etcdMember string, err error) {
	localAddr := ctx.Value(http.LocalAddrContextKey)
	resourceGroup, err = b.resourceGroupResolver(fmt.Sprintf("%s", localAddr))
	if err != nil {
		return "", "", err
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", "", errors.Errorf("failed to get metadata when processing request to etcd in resourceGroup %s", resourceGroup)
	}
	// Calculate the etcd member name by trimming the "etcd-" prefix from ":authority" metadata.
	etcdMember = strings.TrimPrefix(strings.Join(md.Get(":authority"), ","), "etcd-")
	return
}

func (b *baseServer) inspectEtcd(ctx context.Context, cloudClient cclient.Client, etcdMember string) (*pb.MemberListResponse, *pb.StatusResponse, error) {
	etcdPods := &corev1.PodList{}
	if err := cloudClient.List(ctx, etcdPods,
		client.InNamespace(metav1.NamespaceSystem),
		client.MatchingLabels{
			"component": "etcd",
			"tier":      "control-plane"},
	); err != nil {
		return nil, nil, errors.Wrap(err, "failed to list etcd members")
	}

	memberList := &pb.MemberListResponse{}
	statusResponse := &pb.StatusResponse{}
	var leaderID int
	var leaderFrom time.Time
	for _, pod := range etcdPods.Items {
		clusterID, err := strconv.Atoi(pod.Annotations["etcd.inmemory.infrastructure.cluster.x-k8s.io/cluster-id"])
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed read cluster ID annotation from etcd member with name %s", pod.Name)
		}
		memberID, err := strconv.Atoi(pod.Annotations["etcd.inmemory.infrastructure.cluster.x-k8s.io/member-id"])
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed read member ID annotation from etcd member with name %s", pod.Name)
		}

		if t, err := time.Parse(time.RFC3339, pod.Annotations["etcd.inmemory.infrastructure.cluster.x-k8s.io/leader-from"]); err == nil {
			if t.After(leaderFrom) {
				leaderID = memberID
				leaderFrom = t
			}
		}

		if pod.Name == etcdMember {
			memberList.Header = &pb.ResponseHeader{
				ClusterId: uint64(clusterID),
				MemberId:  uint64(memberID),
			}

			statusResponse.Header = memberList.Header
		}
		memberList.Members = append(memberList.Members, &pb.Member{
			ID:   uint64(memberID),
			Name: strings.TrimPrefix(pod.Name, "etcd-"),
		})
	}
	statusResponse.Leader = uint64(leaderID)

	return memberList, statusResponse, nil
}
