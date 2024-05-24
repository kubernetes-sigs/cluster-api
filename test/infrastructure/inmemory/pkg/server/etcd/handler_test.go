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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"google.golang.org/grpc/metadata"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
)

func Test_etcd_scalingflow(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	// During a scale down event - for example during upgrade - KCP will call `MoveLeader` and `MemberRemove` in sequence.
	g := NewWithT(t)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.New(map[string]string{":authority": "etcd-1"}))
	manager := inmemoryruntime.NewManager(scheme)
	resourceGroupResolver := func(string) (string, error) { return "group1", nil }
	c := &clusterServerServer{
		baseServer: &baseServer{
			log:                   log.FromContext(ctx),
			manager:               manager,
			resourceGroupResolver: resourceGroupResolver,
		},
	}

	m := &maintenanceServer{
		baseServer: &baseServer{
			log:                   log.FromContext(ctx),
			manager:               manager,
			resourceGroupResolver: resourceGroupResolver,
		},
	}
	c.manager.AddResourceGroup("group1")
	inmemoryClient := c.manager.GetResourceGroup("group1").GetClient()

	for i := 1; i <= 3; i++ {
		etcdMember := fmt.Sprintf("etcd-%d", i)
		etcdPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: metav1.NamespaceSystem,
				Name:      etcdMember,
				Labels: map[string]string{
					"component": "etcd",
					"tier":      "control-plane",
				},
				Annotations: map[string]string{
					cloudv1.EtcdMemberIDAnnotationName:  fmt.Sprintf("%d", i),
					cloudv1.EtcdClusterIDAnnotationName: "15",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: fmt.Sprintf("etcd-%d", i),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		// Initially set leader to `etcd-1`
		if i == 1 {
			etcdPod.Annotations[cloudv1.EtcdLeaderFromAnnotationName] = time.Date(2020, 07, 03, 14, 25, 58, 651387237, time.UTC).Format(time.RFC3339)
		}
		g.Expect(inmemoryClient.Create(ctx, etcdPod)).To(Succeed())
	}
	var etcdMemberToRemove uint64 = 2
	var etcdMemberToBeLeader uint64 = 3

	t.Run("move leader and remove etcd member", func(*testing.T) {
		_, err := m.MoveLeader(ctx, &pb.MoveLeaderRequest{TargetID: etcdMemberToBeLeader})
		g.Expect(err).NotTo(HaveOccurred())

		_, err = c.MemberRemove(ctx, &pb.MemberRemoveRequest{ID: etcdMemberToRemove})
		g.Expect(err).NotTo(HaveOccurred())

		// Expect the inspect call to fail on a member which has been removed.
		_, _, err = c.inspectEtcd(ctx, inmemoryClient, fmt.Sprintf("%d", etcdMemberToRemove))
		g.Expect(err).To(HaveOccurred())

		// inspectEtcd should succeed when calling on a member that has not been removed.
		members, status, err := c.inspectEtcd(ctx, inmemoryClient, fmt.Sprintf("%d", etcdMemberToBeLeader))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(status.Leader).To(Equal(etcdMemberToBeLeader))
		g.Expect(members.GetMembers()).To(HaveLen(2))
		g.Expect(members.GetMembers()).NotTo(ContainElement(fmt.Sprintf("etcd-%d", etcdMemberToRemove)))
	})
}
