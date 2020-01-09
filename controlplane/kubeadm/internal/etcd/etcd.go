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
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/etcdserver/etcdserverpb"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/wait"
)

const etcdTimeout = 2 * time.Second

// exponential backoff for etcd operations, values taken from kubeadm.
var etcdBackoff = wait.Backoff{
	Steps:    9,
	Duration: 50 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
}

type grpcDial = func(ctx context.Context, addr string) (net.Conn, error)

// Client provides connection parameters for an etcd cluster.
type Client struct {
	timeout       time.Duration
	backoffParams *wait.Backoff
	endpoint      string
	etcdClient    *clientv3.Client
}

// MemberAlarm represents an alarm type association with a cluster member.
type MemberAlarm struct {
	// MemberID is the ID of the member associated with the raised alarm.
	MemberID uint64

	// Type is the type of alarm which has been raised.
	Type AlarmType
}

type AlarmType int32

const (
	// AlarmOK denotes that the cluster member is OK.
	AlarmOk AlarmType = iota
	// AlarmNoSpace denotes that the cluster member has run out of disk space.
	AlarmNoSpace
	// AlarmCorrupt denotes that the cluster member has corrupted data.
	AlarmCorrupt
)

// Adapted from kubeadm

// Member struct defines an etcd member; it is used to avoid spreading
// github.com/coreos/etcd dependencies.
type Member struct {
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

// New creates a new etcd client with a custom dialer
func NewClient(dialer grpcDial, tlsConfig *tls.Config, options ...func(*Client) error) (*Client, error) {
	client := &Client{}

	for _, option := range options {
		err := option(client)
		if err != nil {
			return nil, err
		}
	}

	if client.endpoint == "" {
		client.endpoint = "localhost"
	}

	if client.timeout == 0 {
		client.timeout = etcdTimeout
	}

	if client.backoffParams == nil {
		client.backoffParams = &etcdBackoff
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{client.endpoint},
		DialTimeout: etcdTimeout,
		DialOptions: []grpc.DialOption{
			grpc.WithBlock(), // block until the underlying connection is up
			grpc.WithContextDialer(dialer),
		},
		TLS: tlsConfig,
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to create etcd client")
	}

	client.etcdClient = etcdClient

	return client, nil
}

// Members retrieves a list of etcd members.
func (c *Client) Members() (*[]Member, error) {

	resp, err := c.doCall(
		func(ctx context.Context) (interface{}, error) {
			return c.etcdClient.MemberList(ctx)
		})

	if err != nil {
		return nil, errors.Wrap(err, "failed to get list of members for etcd cluster")
	}

	membersResponse := resp.(*clientv3.MemberListResponse)

	alarms, err := c.Alarms()
	if err != nil {
		return nil, err
	}

	members := make([]Member, len(membersResponse.Members))
	for i, m := range membersResponse.Members {
		newMember := pbMemberToMember(m)
		for _, c := range *alarms {
			if c.MemberID == newMember.ID {
				newMember.Alarms = append(newMember.Alarms, c.Type)
			}
			members[i] = newMember
		}
	}

	return &members, nil
}

// pbMemberToMember converts the protobuf representation of a cluster member
// to a Member struct.
func pbMemberToMember(m *etcdserverpb.Member) Member {
	return Member{
		ID:         m.GetID(),
		Name:       m.GetName(),
		PeerURLs:   m.GetPeerURLs(),
		ClientURLs: m.GetClientURLs(),
		IsLearner:  m.GetIsLearner(),
		Alarms:     []AlarmType{},
	}
}

// Member retrieves the Member information for a given peer.
func (c *Client) Member(peerURL string) (*Member, error) {
	members, err := c.Members()
	if err != nil {
		return nil, err
	}
	for _, m := range *members {
		if m.PeerURLs[0] == peerURL {
			return &m, nil
		}
	}
	return nil, errors.Errorf("etcd member %v not found", peerURL)
}

// Status returns the cluster status as known by the connected etcd server.
func (c *Client) Status() (*clientv3.StatusResponse, error) {
	resp, err := c.doCall(
		func(ctx context.Context) (interface{}, error) {
			return c.etcdClient.Status(ctx, c.endpoint)
		})

	if err != nil {
		return nil, errors.Wrap(err, "failed to get etcd cluster status")
	}

	return resp.(*clientv3.StatusResponse), nil

}

// MoveLeader moves the leader to the provided member ID.
func (c *Client) MoveLeader(transfereeID uint64) error {
	_, err := c.doCall(
		func(ctx context.Context) (interface{}, error) {
			return c.etcdClient.MoveLeader(ctx, transfereeID)
		})

	if err != nil {
		return errors.Wrap(err, "failed to move etcd leader")
	}
	return nil
}

// RemoveMember removes a given member.
func (c *Client) RemoveMember(memberID uint64) error {
	_, err := c.doCall(
		func(ctx context.Context) (interface{}, error) {
			return c.etcdClient.MemberRemove(ctx, memberID)
		})
	if err != nil {
		return errors.Wrap(err, "failed to remove etcd member")
	}
	return nil
}

// UpdateMemberPeerList updates the given member's list of peers.
func (c *Client) UpdateMemberPeerList(id uint64, peerURLs []string) (*[]Member, error) {
	resp, err := c.doCall(
		func(ctx context.Context) (interface{}, error) {
			return c.etcdClient.MemberUpdate(ctx, id, peerURLs)
		})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to update etcd member %v's peer list to %+v", id, peerURLs)
	}

	memberUpdateResponse := resp.(*clientv3.MemberUpdateResponse)

	members := make([]Member, 0, len(memberUpdateResponse.Members))
	for _, m := range memberUpdateResponse.Members {
		members = append(members, pbMemberToMember(m))
	}

	return &members, nil
}

// Alarms retrieves all alarms on a cluster.
func (c *Client) Alarms() (*[]MemberAlarm, error) {
	resp, err := c.doCall(
		func(ctx context.Context) (interface{}, error) {
			return c.etcdClient.AlarmList(ctx)
		})

	if err != nil {
		return nil, errors.Wrap(err, "failed to get alarms for etcd cluster")
	}

	alarmResponse := resp.(*clientv3.AlarmResponse)

	memberAlarms := make([]MemberAlarm, 0, len(alarmResponse.Alarms))
	for _, a := range alarmResponse.Alarms {
		memberAlarms = append(memberAlarms, MemberAlarm{
			MemberID: a.GetMemberID(),
			Type:     AlarmType(a.GetAlarm()),
		})
	}

	return &memberAlarms, nil
}

func (c *Client) doCall(call func(context.Context) (interface{}, error)) (interface{}, error) {
	var lastError error
	var resp interface{}
	err := wait.ExponentialBackoff(*c.backoffParams, func() (bool, error) {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		var err error
		defer cancel()
		resp, err = call(ctx)
		if err == nil {
			return true, nil
		}
		lastError = err
		return false, nil
	})
	if err != nil {
		return nil, lastError
	}
	return resp, nil
}

// Close closes the etcd client.
func (c *Client) Close() {
	c.etcdClient.Close()
}

// BackoffParams sets the parameters for the client
func BackoffParams(params wait.Backoff) func(*Client) error {
	return func(c *Client) error {
		return c.setBackoffParams(params)
	}
}

func (c *Client) setBackoffParams(params wait.Backoff) error {
	c.backoffParams = &params
	return nil
}

// Endpoint changes the etcd endpoint for the client
func Endpoint(addr string) func(*Client) error {
	return func(c *Client) error {
		return c.setEndpoint(addr)
	}
}

func (c Client) setEndpoint(addr string) error {
	c.endpoint = addr
	return nil
}

// Timeout changes the method timeout for the etcd client
func Timeout(duration time.Duration) func(*Client) error {
	return func(c *Client) error {
		return c.setTimeout(duration)
	}
}

func (c Client) setTimeout(duration time.Duration) error {
	c.timeout = duration
	return nil
}
