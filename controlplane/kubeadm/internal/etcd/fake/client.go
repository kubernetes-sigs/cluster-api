package fake

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

type FakeEtcdClient struct {
	leaderID uint64
	members  map[uint64]*etcd.Member
	healthy  map[uint64]bool
	alarms   []etcd.MemberAlarm
}

func NewClient() *FakeEtcdClient {
	c := FakeEtcdClient{
		members: make(map[uint64]*etcd.Member),
		healthy: make(map[uint64]bool),
	}
	return &c
}

// Receivers that manipulate the state of the fake etcd cluster.

func (c *FakeEtcdClient) AddMember(memberID uint64, peerURLs []string) error {
	_, ok := c.members[memberID]
	if ok {
		return fmt.Errorf("member with ID %d already exists", memberID)
	}
	c.members[memberID] = &etcd.Member{
		ID:       memberID,
		PeerURLs: peerURLs,
	}
	c.healthy[memberID] = false
	return nil
}

func (c *FakeEtcdClient) StartMember(memberID uint64, name string, clientURLs []string) error {
	m, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	m.Name = name
	m.ClientURLs = clientURLs
	c.healthy[memberID] = true
	return nil
}

func (c *FakeEtcdClient) SetHealthy(memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	c.healthy[memberID] = true
	return nil
}

func (c *FakeEtcdClient) SetUnhealthy(memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	c.healthy[memberID] = false
	return nil
}

func (c *FakeEtcdClient) SetLeader(memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	c.leaderID = memberID
	return nil
}

func (c *FakeEtcdClient) Leader() (*etcd.Member, error) {
	leader, ok := c.members[c.leaderID]
	if !ok {
		return nil, errors.New("cluster has no leader")
	}
	return leader, nil
}

func (c *FakeEtcdClient) SetAlarm(alarmType etcd.AlarmType, memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	for _, a := range c.alarms {
		if a.Type == alarmType && a.MemberID == memberID {
			// Alarm is already set
			return nil
		}
	}
	mAlarm := etcd.MemberAlarm{
		MemberID: memberID,
		Type:     alarmType,
	}
	c.alarms = append(c.alarms, mAlarm)
	return nil
}

func (c *FakeEtcdClient) ClearAlarm(alarmType etcd.AlarmType, memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	indexToDelete := -1
	for i, a := range c.alarms {
		if a.Type == alarmType && a.MemberID == memberID {
			indexToDelete = i
			break
		}
	}
	if indexToDelete >= 0 {
		c.alarms = append(c.alarms[:indexToDelete], c.alarms[indexToDelete:]...)
	}
	return nil
}

// Receivers that implement the controllers.EtcdClient interface.

func (c *FakeEtcdClient) Close() error {
	return nil
}

func (c *FakeEtcdClient) Members(ctx context.Context) ([]*etcd.Member, error) {
	members := []*etcd.Member{}
	for i := range c.members {
		members = append(members, c.members[i])
	}
	return members, nil
}

func (c *FakeEtcdClient) MoveLeader(ctx context.Context, memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	c.leaderID = memberID
	return nil
}

func (c *FakeEtcdClient) RemoveMember(ctx context.Context, memberID uint64) error {
	_, ok := c.members[memberID]
	if !ok {
		return fmt.Errorf("no member with ID %d", memberID)
	}
	if c.leaderID == memberID {
		c.leaderID = 0
	}
	delete(c.members, memberID)
	return nil
}

func (c *FakeEtcdClient) UpdateMemberPeerURLs(ctx context.Context, memberID uint64, peerURLs []string) ([]*etcd.Member, error) {
	m, ok := c.members[memberID]
	if !ok {
		return nil, fmt.Errorf("no member with ID %d", memberID)
	}
	m.PeerURLs = peerURLs
	return c.Members(ctx)
}

func (c *FakeEtcdClient) Alarms(ctx context.Context) ([]etcd.MemberAlarm, error) {
	alarms := []etcd.MemberAlarm{}
	for i := range c.alarms {
		alarms = append(alarms, c.alarms[i])
	}
	return alarms, nil
}
