/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/cluster-api/feature"
)

func TestDockerClusterTemplateValidationFeatureGateEnabled(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	t.Run("create dockerclustertemplate should pass if gate enabled and valid dockerclustertemplate", func(t *testing.T) {
		g := NewWithT(t)
		dct := &DockerClusterTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dockerclustertemplate-test",
				Namespace: "test-namespace",
			},
			Spec: DockerClusterTemplateSpec{
				Template: DockerClusterTemplateResource{
					Spec: DockerClusterSpec{},
				},
			},
		}
		g.Expect(dct.ValidateCreate()).To(Succeed())
	})
}

func TestDockerClusterTemplateValidationFeatureGateDisabled(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create DockerClusterTemplate.
	t.Run("create dockerclustertemplate should not pass if gate disabled and valid dockerclustertemplate", func(t *testing.T) {
		g := NewWithT(t)
		dct := &DockerClusterTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dockerclustertemplate-test",
				Namespace: "test-namespace",
			},
			Spec: DockerClusterTemplateSpec{
				Template: DockerClusterTemplateResource{
					Spec: DockerClusterSpec{},
				},
			},
		}
		g.Expect(dct.ValidateCreate()).NotTo(Succeed())
	})
}
