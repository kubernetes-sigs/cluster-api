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
	"go.uber.org/zap"
	"google.golang.org/grpc"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/proxy"
)

// GRPCDial is a function that creates a connection to a given endpoint.
type GRPCDial func(ctx context.Context, addr string) (net.Conn, error)

// etcd wraps the etcd client from etcd's clientv3 package.
// This interface is implemented by both the clientv3 package and the backoff adapter that adds retries to the client.
type etcd interface {
	AlarmList(ctx context.Context) (*clientv3.AlarmResponse, error)
	AlarmDisarm(ctx context.Context, m *clientv3.AlarmMember) (*clientv3.AlarmResponse, error)
	Close() error
	Defragment(ctx context.Context, endpoint string) (*clientv3.DefragmentResponse, error)
	Endpoints() []string
	MemberList(ctx context.Context, opts ...clientv3.OpOption) (*clientv3.MemberListResponse, error)
	MemberRemove(ctx context.Context, id uint64) (*clientv3.MemberRemoveResponse, error)
	MoveLeader(ctx context.Context, id uint64) (*clientv3.MoveLeaderResponse, error)
	Status(ctx context.Context, endpoint string) (*clientv3.StatusResponse, error)
}

// Client wraps an etcd client formatting its output to something more consumable.
type Client struct {
	EtcdClient  etcd
	Endpoint    string
	LeaderID    uint64
	CallTimeout time.Duration
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

// DefaultCallTimeout represents the duration that the etcd client waits at most
// for read and write operations to etcd.
const DefaultCallTimeout = 15 * time.Second

// defragCallTimeout is the timeout used for etcd defragmentation calls.
// Defragmentation is a write-heavy, long-running operation that can take significantly
// longer than regular read/write calls, so it uses a dedicated larger timeout.
// The effective deadline is the minimum of defragCallTimeout and any deadline already
// set on the parent context (e.g. the reconciliation timeout).
//
// Note: if the client-side context expires before the server acknowledges completion,
// the client will receive a deadline-exceeded error, but the etcd server will continue
// and finish the defragmentation regardless, because the server-side operation is not
// interruptible. A timeout error here therefore means the result is unknown, not that
// defragmentation failed.
const defragCallTimeout = 60 * time.Second

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
}

// Members is a slice of Member pointers that implements sort.Interface, ordering by member name.
type Members []*Member

func (m Members) Len() int           { return len(m) }
func (m Members) Less(i, j int) bool { return m[i].Name < m[j].Name }
func (m Members) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }

// pbMemberToMember converts the protobuf representation of a cluster member to a Member struct.
func pbMemberToMember(m *etcdserverpb.Member) *Member {
	return &Member{
		ID:         m.GetID(),
		Name:       m.GetName(),
		PeerURLs:   m.GetPeerURLs(),
		ClientURLs: m.GetClientURLs(),
		IsLearner:  m.GetIsLearner(),
	}
}

// ClientConfiguration describes the configuration for an etcd client.
type ClientConfiguration struct {
	Endpoint    string
	Proxy       proxy.Proxy
	TLSConfig   *tls.Config
	DialTimeout time.Duration
	CallTimeout time.Duration
	Logger      *zap.Logger
}

// NewClient creates a new etcd client with the given configuration.
func NewClient(ctx context.Context, config ClientConfiguration) (*Client, error) {
	dialer, err := proxy.NewDialer(config.Proxy)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to create a dialer for the etcd client connecting to %s", config.Endpoint)
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{config.Endpoint}, // NOTE: endpoint is used only as a host for certificate validation, the network connection is defined by DialOptions.
		DialTimeout: config.DialTimeout,
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(dialer.DialContextWithAddr),
		},
		TLS:    config.TLSConfig,
		Logger: config.Logger,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create etcd client")
	}

	callTimeout := config.CallTimeout
	if callTimeout == 0 {
		callTimeout = DefaultCallTimeout
	}

	client, err := newEtcdClient(ctx, etcdClient, callTimeout)
	if err != nil {
		closeErr := etcdClient.Close()
		return nil, kerrors.NewAggregate([]error{err, closeErr})
	}
	return client, nil
}

func newEtcdClient(ctx context.Context, etcdClient etcd, callTimeout time.Duration) (*Client, error) {
	endpoints := etcdClient.Endpoints()
	if len(endpoints) == 0 {
		return nil, errors.New("invalid argument: newEtcdClient cannot be called without any endpoint")
	}

	ctx, cancel := context.WithTimeoutCause(ctx, callTimeout, errors.New("call timeout expired"))
	defer cancel()

	status, err := etcdClient.Status(ctx, endpoints[0])
	if err != nil {
		return nil, errors.Wrap(err, "failed to get etcd status")
	}

	// We don't need to read or handle StatusResponse.Errors here. They
	// are intended for human consumption, not for programmatic processing.
	// KCP should rely only on alarms.
	return &Client{
		Endpoint:    endpoints[0],
		EtcdClient:  etcdClient,
		LeaderID:    status.Leader,
		CallTimeout: callTimeout,
	}, nil
}

// Close closes the etcd client.
func (c *Client) Close() error {
	return c.EtcdClient.Close()
}

// Members retrieves a list of etcd members.
func (c *Client) Members(ctx context.Context) ([]*Member, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, c.CallTimeout, errors.New("call timeout expired"))
	defer cancel()

	response, err := c.EtcdClient.MemberList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get etcd members")
	}

	clusterID := response.Header.GetClusterId()
	members := make([]*Member, 0)
	for _, m := range response.Members {
		newMember := pbMemberToMember(m)
		newMember.ClusterID = clusterID
		members = append(members, newMember)
	}

	return members, nil
}

// MoveLeader moves the leader to the provided member ID.
func (c *Client) MoveLeader(ctx context.Context, newLeaderID uint64) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.CallTimeout, errors.New("call timeout expired"))
	defer cancel()

	_, err := c.EtcdClient.MoveLeader(ctx, newLeaderID)
	return errors.Wrapf(err, "failed to move etcd leader to: %v", newLeaderID)
}

// RemoveMember removes a given member.
func (c *Client) RemoveMember(ctx context.Context, id uint64) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.CallTimeout, errors.New("call timeout expired"))
	defer cancel()

	_, err := c.EtcdClient.MemberRemove(ctx, id)
	return errors.Wrapf(err, "failed to remove etcd member: %v", id)
}

// MemberStatus holds per-member status information derived from the etcd Status RPC.
// It mirrors the non-protobuf fields of clientv3.StatusResponse (which is pb.StatusResponse),
// omitting Header and DowngradeInfo (both protobuf types) and exposing Header.MemberId as ID.
// Callers can determine leadership by comparing ID == Leader.
type MemberStatus struct {
	// ID is the member ID of this member, taken from Header.MemberId in the Status response.
	ID uint64

	// Version is the cluster protocol version used by this member.
	Version string

	// StorageVersion is the version of the db file on this member.
	StorageVersion string

	// DbSize is the total size of the etcd database file, in bytes.
	DbSize int64

	// DbSizeInUse is the total size actually in use in the etcd database, in bytes.
	DbSizeInUse int64

	// DbSizeQuota is the configured etcd storage quota in bytes, populated from the
	// etcd Status response on etcd v3.6+. On etcd v3.5 this field is always 0.
	DbSizeQuota int64

	// RaftIndex is the current raft committed index of this member.
	RaftIndex uint64

	// RaftTerm is the current raft term of this member.
	RaftTerm uint64

	// RaftAppliedIndex is the current raft applied index of this member.
	RaftAppliedIndex uint64

	// Leader is the member ID that this member believes is the current leader.
	Leader uint64

	// IsLearner indicates whether this member is a raft learner.
	IsLearner bool

	// Errors contains alarm/health information reported by this member.
	Errors []string
}

// MemberStatus fetches a fresh status for this client's endpoint and returns database
// size metrics and leadership information.
func (c *Client) MemberStatus(ctx context.Context) (*MemberStatus, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, c.CallTimeout, errors.New("call timeout expired"))
	defer cancel()

	resp, err := c.EtcdClient.Status(ctx, c.Endpoint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get etcd member status")
	}
	return &MemberStatus{
		ID:               resp.Header.GetMemberId(),
		Version:          resp.Version,
		StorageVersion:   resp.StorageVersion,
		DbSize:           resp.DbSize,
		DbSizeInUse:      resp.DbSizeInUse,
		DbSizeQuota:      resp.DbSizeQuota,
		RaftIndex:        resp.RaftIndex,
		RaftTerm:         resp.RaftTerm,
		RaftAppliedIndex: resp.RaftAppliedIndex,
		Leader:           resp.Leader,
		IsLearner:        resp.IsLearner,
		Errors:           resp.Errors,
	}, nil
}

// Defragment defragments this client's etcd member, reclaiming space freed by prior compactions.
// This is a blocking, write-heavy operation; it uses defragCallTimeout rather than the
// standard CallTimeout to accommodate large databases.
func (c *Client) Defragment(ctx context.Context) error {
	ctx, cancel := context.WithTimeoutCause(ctx, defragCallTimeout, errors.New("defrag call timeout expired"))
	defer cancel()

	_, err := c.EtcdClient.Defragment(ctx, c.Endpoint)
	return errors.Wrapf(err, "failed to defragment etcd member %s", c.Endpoint)
}

// DisarmNoSpaceAlarm clears the NOSPACE alarm on the given etcd member.
func (c *Client) DisarmNoSpaceAlarm(ctx context.Context, memberID uint64) error {
	ctx, cancel := context.WithTimeoutCause(ctx, c.CallTimeout, errors.New("call timeout expired"))
	defer cancel()

	_, err := c.EtcdClient.AlarmDisarm(ctx, &clientv3.AlarmMember{
		MemberID: memberID,
		Alarm:    etcdserverpb.AlarmType_NOSPACE,
	})
	return errors.Wrapf(err, "failed to disarm NOSPACE alarm on etcd member %d", memberID)
}

// Alarms retrieves all alarms on a cluster.
func (c *Client) Alarms(ctx context.Context) ([]MemberAlarm, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, c.CallTimeout, errors.New("call timeout expired"))
	defer cancel()

	alarmResponse, err := c.EtcdClient.AlarmList(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get etcd alarms")
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
