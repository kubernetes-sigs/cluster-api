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

package internal

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/blang/semver"
	"go.etcd.io/etcd/clientv3"
	pb "go.etcd.io/etcd/etcdserver/etcdserverpb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	fake2 "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/fake"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCluster_ReconcileKubeletRBACBinding_NoError(t *testing.T) {
	tests := []struct {
		name   string
		client ctrlclient.Client
	}{
		{
			name: "role binding and role already exist",
			client: &fakeClient{
				get: map[string]interface{}{
					"kube-system/kubeadm:kubelet-config-1.12": &rbacv1.RoleBinding{},
					"kube-system/kubeadm:kubelet-config-1.13": &rbacv1.Role{},
				},
			},
		},
		{
			name:   "role binding and role don't exist",
			client: &fakeClient{},
		},
		{
			name: "create returns an already exists error",
			client: &fakeClient{
				createErr: apierrors.NewAlreadyExists(schema.GroupResource{}, ""),
			},
		},
	}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &Workload{
				Client: tt.client,
			}
			g.Expect(c.ReconcileKubeletRBACBinding(ctx, semver.MustParse("1.12.3"))).To(Succeed())
			g.Expect(c.ReconcileKubeletRBACRole(ctx, semver.MustParse("1.13.3"))).To(Succeed())
		})
	}
}

func TestCluster_ReconcileKubeletRBACBinding_Error(t *testing.T) {
	tests := []struct {
		name   string
		client ctrlclient.Client
	}{
		{
			name: "client fails to retrieve an expected error or the role binding/role",
			client: &fakeClient{
				getErr: errors.New(""),
			},
		},
		{
			name: "fails to create the role binding/role",
			client: &fakeClient{
				createErr: errors.New(""),
			},
		},
	}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &Workload{
				Client: tt.client,
			}
			g.Expect(c.ReconcileKubeletRBACBinding(ctx, semver.MustParse("1.12.3"))).NotTo(Succeed())
			g.Expect(c.ReconcileKubeletRBACRole(ctx, semver.MustParse("1.13.3"))).NotTo(Succeed())
		})
	}
}

func newKubeProxyDS() appsv1.DaemonSet {
	return appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeProxyKey,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "k8s.gcr.io/kube-proxy:v1.16.2",
							Name:  "kube-proxy",
						},
					},
				},
			},
		},
	}
}

func newKubeProxyDSWithImage(image string) appsv1.DaemonSet {
	ds := newKubeProxyDS()
	ds.Spec.Template.Spec.Containers[0].Image = image
	return ds
}

func TestUpdateKubeProxyImageInfo(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name        string
		ds          appsv1.DaemonSet
		expectErr   bool
		expectImage string
		clientGet   map[string]interface{}
		patchErr    error
		KCP         *v1alpha3.KubeadmControlPlane
	}{
		{
			name:        "succeeds if patch correctly",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "returns error if image in kube-proxy ds was in digest format",
			ds:          newKubeProxyDSWithImage("k8s.gcr.io/kube-proxy@sha256:47bfd"),
			expectErr:   true,
			expectImage: "k8s.gcr.io/kube-proxy@sha256:47bfd",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "expects OCI compatible format of tag",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3_build1",
			KCP:         &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3+build1"}},
		},
		{
			name:      "returns error if image in kube-proxy ds was in wrong format",
			ds:        newKubeProxyDSWithImage(""),
			expectErr: true,
			KCP:       &v1alpha3.KubeadmControlPlane{Spec: v1alpha3.KubeadmControlPlaneSpec{Version: "v1.16.3"}},
		},
		{
			name:        "updates image repository if one has been set on the control plane",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "foo.bar.example/baz/qux/kube-proxy:v1.16.3",
			KCP: &v1alpha3.KubeadmControlPlane{
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
							ImageRepository: "foo.bar.example/baz/qux",
						},
					},
				}},
		},
		{
			name:        "does not update image repository if it is blank",
			ds:          newKubeProxyDS(),
			expectErr:   false,
			expectImage: "k8s.gcr.io/kube-proxy:v1.16.3",
			KCP: &v1alpha3.KubeadmControlPlane{
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
							ImageRepository: "",
						},
					},
				}},
		},
		{
			name:      "returns error if image repository is invalid",
			ds:        newKubeProxyDS(),
			expectErr: true,
			KCP: &v1alpha3.KubeadmControlPlane{
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
					KubeadmConfigSpec: cabpkv1.KubeadmConfigSpec{
						ClusterConfiguration: &kubeadmv1beta1.ClusterConfiguration{
							ImageRepository: "%%%",
						},
					},
				}},
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			objects := []runtime.Object{
				&tt.ds,
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme, objects...)
			w := &Workload{
				Client: fakeClient,
			}
			err := w.UpdateKubeProxyImageInfo(ctx, tt.KCP)
			if tt.expectErr {
				gs.Expect(err).To(HaveOccurred())
			} else {
				gs.Expect(err).NotTo(HaveOccurred())
			}

			proxyImage, err := getProxyImageInfo(ctx, w.Client)
			gs.Expect(err).NotTo(HaveOccurred())
			if tt.expectImage != "" {
				gs.Expect(proxyImage).To(Equal(tt.expectImage))
			}
		})
	}
}

func getProxyImageInfo(ctx context.Context, client ctrlclient.Client) (string, error) {
	ds := &appsv1.DaemonSet{}

	if err := client.Get(ctx, ctrlclient.ObjectKey{Name: kubeProxyKey, Namespace: metav1.NamespaceSystem}, ds); err != nil {
		if apierrors.IsNotFound(err) {
			return "", errors.New("no image found")
		}
		return "", errors.New("failed to determine if daemonset already exists")
	}
	container := findKubeProxyContainer(ds)
	if container == nil {
		return "", errors.New("unable to find container")
	}
	return container.Image, nil
}

func TestWorkload_EtcdIsHealthy(t *testing.T) {
	g := NewWithT(t)

	workload := &Workload{
		Client: &fakeClient{
			get: map[string]interface{}{
				"kube-system/etcd-test-1": etcdPod("etcd-test-1", withReadyOption),
				"kube-system/etcd-test-2": etcdPod("etcd-test-2", withReadyOption),
				"kube-system/etcd-test-3": etcdPod("etcd-test-3", withReadyOption),
				"kube-system/etcd-test-4": etcdPod("etcd-test-4"),
			},
			list: &corev1.NodeList{
				Items: []corev1.Node{
					nodeNamed("test-1", withProviderID("my-provider-id-1")),
					nodeNamed("test-2", withProviderID("my-provider-id-2")),
					nodeNamed("test-3", withProviderID("my-provider-id-3")),
					nodeNamed("test-4", withProviderID("my-provider-id-4")),
				},
			},
		},
		etcdClientGenerator: &fakeEtcdClientGenerator{
			client: &etcd.Client{
				EtcdClient: &fake2.FakeEtcdClient{
					EtcdEndpoints: []string{},
					MemberListResponse: &clientv3.MemberListResponse{
						Members: []*pb.Member{
							{Name: "test-1", ID: uint64(1)},
							{Name: "test-2", ID: uint64(2)},
							{Name: "test-3", ID: uint64(3)},
						},
					},
					AlarmResponse: &clientv3.AlarmResponse{
						Alarms: []*pb.AlarmMember{},
					},
				},
			},
		},
	}
	ctx := context.Background()
	health, err := workload.EtcdIsHealthy(ctx)
	g.Expect(err).NotTo(HaveOccurred())

	for _, err := range health {
		g.Expect(err).NotTo(HaveOccurred())
	}
}

type podOption func(*corev1.Pod)

func etcdPod(name string, options ...podOption) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceSystem,
		},
	}
	for _, opt := range options {
		opt(p)
	}
	return p
}
func withReadyOption(pod *corev1.Pod) {
	readyCondition := corev1.PodCondition{
		Type:   corev1.PodReady,
		Status: corev1.ConditionTrue,
	}
	pod.Status.Conditions = append(pod.Status.Conditions, readyCondition)
}

func withProviderID(pi string) func(corev1.Node) corev1.Node {
	return func(node corev1.Node) corev1.Node {
		node.Spec.ProviderID = pi
		return node
	}
}

type fakeEtcdClientGenerator struct {
	client *etcd.Client
}

func (c *fakeEtcdClientGenerator) forNode(_ context.Context, _ string) (*etcd.Client, error) {
	return c.client, nil
}
