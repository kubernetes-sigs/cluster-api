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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCheckStaticPodReadyCondition(t *testing.T) {
	table := []checkStaticPodReadyConditionTest{
		{
			name:       "pod is ready",
			conditions: []corev1.PodCondition{podReady(corev1.ConditionTrue)},
		},
	}
	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			pod := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec:   corev1.PodSpec{},
				Status: corev1.PodStatus{Conditions: test.conditions},
			}
			g.Expect(checkStaticPodReadyCondition(pod)).To(Succeed())
		})
	}
}

func TestCheckStaticPodNotReadyCondition(t *testing.T) {
	table := []checkStaticPodReadyConditionTest{
		{
			name: "no pod status",
		},
		{
			name:       "not ready pod status",
			conditions: []corev1.PodCondition{podReady(corev1.ConditionFalse)},
		},
	}
	for _, test := range table {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			pod := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Spec:   corev1.PodSpec{},
				Status: corev1.PodStatus{Conditions: test.conditions},
			}
			g.Expect(checkStaticPodReadyCondition(pod)).NotTo(Succeed())
		})
	}
}

func TestControlPlaneIsHealthy(t *testing.T) {
	g := NewWithT(t)

	readyStatus := corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			},
		},
	}
	workloadCluster := &Workload{
		Client: &fakeClient{
			list: nodeListForTestControlPlaneIsHealthy(),
			get: map[string]interface{}{
				"kube-system/kube-apiserver-first-control-plane":           &corev1.Pod{Status: readyStatus},
				"kube-system/kube-apiserver-second-control-plane":          &corev1.Pod{Status: readyStatus},
				"kube-system/kube-apiserver-third-control-plane":           &corev1.Pod{Status: readyStatus},
				"kube-system/kube-controller-manager-first-control-plane":  &corev1.Pod{Status: readyStatus},
				"kube-system/kube-controller-manager-second-control-plane": &corev1.Pod{Status: readyStatus},
				"kube-system/kube-controller-manager-third-control-plane":  &corev1.Pod{Status: readyStatus},
			},
		},
	}

	health, err := workloadCluster.ControlPlaneIsHealthy(context.Background())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(health).NotTo(HaveLen(0))
	g.Expect(health).To(HaveLen(len(nodeListForTestControlPlaneIsHealthy().Items)))
}

func TestGetMachinesForCluster(t *testing.T) {
	g := NewWithT(t)

	m := Management{Client: &fakeClient{
		list: machineListForTestGetMachinesForCluster(),
	}}
	clusterKey := client.ObjectKey{
		Namespace: "my-namespace",
		Name:      "my-cluster",
	}
	machines, err := m.GetMachinesForCluster(context.Background(), clusterKey)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines).To(HaveLen(3))

	// Test the ControlPlaneMachines works
	machines, err = m.GetMachinesForCluster(context.Background(), clusterKey, machinefilters.ControlPlaneMachines("my-cluster"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines).To(HaveLen(1))

	// Test that the filters use AND logic instead of OR logic
	nameFilter := func(cluster *clusterv1.Machine) bool {
		return cluster.Name == "first-machine"
	}
	machines, err = m.GetMachinesForCluster(context.Background(), clusterKey, machinefilters.ControlPlaneMachines("my-cluster"), nameFilter)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(machines).To(HaveLen(1))
}

func TestGetWorkloadCluster(t *testing.T) {
	g := NewWithT(t)

	ns, err := testEnv.CreateNamespace(ctx, "workload-cluster2")
	g.Expect(err).ToNot(HaveOccurred())
	defer func() {
		g.Expect(testEnv.Cleanup(ctx, ns)).To(Succeed())
	}()

	// Create an etcd secret with valid certs
	key, err := certs.NewPrivateKey()
	g.Expect(err).ToNot(HaveOccurred())
	cert, err := getTestCACert(key)
	g.Expect(err).ToNot(HaveOccurred())
	etcdSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster-etcd",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			secret.TLSCrtDataName: certs.EncodeCertPEM(cert),
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(key),
		},
	}
	emptyCrtEtcdSecret := etcdSecret.DeepCopy()
	delete(emptyCrtEtcdSecret.Data, secret.TLSCrtDataName)
	emptyKeyEtcdSecret := etcdSecret.DeepCopy()
	delete(emptyKeyEtcdSecret.Data, secret.TLSKeyDataName)
	badCrtEtcdSecret := etcdSecret.DeepCopy()
	badCrtEtcdSecret.Data[secret.TLSCrtDataName] = []byte("bad cert")

	// Create kubeconfig secret
	// Store the envtest config as the contents of the kubeconfig secret.
	// This way we are using the envtest environment as both the
	// management and the workload cluster.
	testEnvKubeconfig := kubeconfig.FromEnvTestConfig(testEnv.GetConfig(), &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: ns.Name,
		},
	})
	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster-kubeconfig",
			Namespace: ns.Name,
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: testEnvKubeconfig,
		},
	}
	clusterKey := client.ObjectKey{
		Name:      "my-cluster",
		Namespace: ns.Name,
	}

	tests := []struct {
		name       string
		clusterKey client.ObjectKey
		objs       []runtime.Object
		expectErr  bool
	}{
		{
			name:       "returns a workload cluster",
			clusterKey: clusterKey,
			objs:       []runtime.Object{etcdSecret.DeepCopy(), kubeconfigSecret.DeepCopy()},
			expectErr:  false,
		},
		{
			name:       "returns error if cannot get rest.Config from kubeconfigSecret",
			clusterKey: clusterKey,
			objs:       []runtime.Object{etcdSecret.DeepCopy()},
			expectErr:  true,
		},
		{
			name:       "returns error if unable to find the etcd secret",
			clusterKey: clusterKey,
			objs:       []runtime.Object{kubeconfigSecret.DeepCopy()},
			expectErr:  true,
		},
		{
			name:       "returns error if unable to find the certificate in the etcd secret",
			clusterKey: clusterKey,
			objs:       []runtime.Object{emptyCrtEtcdSecret.DeepCopy(), kubeconfigSecret.DeepCopy()},
			expectErr:  true,
		},
		{
			name:       "returns error if unable to find the key in the etcd secret",
			clusterKey: clusterKey,
			objs:       []runtime.Object{emptyKeyEtcdSecret.DeepCopy(), kubeconfigSecret.DeepCopy()},
			expectErr:  true,
		},
		{
			name:       "returns error if unable to generate client cert",
			clusterKey: clusterKey,
			objs:       []runtime.Object{badCrtEtcdSecret.DeepCopy(), kubeconfigSecret.DeepCopy()},
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			for _, o := range tt.objs {
				g.Expect(testEnv.CreateObj(ctx, o)).To(Succeed())
				defer func(do runtime.Object) {
					g.Expect(testEnv.Cleanup(ctx, do)).To(Succeed())
				}(o)
			}

			m := Management{
				Client: testEnv,
			}

			workloadCluster, err := m.GetWorkloadCluster(context.Background(), tt.clusterKey)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(workloadCluster).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(workloadCluster).ToNot(BeNil())
		})
	}

}

func getTestCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "kubernetes",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now.Add(time.Minute * -5),
		NotAfter:              now.Add(time.Hour * 24), // 1 day
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}

	c, err := x509.ParseCertificate(b)
	return c, err
}

func podReady(isReady corev1.ConditionStatus) corev1.PodCondition {
	return corev1.PodCondition{
		Type:   corev1.PodReady,
		Status: isReady,
	}
}

type checkStaticPodReadyConditionTest struct {
	name       string
	conditions []corev1.PodCondition
}

func nodeListForTestControlPlaneIsHealthy() *corev1.NodeList {
	return &corev1.NodeList{
		Items: []corev1.Node{
			nodeNamed("first-control-plane"),
			nodeNamed("second-control-plane"),
			nodeNamed("third-control-plane"),
		},
	}
}

func nodeNamed(name string, options ...func(n corev1.Node) corev1.Node) corev1.Node {
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range options {
		node = opt(node)
	}
	return node
}

func machineListForTestGetMachinesForCluster() *clusterv1.MachineList {
	owned := true
	ownedRef := []metav1.OwnerReference{
		{
			Kind:       "KubeadmControlPlane",
			Name:       "my-control-plane",
			Controller: &owned,
		},
	}
	machine := func(name string) clusterv1.Machine {
		return clusterv1.Machine{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "my-namespace",
				Labels: map[string]string{
					clusterv1.ClusterLabelName: "my-cluster",
				},
			},
		}
	}
	controlPlaneMachine := machine("first-machine")
	controlPlaneMachine.ObjectMeta.Labels[clusterv1.MachineControlPlaneLabelName] = ""
	controlPlaneMachine.OwnerReferences = ownedRef

	return &clusterv1.MachineList{
		Items: []clusterv1.Machine{
			controlPlaneMachine,
			machine("second-machine"),
			machine("third-machine"),
		},
	}
}

type fakeClient struct {
	client.Client
	list interface{}

	createErr    error
	get          map[string]interface{}
	getCalled    bool
	updateCalled bool
	getErr       error
	patchErr     error
	updateErr    error
	listErr      error
}

func (f *fakeClient) Get(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
	f.getCalled = true
	if f.getErr != nil {
		return f.getErr
	}
	item := f.get[key.String()]
	switch l := item.(type) {
	case *corev1.Pod:
		l.DeepCopyInto(obj.(*corev1.Pod))
	case *rbacv1.RoleBinding:
		l.DeepCopyInto(obj.(*rbacv1.RoleBinding))
	case *rbacv1.Role:
		l.DeepCopyInto(obj.(*rbacv1.Role))
	case *appsv1.DaemonSet:
		l.DeepCopyInto(obj.(*appsv1.DaemonSet))
	case *corev1.ConfigMap:
		l.DeepCopyInto(obj.(*corev1.ConfigMap))
	case *corev1.Secret:
		l.DeepCopyInto(obj.(*corev1.Secret))
	case nil:
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	default:
		return fmt.Errorf("unknown type: %s", l)
	}
	return nil
}

func (f *fakeClient) List(_ context.Context, list runtime.Object, _ ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	switch l := f.list.(type) {
	case *clusterv1.MachineList:
		l.DeepCopyInto(list.(*clusterv1.MachineList))
	case *corev1.NodeList:
		l.DeepCopyInto(list.(*corev1.NodeList))
	case *corev1.PodList:
		l.DeepCopyInto(list.(*corev1.PodList))
	default:
		return fmt.Errorf("unknown type: %s", l)
	}
	return nil
}

func (f *fakeClient) Create(_ context.Context, _ runtime.Object, _ ...client.CreateOption) error {
	if f.createErr != nil {
		return f.createErr
	}
	return nil
}

func (f *fakeClient) Patch(_ context.Context, _ runtime.Object, _ client.Patch, _ ...client.PatchOption) error {
	if f.patchErr != nil {
		return f.patchErr
	}
	return nil
}

func (f *fakeClient) Update(_ context.Context, _ runtime.Object, _ ...client.UpdateOption) error {
	f.updateCalled = true
	if f.updateErr != nil {
		return f.updateErr
	}
	return nil
}
