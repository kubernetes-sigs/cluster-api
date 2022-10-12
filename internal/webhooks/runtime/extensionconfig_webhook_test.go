/*
Copyright 2022 The Kubernetes Authors.

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

package runtime

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = runtimev1.AddToScheme(fakeScheme)
}

func TestExtensionConfigValidationFeatureGated(t *testing.T) {
	extension := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-extension",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String("https://extension-address.com"),
			},
			NamespaceSelector: &metav1.LabelSelector{},
		},
	}
	updatedExtension := extension.DeepCopy()
	updatedExtension.Spec.ClientConfig.URL = pointer.String("https://a-new-extension-address.com")
	tests := []struct {
		name        string
		new         *runtimev1.ExtensionConfig
		old         *runtimev1.ExtensionConfig
		featureGate bool
		expectErr   bool
	}{
		{
			name:        "creation should fail if feature flag is disabled",
			new:         extension,
			featureGate: false,
			expectErr:   true,
		},
		{
			name:        "update should fail if feature flag is disabled",
			old:         extension,
			new:         updatedExtension,
			featureGate: false,
			expectErr:   true,
		},
		{
			name:        "creation should succeed if feature flag is enabled",
			new:         extension,
			featureGate: true,
			expectErr:   false,
		},
		{
			name:        "update should fail if feature flag is enabled",
			old:         extension,
			new:         updatedExtension,
			featureGate: true,
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, tt.featureGate)()
			webhook := ExtensionConfig{}
			g := NewWithT(t)
			err := webhook.validate(context.TODO(), tt.old, tt.new)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestExtensionConfigDefault(t *testing.T) {
	g := NewWithT(t)
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

	extensionConfig := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-extension",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				Service: &runtimev1.ServiceReference{
					Name:      "name",
					Namespace: "namespace",
				},
			},
		},
	}

	extensionConfigWebhook := &ExtensionConfig{}
	t.Run("for Extension", util.CustomDefaultValidateTest(ctx, extensionConfig, extensionConfigWebhook))

	g.Expect(extensionConfigWebhook.Default(ctx, extensionConfig)).To(Succeed())
	g.Expect(extensionConfig.Spec.NamespaceSelector).To(Equal(&metav1.LabelSelector{}))
	g.Expect(extensionConfig.Spec.ClientConfig.Service.Port).To(Equal(pointer.Int32(443)))
}

func TestExtensionConfigValidate(t *testing.T) {
	extensionWithURL := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-extension",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String("https://extension-address.com"),
			},
		},
	}

	extensionWithService := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-extension",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				Service: &runtimev1.ServiceReference{
					Path:      pointer.String("/path/to/handler"),
					Port:      pointer.Int32(1),
					Name:      "foo",
					Namespace: "bar",
				}},
		},
	}

	extensionWithServiceAndURL := extensionWithURL.DeepCopy()
	extensionWithServiceAndURL.Spec.ClientConfig.Service = extensionWithService.Spec.ClientConfig.Service

	// Valid updated Extension
	updatedExtension := extensionWithURL.DeepCopy()
	updatedExtension.Spec.ClientConfig.URL = pointer.String("https://a-in-extension-address.com")

	extensionWithoutURLOrService := extensionWithURL.DeepCopy()
	extensionWithoutURLOrService.Spec.ClientConfig.URL = nil

	extensionWithInvalidServicePath := extensionWithService.DeepCopy()
	extensionWithInvalidServicePath.Spec.ClientConfig.Service.Path = pointer.String("https://example.com")

	extensionWithNoServiceName := extensionWithService.DeepCopy()
	extensionWithNoServiceName.Spec.ClientConfig.Service.Name = ""

	extensionWithBadServiceName := extensionWithService.DeepCopy()
	extensionWithBadServiceName.Spec.ClientConfig.Service.Name = "NOT_ALLOWED"

	extensionWithNoServiceNamespace := extensionWithService.DeepCopy()
	extensionWithNoServiceNamespace.Spec.ClientConfig.Service.Namespace = ""

	extensionWithBadServiceNamespace := extensionWithService.DeepCopy()
	extensionWithBadServiceNamespace.Spec.ClientConfig.Service.Namespace = "INVALID"

	badURLExtension := extensionWithURL.DeepCopy()
	badURLExtension.Spec.ClientConfig.URL = pointer.String("https//extension-address.com")

	badSchemeExtension := extensionWithURL.DeepCopy()
	badSchemeExtension.Spec.ClientConfig.URL = pointer.String("unknown://extension-address.com")

	extensionWithInvalidServicePort := extensionWithService.DeepCopy()
	extensionWithInvalidServicePort.Spec.ClientConfig.Service.Port = pointer.Int32(90000)

	extensionWithInvalidNamespaceSelector := extensionWithService.DeepCopy()
	extensionWithInvalidNamespaceSelector.Spec.NamespaceSelector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "foo",
				Operator: "bad-operator",
				Values:   []string{"foo", "bar"},
			},
		},
	}
	extensionWithValidNamespaceSelector := extensionWithService.DeepCopy()
	extensionWithValidNamespaceSelector.Spec.NamespaceSelector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "foo",
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}

	tests := []struct {
		name        string
		in          *runtimev1.ExtensionConfig
		old         *runtimev1.ExtensionConfig
		featureGate bool
		expectErr   bool
	}{
		{
			name:        "creation should fail if feature flag is disabled",
			in:          extensionWithURL,
			featureGate: false,
			expectErr:   true,
		},
		{
			name:        "update should fail if feature flag is disabled",
			old:         extensionWithURL,
			in:          updatedExtension,
			featureGate: false,
			expectErr:   true,
		},
		{
			name:        "creation should fail if no Service Name is defined",
			in:          extensionWithNoServiceName,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "creation should fail if Service Name violates Kubernetes naming rules",
			in:          extensionWithBadServiceName,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "creation should fail if no Service Namespace is defined",
			in:          extensionWithNoServiceNamespace,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "creation should fail if Service Namespace violates Kubernetes naming rules",
			in:          extensionWithBadServiceNamespace,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "creation should succeed if NamespaceSelector is correctly defined",
			in:          extensionWithValidNamespaceSelector,
			featureGate: true,
			expectErr:   false,
		},

		{
			name:        "creation should fail if NamespaceSelector is incorrectly defined",
			in:          extensionWithInvalidNamespaceSelector,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should fail if URL is invalid",
			old:         extensionWithURL,
			in:          badURLExtension,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should fail if URL scheme is invalid",
			old:         extensionWithURL,
			in:          badSchemeExtension,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should fail if URL and Service are both nil",
			old:         extensionWithURL,
			in:          extensionWithoutURLOrService,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should fail if both URL and Service are defined",
			old:         extensionWithService,
			in:          extensionWithServiceAndURL,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should fail if Service Path is invalid",
			old:         extensionWithService,
			in:          extensionWithInvalidServicePath,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should fail if Service Port is invalid",
			old:         extensionWithService,
			in:          extensionWithInvalidServicePort,
			featureGate: true,
			expectErr:   true,
		},
		{
			name:        "update should pass if updated Extension is valid",
			old:         extensionWithService,
			in:          extensionWithService,
			featureGate: true,
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, tt.featureGate)()
			g := NewWithT(t)
			webhook := &ExtensionConfig{}
			// Default the objects so we're not handling defaulted cases.
			g.Expect(webhook.Default(ctx, tt.in)).To(Succeed())
			if tt.old != nil {
				g.Expect(webhook.Default(ctx, tt.old)).To(Succeed())
			}

			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
