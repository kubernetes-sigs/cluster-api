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
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
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
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Those values are dummy for test only
	expectedHash    = "dummy-hash"
	expectedVersion = "v0.11.2"
)

func Test_VersionMarkerUpToDate(t *testing.T) {
	pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
		return nil
	}
	fakeConfigClient := newFakeConfig("")
	cm, err := newCertManagerClient(fakeConfigClient, nil, pollImmediateWaiter)

	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cm.embeddedCertManagerManifestVersion).ToNot(BeEmpty())
	g.Expect(cm.embeddedCertManagerManifestHash).ToNot(BeEmpty())
}

func Test_certManagerClient_getManifestObjects(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
		assert    func(*testing.T, []unstructured.Unstructured)
	}{
		{
			name:      "it should not contain the cert-manager-leaderelection ClusterRoleBinding",
			expectErr: false,
			assert: func(t *testing.T, objs []unstructured.Unstructured) {
				for _, o := range objs {
					if o.GetKind() == "ClusterRoleBinding" && o.GetName() == "cert-manager-leaderelection" {
						t.Error("should not find cert-manager-leaderelection ClusterRoleBinding")
					}
				}
			},
		},
		{
			name:      "the MutatingWebhookConfiguration should have sideEffects set to None ",
			expectErr: false,
			assert: func(t *testing.T, objs []unstructured.Unstructured) {
				found := false
				for i := range objs {
					o := objs[i]
					gvk := o.GroupVersionKind()
					if gvk.Kind == "MutatingWebhookConfiguration" && o.GetName() == "cert-manager-webhook" {
						switch gvk.Version {
						case "v1beta1":
							w := &admissionregistrationv1beta1.MutatingWebhookConfiguration{}
							err := scheme.Scheme.Convert(&o, w, nil)
							if err != nil {
								t.Errorf("did not expect err, got %s", err)
							}
							if len(w.Webhooks) != 1 {
								t.Error("expected 1 webhook to be configured")
							}
							wh := w.Webhooks[0]
							if wh.SideEffects != nil && *wh.SideEffects == admissionregistrationv1beta1.SideEffectClassNone {
								found = true
							}
						case "v1":
							w := &admissionregistration.MutatingWebhookConfiguration{}
							err := scheme.Scheme.Convert(&o, w, nil)
							if err != nil {
								t.Errorf("did not expect err, got %s", err)
							}
							if len(w.Webhooks) != 1 {
								t.Error("expected 1 webhook to be configured")
							}
							wh := w.Webhooks[0]
							if wh.SideEffects != nil && *wh.SideEffects == admissionregistration.SideEffectClassNone {
								found = true
							}
						}
					}
				}
				if !found {
					t.Error("Expected to find cert-manager-webhook MutatingWebhookConfiguration/v1beta1 with sideEffects=None")
				}
			},
		},
		{
			name:      "the ValidatingWebhookConfiguration should have sideEffects set to None ",
			expectErr: false,
			assert: func(t *testing.T, objs []unstructured.Unstructured) {
				found := false
				for i := range objs {
					o := objs[i]
					gvk := o.GroupVersionKind()
					if gvk.Kind == "ValidatingWebhookConfiguration" && o.GetName() == "cert-manager-webhook" {
						switch gvk.Version {
						case "v1beta1":
							w := &admissionregistrationv1beta1.ValidatingWebhookConfiguration{}
							err := scheme.Scheme.Convert(&o, w, nil)
							if err != nil {
								t.Errorf("did not expect err, got %s", err)
							}
							if len(w.Webhooks) != 1 {
								t.Error("expected 1 webhook to be configured")
							}
							wh := w.Webhooks[0]
							if wh.SideEffects != nil && *wh.SideEffects == admissionregistrationv1beta1.SideEffectClassNone {
								found = true
							}
						case "v1":
							w := &admissionregistration.ValidatingWebhookConfiguration{}
							err := scheme.Scheme.Convert(&o, w, nil)
							if err != nil {
								t.Errorf("did not expect err, got %s", err)
							}
							if len(w.Webhooks) != 1 {
								t.Error("expected 1 webhook to be configured")
							}
							wh := w.Webhooks[0]
							if wh.SideEffects != nil && *wh.SideEffects == admissionregistration.SideEffectClassNone {
								found = true
							}
						}

					}
				}
				if !found {
					t.Error("Expected to find cert-manager-webhook ValidatingWebhookConfiguration/v1beta1 with sideEffects=None")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
				return nil
			}
			fakeConfigClient := newFakeConfig("")

			cm, err := newCertManagerClient(fakeConfigClient, nil, pollImmediateWaiter)
			g.Expect(err).ToNot(HaveOccurred())

			objs, err := cm.getManifestObjs()

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			tt.assert(t, objs)
		})
	}

}

func Test_GetTimeout(t *testing.T) {
	pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
		return nil
	}

	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{
			name:    "no custom value set for timeout",
			timeout: "",
			want:    10 * time.Minute,
		},
		{
			name:    "a custom value of timeout is set",
			timeout: "5m",
			want:    5 * time.Minute,
		},
		{
			name:    "invalid custom value of timeout is set",
			timeout: "5",
			want:    10 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeConfigClient := newFakeConfig(tt.timeout)

			cm, err := newCertManagerClient(fakeConfigClient, nil, pollImmediateWaiter)
			g.Expect(err).ToNot(HaveOccurred())

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
		name        string
		args        args
		wantVersion string
		want        bool
		wantErr     bool
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
			wantVersion: "v0.11.0",
			want:        true,
			wantErr:     false,
		},
		{
			name: "Version & hash are equal, should not upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									certmanagerVersionAnnotation: expectedVersion,
									certmanagerHashAnnotation:    expectedHash,
								},
							},
						},
					},
				},
			},
			wantVersion: expectedVersion,
			want:        false,
			wantErr:     false,
		},
		{
			name: "Version is equal, hash is different, should upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									certmanagerVersionAnnotation: expectedVersion,
									certmanagerHashAnnotation:    "foo",
								},
							},
						},
					},
				},
			},
			wantVersion: fmt.Sprintf("%s (%s)", expectedVersion, "foo"),
			want:        true,
			wantErr:     false,
		},
		{
			name: "Version is older, should upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									certmanagerVersionAnnotation: "v0.11.0",
								},
							},
						},
					},
				},
			},
			wantVersion: "v0.11.0",
			want:        true,
			wantErr:     false,
		},
		{
			name: "Version is newer, should not upgrade",
			args: args{
				objs: []unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"metadata": map[string]interface{}{
								"annotations": map[string]interface{}{
									certmanagerVersionAnnotation: "v100.0.0",
								},
							},
						},
					},
				},
			},
			wantVersion: "v100.0.0",
			want:        false,
			wantErr:     false,
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
									certmanagerVersionAnnotation: "v0.11.0",
								},
							},
						},
					},
				},
			},
			wantVersion: "",
			want:        false,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			proxy := test.NewFakeProxy()
			fakeConfigClient := newFakeConfig("")
			pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
				return nil
			}
			cm, err := newCertManagerClient(fakeConfigClient, proxy, pollImmediateWaiter)
			// set dummy expected hash
			cm.embeddedCertManagerManifestHash = expectedHash
			cm.embeddedCertManagerManifestVersion = expectedVersion
			g.Expect(err).ToNot(HaveOccurred())

			gotVersion, got, err := cm.shouldUpgrade(tt.args.objs)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
			g.Expect(gotVersion).To(Equal(tt.wantVersion))
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
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
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
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
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
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
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
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
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
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
						},
					},
					&appsv1.Deployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Deployment",
							APIVersion: appsv1.SchemeGroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "bar",
							Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
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

			objBefore, err := proxy.ListResources(map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"})
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
						Labels: map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          "v0.11.0",
				To:            expectedVersion,
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
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
						Annotations: map[string]string{certmanagerVersionAnnotation: "v0.10.2", certmanagerHashAnnotation: "some-hash"},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          "v0.10.2",
				To:            expectedVersion,
				ShouldUpgrade: true,
			},
		},
		{
			name: "returns the upgrade plan for cert-manager if same version but different hash",
			objs: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: appsv1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:        "cert-manager",
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
						Annotations: map[string]string{certmanagerVersionAnnotation: expectedVersion, certmanagerHashAnnotation: "some-other-hash"},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          fmt.Sprintf("%s (some-other-hash)", expectedVersion),
				To:            expectedVersion,
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
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
						Annotations: map[string]string{certmanagerVersionAnnotation: expectedVersion, certmanagerHashAnnotation: expectedHash},
					},
				},
			},
			expectErr: false,
			expectedPlan: CertManagerUpgradePlan{
				From:          expectedVersion,
				To:            expectedVersion,
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
						Labels:      map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"},
						Annotations: map[string]string{certmanagerVersionAnnotation: "bad-sem-ver"},
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
			fakeConfigClient := newFakeConfig("")
			pollImmediateWaiter := func(interval, timeout time.Duration, condition wait.ConditionFunc) error {
				return nil
			}
			cm, err := newCertManagerClient(fakeConfigClient, proxy, pollImmediateWaiter)
			// set dummy expected hash
			cm.embeddedCertManagerManifestHash = expectedHash
			cm.embeddedCertManagerManifestVersion = expectedVersion

			g.Expect(err).ToNot(HaveOccurred())

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

func newFakeConfig(timeout string) fakeConfigClient {
	fakeReader := test.NewFakeReader().WithVar("cert-manager-timeout", timeout)

	client, _ := config.New("fake-config", config.InjectReader(fakeReader))
	return fakeConfigClient{
		fakeReader:         fakeReader,
		internalclient:     client,
		certManagerTimeout: timeout,
	}
}

type fakeConfigClient struct {
	fakeReader         *test.FakeReader
	internalclient     config.Client
	certManagerTimeout string
}

var _ config.Client = &fakeConfigClient{}

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
