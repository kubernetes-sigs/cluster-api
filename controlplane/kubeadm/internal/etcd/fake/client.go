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

type FakeKVClient struct {
	GetResponse *clientv3.GetResponse
}

func (kv *FakeKVClient) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	return nil, nil
}
func (kv *FakeKVClient) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return kv.GetResponse, nil
}
func (kv *FakeKVClient) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	return nil, nil
}
func (kv *FakeKVClient) Compact(ctx context.Context, rev int64, opts ...clientv3.CompactOption) (*clientv3.CompactResponse, error) {
	return nil, nil
}

func (kv *FakeKVClient) Do(ctx context.Context, op clientv3.Op) (clientv3.OpResponse, error) {
	return clientv3.OpResponse{}, nil
}

func (kv *FakeKVClient) Txn(ctx context.Context) clientv3.Txn {
	return nil
}

type FakeEtcdClient struct {
	AlarmResponse        *clientv3.AlarmResponse
	EtcdEndpoints        []string
	MemberListResponse   *clientv3.MemberListResponse
	MemberRemoveResponse *clientv3.MemberRemoveResponse
	MemberUpdateResponse *clientv3.MemberUpdateResponse
	MoveLeaderResponse   *clientv3.MoveLeaderResponse
	StatusResponse       *clientv3.StatusResponse
	ErrorResponse        error
	MovedLeader          uint64
	RemovedMember        uint64
}

func (c *FakeEtcdClient) Endpoints() []string {
	return c.EtcdEndpoints
}

func (c *FakeEtcdClient) MoveLeader(_ context.Context, i uint64) (*clientv3.MoveLeaderResponse, error) {
	c.MovedLeader = i
	return c.MoveLeaderResponse, c.ErrorResponse
}

func (c *FakeEtcdClient) Close() error {
	return nil
}

func (c *FakeEtcdClient) AlarmList(_ context.Context) (*clientv3.AlarmResponse, error) {
	return c.AlarmResponse, c.ErrorResponse
}

func (c *FakeEtcdClient) MemberList(_ context.Context) (*clientv3.MemberListResponse, error) {
	return c.MemberListResponse, c.ErrorResponse
}
func (c *FakeEtcdClient) MemberRemove(_ context.Context, i uint64) (*clientv3.MemberRemoveResponse, error) {
	c.RemovedMember = i
	return c.MemberRemoveResponse, c.ErrorResponse
}
func (c *FakeEtcdClient) MemberUpdate(_ context.Context, _ uint64, _ []string) (*clientv3.MemberUpdateResponse, error) {
	return c.MemberUpdateResponse, c.ErrorResponse
}
func (c *FakeEtcdClient) Status(_ context.Context, _ string) (*clientv3.StatusResponse, error) {
	return c.StatusResponse, nil
}
