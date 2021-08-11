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

package cluster

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	admissionregistration "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var certManagerDeploymentYaml = []byte("apiVersion: apps/v1\n" +
	"kind: Deployment\n" +
	"metadata:\n" +
	"  name: cert-manager\n" +
	"spec:\n" +
	"  template:\n" +
	"    spec:\n" +
	"      containers:\n" +
	"      - name: manager\n" +
	"        image: quay.io/jetstack/cert-manager:v1.1.0\n")

var certManagerNamespaceYaml = []byte("apiVersion: v1\n" +
	"kind: Namespace\n" +
	"metadata:\n" +
	"  name: cert-manager\n")

func Test_getManifestObjs(t *testing.T) {
	g := NewWithT(t)

	defaultConfigClient, err := config.New("", config.InjectReader(test.NewFakeReader().WithImageMeta(config.CertManagerImageComponent, "bar-repository.io", "")))
	g.Expect(err).NotTo(HaveOccurred())

	type fields struct {
		configClient config.Client
		repository   repository.Repository
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "successfully gets the cert-manager components",
			fields: fields{
				configClient: defaultConfigClient,
				repository: repository.NewMemoryRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion(config.CertManagerDefaultVersion).
					WithFile(config.CertManagerDefaultVersion, "components.yaml", utilyaml.JoinYaml(certManagerNamespaceYaml, certManagerDeploymentYaml)),
			},
			wantErr: false,
		},
		{
			name: "fails if the file does not exists",
			fields: fields{
				configClient: defaultConfigClient,
				repository: repository.NewMemoryRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v1.0.0"),
			},
			wantErr: true,
		},
		{
			name: "fails if the file does not exists for the desired version",
			fields: fields{
				configClient: defaultConfigClient,
				repository: repository.NewMemoryRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion("v99.0.0").
					WithFile("v99.0.0", "components.yaml", utilyaml.JoinYaml(certManagerNamespaceYaml, certManagerDeploymentYaml)),
			},
			wantErr: true,
		},
		{
			name: "successfully gets the cert-manager components for a custom release",
			fields: fields{
				configClient: func() config.Client {
					configClient, err := config.New("", config.InjectReader(test.NewFakeReader().WithImageMeta(config.CertManagerImageComponent, "bar-repository.io", "").WithCertManager("", "v1.0.0", "")))
					g.Expect(err).ToNot(HaveOccurred())
					return configClient
				}(),
				repository: repository.NewMemoryRepository().
					WithPaths("root", "components.yaml").
					WithDefaultVersion(config.CertManagerDefaultVersion).
					WithFile(config.CertManagerDefaultVersion, "components.yaml", utilyaml.JoinYaml(certManagerNamespaceYaml, certManagerDeploymentYaml)),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cm := &certManagerClient{
				configClient: defaultConfigClient,
				repositoryClientFactory: func(provider config.Provider, configClient config.Client, options ...repository.Option) (repository.Client, error) {
					return repository.New(provider, configClient, repository.InjectRepository(tt.fields.repository))
				},
			}

			certManagerConfig, err := cm.configClient.CertManager().Get()
			g.Expect(err).ToNot(HaveOccurred())

			got, err := cm.getManifestObjs(certManagerConfig)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			for i := range got {
				o := &got[i]
				// Assert Get adds clusterctl labels.
				g.Expect(o.GetLabels()).To(HaveKey(clusterctlv1.ClusterctlLabelName))
				g.Expect(o.GetLabels()).To(HaveKey(clusterctlv1.ClusterctlCoreLabelName))
				g.Expect(o.GetLabels()[clusterctlv1.ClusterctlCoreLabelName]).To(Equal(clusterctlv1.ClusterctlCoreLabelCertManagerValue))

				// Assert Get adds clusterctl annotations.
				g.Expect(o.GetAnnotations()).To(HaveKey(clusterctlv1.CertManagerVersionAnnotation))
				g.Expect(o.GetAnnotations()[clusterctlv1.CertManagerVersionAnnotation]).To(Equal(certManagerConfig.Version()))

				// Assert Get fixes images.
				if o.GetKind() == "Deployment" {
					// Convert Unstructured into a typed object
					d := &appsv1.Deployment{}
					g.Expect(scheme.Scheme.Convert(o, d, nil)).To(Succeed())
					g.Expect(d.Spec.Template.Spec.Containers[0].Image).To(Equal("bar-repository.io/cert-manager:v1.1.0"))
				}
			}
		})
	}
}

func Test_GetTimeout(t *testing.T) {
	pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
		return nil
	}

	tests := []struct {
		name   string
		config *fakeConfigClient
		want   time.Duration
	}{
		{
			name:   "no custom value set for timeout",
			config: newFakeConfig(),
			want:   10 * time.Minute,
		},
		{
			name:   "a custom value of timeout is set",
			config: newFakeConfig().WithCertManager("", "", "5m"),
			want:   5 * time.Minute,
		},
		{
			name:   "invalid custom value of timeout is set",
			config: newFakeConfig().WithCertManager("", "", "foo"),
			want:   10 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			cm := newCertManagerClient(tt.config, nil, nil, pollImmediateWaiter)

			tm := cm.getWaitTimeout()

			g.Expect(tm).To(Equal(tt.want))
		})
	}
}

func Test_shouldUpgrade(t *testing.T) {
	type args struct {
		objs []unstructured.Unstructured
	}
	tests := []struct {
		name            string
		args            args
		wantFromVersion string
		wantToVersion   string
		want            bool
		wantErr         bool
	}{
		{
			name: "Version is not defined (e.g. cluster created with clusterctl < v0.3.9), should upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{},
					},
				},
			},
			wantFromVersion: "v0.11.0",
			wantToVersion:   config.CertManagerDefaultVersion,
			want:            true,
			wantErr:         false,
		},
		{
			name: "Version is equal, should not upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									clusterctlv1.CertManagerVersionAnnotation: config.CertManagerDefaultVersion,
								},
							},
						},
					},
				},
			},
			wantFromVersion: config.CertManagerDefaultVersion,
			wantToVersion:   config.CertManagerDefaultVersion,
			want:            false,
			wantErr:         false,
		},
		{
			name: "Version is older, should upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									clusterctlv1.CertManagerVersionAnnotation: "v0.11.0",
								},
							},
						},
					},
				},
			},
			wantFromVersion: "v0.11.0",
			wantToVersion:   config.CertManagerDefaultVersion,
			want:            true,
			wantErr:         false,
		},
		{
			name: "Version is newer, should not upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									clusterctlv1.CertManagerVersionAnnotation: "v100.0.0",
								},
							},
						},
					},
				},
			},
			wantFromVersion: "v100.0.0",
			wantToVersion:   config.CertManagerDefaultVersion,
			want:            false,
			wantErr:         false,
		},
		{
			name: "Endpoint are ignored",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"kind": "Endpoints",
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									clusterctlv1.CertManagerVersionAnnotation: config.CertManagerDefaultVersion,
								},
							},
						},
					},
				},
			},
			wantFromVersion: "",
			wantToVersion:   config.CertManagerDefaultVersion,
			want:            false,
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			proxy := test.NewFakeProxy()
			fakeConfigClient := newFakeConfig()
			pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
				return nil
			}
			cm := newCertManagerClient(fakeConfigClient, nil, proxy, pollImmediateWaiter)

			fromVersion, toVersion, got, err := cm.shouldUpgrade(tt.args.objs)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
			g.Expect(fromVersion).To(Equal(tt.wantFromVersion))
			g.Expect(toVersion).To(Equal(tt.wantToVersion))
		})
	}
}

func Test_certManagerClient_deleteObjs(t *testing.T) {
	type fields struct {
		objs []client.Object
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string // Define the list of "Kind, Namespace/Name" that should still exist after delete
		wantErr bool
	}{
		{
			name: "CRD should not be deleted",
			fields: fields{
				objs: []client.Object{
					&apiextensionsv1.CustomResourceDefinition{
						TypeMeta: metav1.TypeMeta{
							Kind:       "CustomResourceDefinition",
							APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "foo",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						},
					},
				},
			},
			want:    []string{"CustomResourceDefinition, /foo"},
			wantErr: false,
		},
		{
			name: "Namespace should not be deleted",
			fields: fields{
				objs: []client.Object{
					&corev1.Namespace{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Namespace",
							APIVersion: corev1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "foo",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						},
					},
				},
			},
			want:    []string{"Namespace, /foo"},
			wantErr: false,
		},
		{
			name: "MutatingWebhookConfiguration should not be deleted",
			fields: fields{
				objs: []client.Object{
					&admissionregistration.MutatingWebhookConfiguration{
						TypeMeta: metav1.TypeMeta{
							Kind:       "MutatingWebhookConfiguration",
							APIVersion: admissionregistration.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "foo",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						},
					},
				},
			},
			want:    []string{"MutatingWebhookConfiguration, /foo"},
			wantErr: false,
		},
		{
			name: "ValidatingWebhookConfiguration should not be deleted",
			fields: fields{
				objs: []client.Object{
					&admissionregistration.ValidatingWebhookConfiguration{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ValidatingWebhookConfiguration",
							APIVersion: admissionregistration.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "foo",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						},
					},
				},
			},
			want:    []string{"ValidatingWebhookConfiguration, /foo"},
			wantErr: false,
		},
		{
			name: "Other resources should be deleted",
			fields: fields{
				objs: []client.Object{
					&corev1.ServiceAccount{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ServiceAccount",
							APIVersion: corev1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "foo",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						},
					},
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Deployment",
							APIVersion: appsv1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "bar",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			proxy := test.NewFakeProxy().WithObjs(tt.fields.objs...)
			cm := &certManagerClient{
				pollImmediateWaiter: fakePollImmediateWaiter,
				proxy:               proxy,
			}

			objBefore, err := proxy.ListResources(map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue})
			g.Expect(err).ToNot(HaveOccurred())

			err = cm.deleteObjs(objBefore)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			for _, obj := range tt.fields.objs {
				accessor, err := meta.Accessor(obj)
				g.Expect(err).ToNot(HaveOccurred())

				objShouldStillExist := false
				for _, want := range tt.want {
					if fmt.Sprintf("%s, %s/%s", obj.GetObjectKind().GroupVersionKind().Kind, accessor.GetNamespace(), accessor.GetName()) == want {
						objShouldStillExist = true
					}
				}

				cl, err := proxy.NewClient()
				g.Expect(err).ToNot(HaveOccurred())

				err = cl.Get(ctx, client.ObjectKeyFromObject(obj), obj)
				switch objShouldStillExist {
				case true:
					g.Expect(err).ToNot(HaveOccurred())
				case false:
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
				}
			}
		})
	}
}

func Test_certManagerClient_PlanUpgrade(t *testing.T) {
	tests := []struct {
		name         string
		objs         []client.Object
		expectErr    bool
		expectedPlan CertManagerUpgradePlan
	}{
		{
			name: "returns the upgrade plan for cert-manager if v0.11.0 is installed",
			// Cert-manager deployment without annotation, this must be from
			// v0.11.0
			objs: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   "cert-manager",
						Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          "v0.11.0",
				To:            config.CertManagerDefaultVersion,
				ShouldUpgrade: true,
			},
		},
		{
			name: "returns the upgrade plan for cert-manager if an older version is installed",
			objs: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cert-manager",
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						Annotations: map[string]string{clusterctlv1.CertManagerVersionAnnotation: "v0.10.2"},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          "v0.10.2",
				To:            config.CertManagerDefaultVersion,
				ShouldUpgrade: true,
			},
		},
		{
			name: "returns plan if shouldn't upgrade",
			objs: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cert-manager",
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						Annotations: map[string]string{clusterctlv1.CertManagerVersionAnnotation: config.CertManagerDefaultVersion},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          config.CertManagerDefaultVersion,
				To:            config.CertManagerDefaultVersion,
				ShouldUpgrade: false,
			},
		},
		{
			name: "returns empty plan and error if cannot parse semver",
			objs: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cert-manager",
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue},
						Annotations: map[string]string{clusterctlv1.CertManagerVersionAnnotation: "bad-sem-ver"},
					},
				},
			},
			expectErr: true,
			expectedPlan: CertManagerUpgradePlan{
				From:          "",
				To:            "",
				ShouldUpgrade: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			proxy := test.NewFakeProxy().WithObjs(tt.objs...)
			fakeConfigClient := newFakeConfig()
			pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
				return nil
			}
			cm := newCertManagerClient(fakeConfigClient, nil, proxy, pollImmediateWaiter)

			actualPlan, err := cm.PlanUpgrade()
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(actualPlan).To(Equal(CertManagerUpgradePlan{}))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(actualPlan).To(Equal(tt.expectedPlan))
		})
	}
}

func Test_certManagerClient_EnsureLatestVersion(t *testing.T) {
	type fields struct {
		proxy Proxy
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				proxy: test.NewFakeProxy().WithObjs(
					&corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{},
						},
					},
				),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cm := &certManagerClient{
				proxy: tt.fields.proxy,
			}

			err := cm.EnsureLatestVersion()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func newFakeConfig() *fakeConfigClient {
	fakeReader := test.NewFakeReader()

	client, _ := config.New("fake-config", config.InjectReader(fakeReader))
	return &fakeConfigClient{
		fakeReader:     fakeReader,
		internalclient: client,
	}
}

type fakeConfigClient struct {
	fakeReader     *test.FakeReader
	internalclient config.Client
}

var _ config.Client = &fakeConfigClient{}

func (f fakeConfigClient) CertManager() config.CertManagerClient {
	return f.internalclient.CertManager()
}

func (f fakeConfigClient) Providers() config.ProvidersClient {
	return f.internalclient.Providers()
}

func (f fakeConfigClient) Variables() config.VariablesClient {
	return f.internalclient.Variables()
}

func (f fakeConfigClient) ImageMeta() config.ImageMetaClient {
	return f.internalclient.ImageMeta()
}

func (f *fakeConfigClient) WithVar(key, value string) *fakeConfigClient {
	f.fakeReader.WithVar(key, value)
	return f
}

func (f *fakeConfigClient) WithProvider(provider config.Provider) *fakeConfigClient {
	f.fakeReader.WithProvider(provider.Name(), provider.Type(), provider.URL())
	return f
}

func (f *fakeConfigClient) WithCertManager(url, version, timeout string) *fakeConfigClient {
	f.fakeReader.WithCertManager(url, version, timeout)
	return f
}
