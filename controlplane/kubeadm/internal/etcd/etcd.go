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

package etcd

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/pkg/errors"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/proxy"
)

// etcdTimeout is the maximum time any individual call to the etcd client through the backoff adapter will take.
const etcdTimeout = 2 * time.Second

// GRPCDial is a function that creates a connection to a given endpoint.
type GRPCDial func(ctx context.Context, addr string) (net.Conn, error)

// etcd wraps the etcd client from etcd's clientv3 package.
// This interface is implemented by both the clientv3 package and the backoff adapter that adds retries to the client.
type etcd interface {
	AlarmList(ctx context.Context) (*clientv3.AlarmResponse, error)
	Close() error
	Endpoints() []string
	MemberList(ctx context.Context) (*clientv3.MemberListResponse, error)
	MemberRemove(ctx context.Context, id uint64) (*clientv3.MemberRemoveResponse, error)
	MemberUpdate(ctx context.Context, id uint64, peerURLs []string) (*clientv3.MemberUpdateResponse, error)
	MoveLeader(ctx context.Context, id uint64) (*clientv3.MoveLeaderResponse, error)
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
}

// Client wraps an etcd client formatting its output to something more consumable.
type Client struct {
	EtcdClient etcd
	Endpoint   string
	LeaderID   uint64
	Errors     []string
}

// MemberAlarm represents an alarm type association with a cluster member.
type MemberAlarm struct {
	// MemberID is the ID of the member associated with the raised alarm.
	MemberID uint64

	// Type is the type of alarm which has been raised.
	Type AlarmType
}

// AlarmType defines the type of alarm for etcd.
type AlarmType int32

const (
	// AlarmOK denotes that the cluster member is OK.
	AlarmOK AlarmType = iota

	// AlarmNoSpace denotes that the cluster member has run out of disk space.
	AlarmNoSpace

	// AlarmCorrupt denotes that the cluster member has corrupted data.
	AlarmCorrupt
)

// AlarmTypeName provides a text translation for AlarmType codes.
var AlarmTypeName = map[AlarmType]string{
	AlarmOK:      "NONE",
	AlarmNoSpace: "NOSPACE",
	AlarmCorrupt: "CORRUPT",
}

// Adapted from kubeadm.

// Member struct defines an etcd member; it is used to avoid spreading
// github.com/coreos/etcd dependencies.
type Member struct {
	// ClusterID is the ID of the cluster to which this member belongs
	ClusterID uint64

	// ID is the ID of this cluster member
	ID uint64

	// Name is the human-readable name of the member. If the member is not started, the name will be an empty string.
	Name string

	// PeerURLs is the list of URLs the member exposes to the cluster for communication.
	PeerURLs []string

	// ClientURLs is the list of URLs the member exposes to clients for communication. If the member is not started, clientURLs will be empty.
	ClientURLs []string

	// IsLearner indicates if the member is raft learner.
	IsLearner bool

	// Alarms is the list of alarms for a member.
	Alarms []AlarmType
}

// pbMemberToMember converts the protobuf representation of a cluster member to a Member struct.
func pbMemberToMember(m *etcdserverpb.Member) *Member {
	return &Member{
		ID:         m.GetID(),
		Name:       m.GetName(),
		PeerURLs:   m.GetPeerURLs(),
		ClientURLs: m.GetClientURLs(),
		IsLearner:  m.GetIsLearner(),
		Alarms:     []AlarmType{},
	}
}

// NewClient creates a new etcd client with a proxy, and a TLS configuration.
func NewClient(ctx context.Context, endpoints []string, p proxy.Proxy, tlsConfig *tls.Config) (*Client, error) {
	dialer, err := proxy.NewDialer(p)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create a dialer for etcd client")
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: etcdTimeout,
		DialOptions: []grpc.DialOption{
			grpc.WithBlock(), // block until the underlying connection is up
			grpc.WithContextDialer(dialer.DialContextWithAddr),
		},
		TLS: tlsConfig,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create etcd client")
	}

	return newEtcdClient(ctx, etcdClient)
}

func newEtcdClient(ctx context.Context, etcdClient etcd) (*Client, error) {
	endpoints := etcdClient.Endpoints()
	if len(endpoints) == 0 {
		return nil, errors.New("etcd client was not configured with any endpoints")
	}

	status, err := etcdClient.Status(ctx, endpoints[0])
	if err != nil {
		return nil, err
	}

	return &Client{
		Endpoint:   endpoints[0],
		EtcdClient: etcdClient,
		LeaderID:   status.Leader,
		Errors:     status.Errors,
	}, nil
}

// Close closes the etcd client.
func (c *Client) Close() error {
	return c.EtcdClient.Close()
}

// Members retrieves a list of etcd members.
func (c *Client) Members(ctx context.Context) ([]*Member, error) {
	response, err := c.EtcdClient.MemberList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get list of members for etcd cluster")
	}

	alarms, err := c.Alarms(ctx)
	if err != nil {
		return nil, err
	}

	clusterID := response.Header.GetClusterId()
	members := make([]*Member, 0)
	for _, m := range response.Members {
		newMember := pbMemberToMember(m)
		newMember.ClusterID = clusterID
		for _, c := range alarms {
			if c.MemberID == newMember.ID {
				newMember.Alarms = append(newMember.Alarms, c.Type)
			}
		}
		members = append(members, newMember)
	}

	return members, nil
}

// MoveLeader moves the leader to the provided member ID.
func (c *Client) MoveLeader(ctx context.Context, newLeaderID uint64) error {
	_, err := c.EtcdClient.MoveLeader(ctx, newLeaderID)
	return errors.Wrapf(err, "failed to move etcd leader: %v", newLeaderID)
}

// RemoveMember removes a given member.
func (c *Client) RemoveMember(ctx context.Context, id uint64) error {
	_, err := c.EtcdClient.MemberRemove(ctx, id)
	return errors.Wrapf(err, "failed to remove member: %v", id)
}

// UpdateMemberPeerURLs updates the list of peer URLs.
func (c *Client) UpdateMemberPeerURLs(ctx context.Context, id uint64, peerURLs []string) ([]*Member, error) {
	response, err := c.EtcdClient.MemberUpdate(ctx, id, peerURLs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update etcd member %v's peer list to %+v", id, peerURLs)
	}

	members := make([]*Member, 0, len(response.Members))
	for _, m := range response.Members {
		members = append(members, pbMemberToMember(m))
	}

	return members, nil
}

// Alarms retrieves all alarms on a cluster.
func (c *Client) Alarms(ctx context.Context) ([]MemberAlarm, error) {
	alarmResponse, err := c.EtcdClient.AlarmList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get alarms for etcd cluster")
	}

	memberAlarms := make([]MemberAlarm, 0, len(alarmResponse.Alarms))
	for _, a := range alarmResponse.Alarms {
		memberAlarms = append(memberAlarms, MemberAlarm{
			MemberID: a.GetMemberID(),
			Type:     AlarmType(a.GetAlarm()),
		})
	}

	return memberAlarms, nil
}
