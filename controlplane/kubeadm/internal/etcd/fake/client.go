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

package fake

import (
	"context"

	"go.etcd.io/etcd/clientv3"
)

type FakeEtcdClient struct {
	AlarmResponse        *clientv3.AlarmResponse
	EtcdEndpoints        []string
	MemberListResponse   *clientv3.MemberListResponse
	MemberRemoveResponse *clientv3.MemberRemoveResponse
	MemberUpdateResponse *clientv3.MemberUpdateResponse
	MoveLeaderResponse   *clientv3.MoveLeaderResponse
	StatusResponse       *clientv3.StatusResponse
}

func (c *FakeEtcdClient) Endpoints() []string {
	return c.EtcdEndpoints
}

func (c *FakeEtcdClient) MoveLeader(_ context.Context, _ uint64) (*clientv3.MoveLeaderResponse, error) {
	return c.MoveLeaderResponse, nil
}

func (c *FakeEtcdClient) Close() error {
	return nil
}

func (c *FakeEtcdClient) AlarmList(_ context.Context) (*clientv3.AlarmResponse, error) {
	return c.AlarmResponse, nil
}

func (c *FakeEtcdClient) MemberList(_ context.Context) (*clientv3.MemberListResponse, error) {
	return c.MemberListResponse, nil
}
func (c *FakeEtcdClient) MemberRemove(_ context.Context, _ uint64) (*clientv3.MemberRemoveResponse, error) {
	return c.MemberRemoveResponse, nil
}
func (c *FakeEtcdClient) MemberUpdate(_ context.Context, _ uint64, _ []string) (*clientv3.MemberUpdateResponse, error) {
	return c.MemberUpdateResponse, nil
}
func (c *FakeEtcdClient) Status(_ context.Context, _ string) (*clientv3.StatusResponse, error) {
	return c.StatusResponse, nil
}
