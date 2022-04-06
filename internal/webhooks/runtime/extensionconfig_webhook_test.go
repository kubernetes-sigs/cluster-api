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

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
)

var (
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
		},
	}
	updatedExtension := extension.DeepCopy()
	updatedExtension.Spec.ClientConfig.URL = pointer.StringPtr("https://a-new-extension-address.com")
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
