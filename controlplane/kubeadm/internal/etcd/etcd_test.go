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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	ctrl "sigs.k8s.io/controller-runtime"

	etcdfake "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func TestEtcdMembers_WithErrors(t *testing.T) {
	g := NewWithT(t)

	fakeEtcdClient := &etcdfake.FakeEtcdClient{
		EtcdEndpoints: []string{"https://etcd-instance:2379"},
		MemberListResponse: &clientv3.MemberListResponse{
			Header: &etcdserverpb.ResponseHeader{},
			Members: []*etcdserverpb.Member{
				{ID: 1234, Name: "foo", PeerURLs: []string{"https://1.2.3.4:2000"}},
			},
		},
		MoveLeaderResponse:   &clientv3.MoveLeaderResponse{},
		MemberRemoveResponse: &clientv3.MemberRemoveResponse{},
		StatusResponse:       &clientv3.StatusResponse{},
		ErrorResponse:        errors.New("something went wrong"),
	}

	client, err := newEtcdClient(ctx, fakeEtcdClient, DefaultCallTimeout)
	g.Expect(err).ToNot(HaveOccurred())

	members, err := client.Members(ctx)
	g.Expect(err).To(HaveOccurred())
	g.Expect(members).To(BeEmpty())

	err = client.MoveLeader(ctx, 1)
	g.Expect(err).To(HaveOccurred())

	err = client.RemoveMember(ctx, 1234)
	g.Expect(err).To(HaveOccurred())
}

func TestEtcdMembers_WithSuccess(t *testing.T) {
	g := NewWithT(t)

	fakeEtcdClient := &etcdfake.FakeEtcdClient{
		EtcdEndpoints: []string{"https://etcd-instance:2379"},
		MemberListResponse: &clientv3.MemberListResponse{
			Header: &etcdserverpb.ResponseHeader{},
			Members: []*etcdserverpb.Member{
				{ID: 1234, Name: "foo", PeerURLs: []string{"https://1.2.3.4:2000"}},
			},
		},
		MoveLeaderResponse: &clientv3.MoveLeaderResponse{},
		MemberUpdateResponse: &clientv3.MemberUpdateResponse{
			Header: &etcdserverpb.ResponseHeader{},
			Members: []*etcdserverpb.Member{
				{ID: 1234, Name: "foo", PeerURLs: []string{"https://1.2.3.4:2000", "https://4.5.6.7:2000"}},
			},
		},
		MemberRemoveResponse: &clientv3.MemberRemoveResponse{},
		AlarmResponse:        &clientv3.AlarmResponse{},
		StatusResponse:       &clientv3.StatusResponse{},
	}

	client, err := newEtcdClient(ctx, fakeEtcdClient, DefaultCallTimeout)
	g.Expect(err).ToNot(HaveOccurred())

	members, err := client.Members(ctx)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(members).To(HaveLen(1))

	err = client.MoveLeader(ctx, 1)
	g.Expect(err).ToNot(HaveOccurred())

	err = client.RemoveMember(ctx, 1234)
	g.Expect(err).ToNot(HaveOccurred())

	updatedMembers, err := client.UpdateMemberPeerURLs(ctx, 1234, []string{"https://4.5.6.7:2000"})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updatedMembers[0].PeerURLs).To(HaveLen(2))
	g.Expect(updatedMembers[0].PeerURLs).To(Equal([]string{"https://1.2.3.4:2000", "https://4.5.6.7:2000"}))
}
