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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
		{
			name:        "does not update image repository when no kube-proxy update is requested",
			ds:          newKubeProxyDSWithImage(""), // Using the same image name that would otherwise lead to an error
			expectErr:   false,
			expectImage: "",
			KCP: &v1alpha3.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1alpha3.SkipKubeProxyAnnotation: "",
					},
				},
				Spec: v1alpha3.KubeadmControlPlaneSpec{
					Version: "v1.16.3",
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

func TestRemoveMachineFromKubeadmConfigMap(t *testing.T) {
	machine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "ip-10-0-0-1.ec2.internal",
			},
		},
	}
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterStatusKey: `apiEndpoints:
  ip-10-0-0-1.ec2.internal:
    advertiseAddress: 10.0.0.1
    bindPort: 6443
  ip-10-0-0-2.ec2.internal:
    advertiseAddress: 10.0.0.2
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: bar
apiVersion: kubeadm.k8s.io/vNbetaM
kind: ClusterStatus`,
		},
		BinaryData: map[string][]byte{
			"": nil,
		},
	}
	kconfWithoutKey := kubeadmConfig.DeepCopy()
	delete(kconfWithoutKey.Data, clusterStatusKey)

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name              string
		machine           *clusterv1.Machine
		objs              []runtime.Object
		expectErr         bool
		expectedEndpoints string
	}{
		{
			name:      "does not panic if machine is nil",
			expectErr: false,
		},
		{
			name: "does not panic if machine noderef is nil",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: nil,
				},
			},
			expectErr: false,
		},
		{
			name:      "returns error if unable to find kubeadm-config",
			machine:   machine,
			expectErr: true,
		},
		{
			name:      "returns error if unable to remove api endpoint",
			machine:   machine,
			objs:      []runtime.Object{kconfWithoutKey},
			expectErr: true,
		},
		{
			name:      "removes the machine node ref from kubeadm config",
			machine:   machine,
			objs:      []runtime.Object{kubeadmConfig},
			expectErr: false,
			expectedEndpoints: `apiEndpoints:
  ip-10-0-0-2.ec2.internal:
    advertiseAddress: 10.0.0.2
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: bar
apiVersion: kubeadm.k8s.io/vNbetaM
kind: ClusterStatus
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.RemoveMachineFromKubeadmConfigMap(ctx, tt.machine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			if tt.expectedEndpoints != "" {
				var actualConfig corev1.ConfigMap
				g.Expect(w.Client.Get(
					ctx,
					ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
					&actualConfig,
				)).To(Succeed())
				g.Expect(actualConfig.Data[clusterStatusKey]).To(Equal(tt.expectedEndpoints))
			}
		})
	}
}

func TestUpdateKubeletConfigMap(t *testing.T) {
	kubeletConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "kubelet-config-1.1",
			Namespace:       metav1.NamespaceSystem,
			ResourceVersion: "some-resource-version",
		},
	}

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name      string
		version   semver.Version
		objs      []runtime.Object
		expectErr bool
	}{
		{
			name:      "create new config map",
			version:   semver.Version{Major: 1, Minor: 2},
			objs:      []runtime.Object{kubeletConfig},
			expectErr: false,
		},
		{
			name:      "returns error if cannot find previous config map",
			version:   semver.Version{Major: 1, Minor: 2},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.UpdateKubeletConfigMap(ctx, tt.version)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: "kubelet-config-1.2", Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.ResourceVersion).ToNot(Equal(kubeletConfig.ResourceVersion))
		})
	}
}

func TestUpdateKubernetesVersionInKubeadmConfigMap(t *testing.T) {
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: `
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
kubernetesVersion: v1.16.1
`,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `foobar`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name      string
		version   semver.Version
		objs      []runtime.Object
		expectErr bool
	}{
		{
			name:      "updates the config map",
			version:   semver.Version{Major: 1, Minor: 17, Patch: 2},
			objs:      []runtime.Object{kubeadmConfig},
			expectErr: false,
		},
		{
			name:      "returns error if cannot find config map",
			version:   semver.Version{Major: 1, Minor: 2},
			expectErr: true,
		},
		{
			name:      "returns error if config has bad data",
			version:   semver.Version{Major: 1, Minor: 2},
			objs:      []runtime.Object{kubeadmConfigBadData},
			expectErr: true,
		},
		{
			name:      "returns error if config doesn't have cluster config key",
			version:   semver.Version{Major: 1, Minor: 2},
			objs:      []runtime.Object{kubeadmConfigNoKey},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.UpdateKubernetesVersionInKubeadmConfigMap(ctx, tt.version)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(ContainSubstring("kubernetesVersion: v1.17.2"))
		})
	}
}

func TestUpdateImageRepositoryInKubeadmConfigMap(t *testing.T) {
	kubeadmConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			clusterConfigurationKey: `
apiVersion: kubeadm.k8s.io/v1beta2
kind: ClusterConfiguration
imageRepository: k8s.gcr.io
`,
		},
	}

	kubeadmConfigNoKey := kubeadmConfig.DeepCopy()
	delete(kubeadmConfigNoKey.Data, clusterConfigurationKey)

	kubeadmConfigBadData := kubeadmConfig.DeepCopy()
	kubeadmConfigBadData.Data[clusterConfigurationKey] = `foobar`

	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	tests := []struct {
		name            string
		imageRepository string
		objs            []runtime.Object
		expectErr       bool
	}{
		{
			name:            "updates the config map",
			imageRepository: "myspecialrepo.io",
			objs:            []runtime.Object{kubeadmConfig},
			expectErr:       false,
		},
		{
			name:      "returns error if cannot find config map",
			expectErr: true,
		},
		{
			name:            "returns error if config has bad data",
			objs:            []runtime.Object{kubeadmConfigBadData},
			imageRepository: "myspecialrepo.io",
			expectErr:       true,
		},
		{
			name:            "returns error if config doesn't have cluster config key",
			objs:            []runtime.Object{kubeadmConfigNoKey},
			imageRepository: "myspecialrepo.io",
			expectErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			err := w.UpdateImageRepositoryInKubeadmConfigMap(ctx, tt.imageRepository)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			var actualConfig corev1.ConfigMap
			g.Expect(w.Client.Get(
				ctx,
				ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem},
				&actualConfig,
			)).To(Succeed())
			g.Expect(actualConfig.Data[clusterConfigurationKey]).To(ContainSubstring(tt.imageRepository))
		})
	}
}

func TestClusterStatus(t *testing.T) {
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node1",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node2",
			Labels: map[string]string{
				labelNodeRoleControlPlane: "",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	kconf := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeadmConfigKey,
			Namespace: metav1.NamespaceSystem,
		},
	}
	tests := []struct {
		name          string
		objs          []runtime.Object
		expectErr     bool
		expectHasConf bool
	}{
		{
			name:          "returns cluster status",
			objs:          []runtime.Object{node1, node2},
			expectErr:     false,
			expectHasConf: false,
		},
		{
			name:          "returns cluster status with kubeadm config",
			objs:          []runtime.Object{node1, node2, kconf},
			expectErr:     false,
			expectHasConf: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme := runtime.NewScheme()
			g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
			fakeClient := fake.NewFakeClientWithScheme(scheme, tt.objs...)
			w := &Workload{
				Client: fakeClient,
			}
			ctx := context.TODO()
			status, err := w.ClusterStatus(ctx)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(status.Nodes).To(BeEquivalentTo(2))
			g.Expect(status.ReadyNodes).To(BeEquivalentTo(1))
			if tt.expectHasConf {
				g.Expect(status.HasKubeadmConfig).To(BeTrue())
				return
			}
			g.Expect(status.HasKubeadmConfig).To(BeFalse())
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

func TestHealthCheck_NoError(t *testing.T) {
	threeMachines := []*clusterv1.Machine{
		controlPlaneMachine("one"),
		controlPlaneMachine("two"),
		controlPlaneMachine("three"),
	}
	controlPlane := createControlPlane(threeMachines)
	tests := []struct {
		name             string
		checkResult      HealthCheckResult
		controlPlaneName string
		controlPlane     *ControlPlane
	}{
		{
			name: "simple",
			checkResult: HealthCheckResult{
				"one":   nil,
				"two":   nil,
				"three": nil,
			},
			controlPlane: controlPlane,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(tt.checkResult.Aggregate(controlPlane)).To(Succeed())
		})
	}
}

func TestManagementCluster_healthCheck_Errors(t *testing.T) {
	tests := []struct {
		name             string
		checkResult      HealthCheckResult
		clusterKey       ctrlclient.ObjectKey
		controlPlaneName string
		controlPlane     *ControlPlane
		// expected errors will ensure the error contains this list of strings.
		// If not supplied, no check on the error's value will occur.
		expectedErrors []string
	}{
		{
			name: "machine's node was not checked for health",
			controlPlane: createControlPlane([]*clusterv1.Machine{
				controlPlaneMachine("one"),
				controlPlaneMachine("two"),
				controlPlaneMachine("three"),
			}),
			checkResult: HealthCheckResult{
				"one": nil,
			},
		},
		{
			name: "two nodes error on the check but no overall error occurred",
			controlPlane: createControlPlane([]*clusterv1.Machine{
				controlPlaneMachine("one"),
				controlPlaneMachine("two"),
				controlPlaneMachine("three")}),
			checkResult: HealthCheckResult{
				"one":   nil,
				"two":   errors.New("two"),
				"three": errors.New("three"),
			},
			expectedErrors: []string{"two", "three"},
		},
		{
			name: "more nodes than machines were checked (out of band control plane nodes)",
			controlPlane: createControlPlane([]*clusterv1.Machine{
				controlPlaneMachine("one")}),
			checkResult: HealthCheckResult{
				"one":   nil,
				"two":   nil,
				"three": nil,
			},
		},
		{
			name: "a machine that has a nil node reference",
			controlPlane: createControlPlane([]*clusterv1.Machine{
				controlPlaneMachine("one"),
				controlPlaneMachine("two"),
				nilNodeRef(controlPlaneMachine("three"))}),
			checkResult: HealthCheckResult{
				"one":   nil,
				"two":   nil,
				"three": nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.checkResult.Aggregate(tt.controlPlane)
			g.Expect(err).To(HaveOccurred())

			for _, expectedError := range tt.expectedErrors {
				g.Expect(err).To(MatchError(ContainSubstring(expectedError)))
			}
		})
	}
}
func createControlPlane(machines []*clusterv1.Machine) *ControlPlane {
	defaultInfra := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "InfrastructureMachine",
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1alpha3",
			"metadata": map[string]interface{}{
				"name":      "infra-config1",
				"namespace": "default",
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}

	fakeClient := fake.NewFakeClientWithScheme(runtime.NewScheme(), defaultInfra.DeepCopy())

	controlPlane, _ := NewControlPlane(ctx, fakeClient, &clusterv1.Cluster{}, &v1alpha3.KubeadmControlPlane{}, NewFilterableMachineCollection(machines...))
	return controlPlane
}

func controlPlaneMachine(name string) *clusterv1.Machine {
	t := true
	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
		Name:       "infra",
		Namespace:  "default",
	}

	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
			Labels:    ControlPlaneLabelsForCluster("cluster-name"),
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "KubeadmControlPlane",
					Name:       "control-plane-name",
					Controller: &t,
				},
			},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: *infraRef.DeepCopy(),
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: name,
			},
		},
	}
}

func nilNodeRef(machine *clusterv1.Machine) *clusterv1.Machine {
	machine.Status.NodeRef = nil
	return machine
}
