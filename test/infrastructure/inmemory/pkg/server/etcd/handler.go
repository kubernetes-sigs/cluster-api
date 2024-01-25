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

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
)

// ResourceGroupResolver defines a func that can identify which workloadCluster/resourceGroup a
// request targets.
type ResourceGroupResolver func(host string) (string, error)

// NewEtcdServerHandler returns an http.Handler for fake etcd members.
func NewEtcdServerHandler(manager inmemoryruntime.Manager, log logr.Logger, resolver ResourceGroupResolver) http.Handler {
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
	var resourceGroup string
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("Alarm", resourceGroup).Observe(time.Since(start).Seconds())
	}()

	var etcdMember string
	var err error
	resourceGroup, etcdMember, err = m.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}

	m.log.V(4).Info("Etcd: Alarm", "resourceGroup", resourceGroup, "etcdMember", etcdMember)

	return &pb.AlarmResponse{}, nil
}

func (m *maintenanceServer) Status(ctx context.Context, _ *pb.StatusRequest) (*pb.StatusResponse, error) {
	var resourceGroup string
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("Status", resourceGroup).Observe(time.Since(start).Seconds())
	}()

	var etcdMember string
	var err error
	resourceGroup, etcdMember, err = m.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}
	inmemoryClient := m.manager.GetResourceGroup(resourceGroup).GetClient()

	m.log.V(4).Info("Etcd: Status", "resourceGroup", resourceGroup, "etcdMember", etcdMember)
	_, statusResponse, err := m.inspectEtcd(ctx, inmemoryClient, etcdMember)
	if err != nil {
		return nil, err
	}

	return statusResponse, nil
}

func (m *maintenanceServer) Defragment(_ context.Context, _ *pb.DefragmentRequest) (*pb.DefragmentResponse, error) {
	return nil, fmt.Errorf("not implemented: Defragment")
}

func (m *maintenanceServer) Hash(_ context.Context, _ *pb.HashRequest) (*pb.HashResponse, error) {
	return nil, fmt.Errorf("not implemented: Hash")
}

func (m *maintenanceServer) HashKV(_ context.Context, _ *pb.HashKVRequest) (*pb.HashKVResponse, error) {
	return nil, fmt.Errorf("not implemented: HashKV")
}

func (m *maintenanceServer) Snapshot(_ *pb.SnapshotRequest, _ pb.Maintenance_SnapshotServer) error {
	return fmt.Errorf("not implemented: Snapshot")
}

func (m *maintenanceServer) MoveLeader(ctx context.Context, req *pb.MoveLeaderRequest) (*pb.MoveLeaderResponse, error) {
	var resourceGroup string
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("MoveLeader", resourceGroup).Observe(time.Since(start).Seconds())
	}()

	out := new(pb.MoveLeaderResponse)
	var err error
	resourceGroup, _, err = m.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}
	etcdPods := &corev1.PodList{}
	inmemoryClient := m.manager.GetResourceGroup(resourceGroup).GetClient()
	if err := inmemoryClient.List(ctx, etcdPods,
		client.InNamespace(metav1.NamespaceSystem),
		client.MatchingLabels{
			"component": "etcd",
			"tier":      "control-plane"},
	); err != nil {
		return nil, errors.Wrap(err, "failed to list etcd members")
	}

	if len(etcdPods.Items) == 0 {
		return nil, errors.New("failed to list etcd members: no etcd pods found")
	}

	for i := range etcdPods.Items {
		pod := &etcdPods.Items[i]
		for k, v := range pod.GetAnnotations() {
			if k == cloudv1.EtcdMemberIDAnnotationName {
				target := strconv.FormatInt(int64(req.TargetID), 10)
				if v == target {
					updatedPod := pod.DeepCopy()
					annotations := updatedPod.GetAnnotations()
					annotations[cloudv1.EtcdLeaderFromAnnotationName] = time.Now().Format(time.RFC3339)
					updatedPod.SetAnnotations(annotations)
					err := inmemoryClient.Patch(ctx, updatedPod, client.MergeFrom(pod))
					if err != nil {
						return nil, err
					}
					return out, nil
				}
			}
		}
	}
	// If we reach this point leadership was not moved.
	return nil, errors.Errorf("etcd member with ID %d did not become the leader: expected etcd Pod not found", req.TargetID)
}

func (m *maintenanceServer) Downgrade(_ context.Context, _ *pb.DowngradeRequest) (*pb.DowngradeResponse, error) {
	return nil, fmt.Errorf("not implemented: Downgrade")
}

// clusterServerServer implements the ClusterServer grpc server.
type clusterServerServer struct {
	*baseServer
}

func (c *clusterServerServer) MemberAdd(_ context.Context, _ *pb.MemberAddRequest) (*pb.MemberAddResponse, error) {
	return nil, fmt.Errorf("not implemented: MemberAdd")
}

func (c *clusterServerServer) MemberRemove(ctx context.Context, req *pb.MemberRemoveRequest) (*pb.MemberRemoveResponse, error) {
	var resourceGroup string
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("MemberRemove", resourceGroup).Observe(time.Since(start).Seconds())
	}()

	out := new(pb.MemberRemoveResponse)
	var err error
	resourceGroup, _, err = c.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}
	inmemoryClient := c.manager.GetResourceGroup(resourceGroup).GetClient()

	etcdPods := &corev1.PodList{}

	if err := inmemoryClient.List(ctx, etcdPods,
		client.InNamespace(metav1.NamespaceSystem),
		client.MatchingLabels{
			"component": "etcd",
			"tier":      "control-plane"},
	); err != nil {
		return nil, errors.Wrap(err, "failed to list etcd members")
	}

	for i := range etcdPods.Items {
		pod := etcdPods.Items[i]
		memberID := pod.Annotations[cloudv1.EtcdMemberIDAnnotationName]
		if memberID != fmt.Sprintf("%d", req.ID) {
			continue
		}
		updatedPod := pod.DeepCopy()
		updatedPod.Annotations[cloudv1.EtcdMemberRemoved] = ""
		if err := inmemoryClient.Patch(ctx, updatedPod, client.MergeFrom(&pod)); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, errors.Errorf("no etcd member with id %d found", req.ID)
}

func (c *clusterServerServer) MemberUpdate(_ context.Context, _ *pb.MemberUpdateRequest) (*pb.MemberUpdateResponse, error) {
	return nil, fmt.Errorf("not implemented: MemberUpdate")
}

func (c *clusterServerServer) MemberList(ctx context.Context, _ *pb.MemberListRequest) (*pb.MemberListResponse, error) {
	var resourceGroup string
	start := time.Now()
	defer func() {
		requestLatency.WithLabelValues("MemberList", resourceGroup).Observe(time.Since(start).Seconds())
	}()

	var etcdMember string
	var err error
	resourceGroup, etcdMember, err = c.getResourceGroupAndMember(ctx)
	if err != nil {
		return nil, err
	}
	inmemoryClient := c.manager.GetResourceGroup(resourceGroup).GetClient()

	c.log.V(4).Info("Etcd: MemberList", "resourceGroup", resourceGroup, "etcdMember", etcdMember)
	memberList, _, err := c.inspectEtcd(ctx, inmemoryClient, etcdMember)
	if err != nil {
		return nil, err
	}

	return memberList, nil
}

func (c *clusterServerServer) MemberPromote(_ context.Context, _ *pb.MemberPromoteRequest) (*pb.MemberPromoteResponse, error) {
	return nil, fmt.Errorf("not implemented: MemberPromote")
}

type baseServer struct {
	manager               inmemoryruntime.Manager
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

func (b *baseServer) inspectEtcd(ctx context.Context, inmemoryClient inmemoryruntime.Client, etcdMember string) (*pb.MemberListResponse, *pb.StatusResponse, error) {
	etcdPods := &corev1.PodList{}
	if err := inmemoryClient.List(ctx, etcdPods,
		client.InNamespace(metav1.NamespaceSystem),
		client.MatchingLabels{
			"component": "etcd",
			"tier":      "control-plane"},
	); err != nil {
		return nil, nil, errors.Wrap(err, "failed to list etcd members")
	}

	memberList := &pb.MemberListResponse{}
	statusResponse := &pb.StatusResponse{}
	var clusterID int
	var leaderID int
	var leaderFrom time.Time
	for _, pod := range etcdPods.Items {
		if _, ok := pod.Annotations[cloudv1.EtcdMemberRemoved]; ok {
			if pod.Name == fmt.Sprintf("%s%s", "etcd-", etcdMember) {
				return nil, nil, errors.New("inspect called on etcd which has been removed")
			}
			continue
		}
		if clusterID == 0 {
			var err error
			clusterID, err = strconv.Atoi(pod.Annotations[cloudv1.EtcdClusterIDAnnotationName])
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed read cluster ID annotation from etcd member with name %s", pod.Name)
			}
		} else if pod.Annotations[cloudv1.EtcdClusterIDAnnotationName] != fmt.Sprintf("%d", clusterID) {
			return nil, nil, errors.New("invalid etcd cluster, members have different cluster ID")
		}

		memberID, err := strconv.Atoi(pod.Annotations[cloudv1.EtcdMemberIDAnnotationName])
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed read member ID annotation from etcd member with name %s", pod.Name)
		}

		if t, err := time.Parse(time.RFC3339, pod.Annotations[cloudv1.EtcdLeaderFromAnnotationName]); err == nil {
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

	if leaderID == 0 {
		// TODO: consider if and how to automatically recover from this case
		//  note: this can happen also when adding a new etcd members in the handler, might be it is something we have to take case before deletion...
		//  for now it should not be an issue because KCP forwards etcd leadership before deletion.
		return nil, nil, errors.New("invalid etcd cluster, no leader found")
	}
	statusResponse.Leader = uint64(leaderID)

	return memberList, statusResponse, nil
}
