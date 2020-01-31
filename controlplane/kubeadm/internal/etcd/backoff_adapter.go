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
	"time"

	"go.etcd.io/etcd/clientv3"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/klogr"
)

// Log is the global logger used. Global var can be swapped out if necessary.
var Log = klogr.New()

// etcdTimeout is the maximum time any individual call to the etcd client through the backoff adapter will take.
const etcdTimeout = 2 * time.Second

// etcdBackoff are default exponential backoff values for etcd operations.
// Values have been lifted from kubeadm.
var etcdBackoff = wait.Backoff{
	Steps:    9,
	Duration: 50 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
}

// EtcdBackoffAdapterOption defines the option type for the EtcdBackoffAdapter
type EtcdBackoffAdapterOption func(*EtcdBackoffAdapter)

// WithBackoff configures the backoff period of etcd calls the adapter makes.
func WithBackoff(backoff wait.Backoff) EtcdBackoffAdapterOption {
	return func(a *EtcdBackoffAdapter) {
		a.BackoffParams = backoff
	}
}

// WithTimeout will configure the backoff adapter's timeout for any etcd calls it makes.
func WithTimeout(timeout time.Duration) EtcdBackoffAdapterOption {
	return func(a *EtcdBackoffAdapter) {
		a.Timeout = timeout
	}
}

// EtcdBackoffAdapter wraps EtcdClient calls in a wait.ExponentialBackoff.
type EtcdBackoffAdapter struct {
	EtcdClient    *clientv3.Client
	BackoffParams wait.Backoff
	Timeout       time.Duration
}

// NewEtcdBackoffAdapter will wrap an etcd client with default backoff retries.
// Options allow a client to set various timeout parameters.
func NewEtcdBackoffAdapter(c *clientv3.Client, options ...EtcdBackoffAdapterOption) *EtcdBackoffAdapter {
	adapter := &EtcdBackoffAdapter{
		EtcdClient:    c,
		BackoffParams: etcdBackoff,
		Timeout:       etcdTimeout,
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

// Close calls close on the etcd client
func (e *EtcdBackoffAdapter) Close() error {
	return e.EtcdClient.Close()
}

// AlarmList calls AlarmList on the etcd client with a backoff retry.
func (e *EtcdBackoffAdapter) AlarmList(ctx context.Context) (*clientv3.AlarmResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	var alarmResponse *clientv3.AlarmResponse
	err := wait.ExponentialBackoff(e.BackoffParams, func() (bool, error) {
		resp, err := e.EtcdClient.AlarmList(ctx)
		if err != nil {
			Log.Info("failed to get etcd alarm list", "etcd client error", err)
			return false, nil
		}
		alarmResponse = resp
		return true, nil
	})
	return alarmResponse, err
}

// MemberList calls MemberList on the etcd client with a backoff retry.
func (e *EtcdBackoffAdapter) MemberList(ctx context.Context) (*clientv3.MemberListResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	var response *clientv3.MemberListResponse
	err := wait.ExponentialBackoff(e.BackoffParams, func() (bool, error) {
		resp, err := e.EtcdClient.MemberList(ctx)
		if err != nil {
			Log.Info("failed to list etcd members", "etcd client error", err)
			return false, nil
		}
		response = resp
		return true, nil
	})
	return response, err
}

// MemberUpdate calls MemberUpdate on the etcd client with a backoff retry.
func (e *EtcdBackoffAdapter) MemberUpdate(ctx context.Context, id uint64, peerURLs []string) (*clientv3.MemberUpdateResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	var response *clientv3.MemberUpdateResponse
	err := wait.ExponentialBackoff(e.BackoffParams, func() (bool, error) {
		resp, err := e.EtcdClient.MemberUpdate(ctx, id, peerURLs)
		if err != nil {
			Log.Info("failed to update etcd member", "etcd client error", err)
			return false, nil
		}
		response = resp
		return true, nil
	})
	return response, err
}

// MemberRemove calls MemberRemove on the etcd client with a backoff retry.
func (e *EtcdBackoffAdapter) MemberRemove(ctx context.Context, id uint64) (*clientv3.MemberRemoveResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	var response *clientv3.MemberRemoveResponse
	err := wait.ExponentialBackoff(e.BackoffParams, func() (bool, error) {
		resp, err := e.EtcdClient.MemberRemove(ctx, id)
		if err != nil {
			Log.Info("failed to remove etcd member", "etcd client error", err)
			return false, nil
		}
		response = resp
		return true, nil
	})
	return response, err
}

// MoveLeader calls MoveLeader on the etcd client with a backoff retry.
func (e *EtcdBackoffAdapter) MoveLeader(ctx context.Context, id uint64) (*clientv3.MoveLeaderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	var response *clientv3.MoveLeaderResponse
	err := wait.ExponentialBackoff(e.BackoffParams, func() (bool, error) {
		resp, err := e.EtcdClient.MoveLeader(ctx, id)
		if err != nil {
			Log.Info("failed to move etcd member", "etcd client error", err)
			return false, nil
		}
		response = resp
		return true, nil
	})
	return response, err
}
