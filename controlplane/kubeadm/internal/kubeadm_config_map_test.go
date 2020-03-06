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
	"testing"

	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var (
	HaveOccurred     = gomega.HaveOccurred
	Succeed          = gomega.Succeed
	SatisfyAll       = gomega.SatisfyAll
	HaveLen          = gomega.HaveLen
	HaveKey          = gomega.HaveKey
	HaveKeyWithValue = gomega.HaveKeyWithValue
	WithTransform    = gomega.WithTransform
	BeEmpty          = gomega.BeEmpty
)

func Test_kubeadmConfig_RemoveAPIEndpoint(t *testing.T) {
	g := gomega.NewWithT(t)
	original := &corev1.ConfigMap{
		Data: map[string]string{
			"ClusterStatus": `apiEndpoints:
  ip-10-0-0-1.ec2.internal:
    advertiseAddress: 10.0.0.1
    bindPort: 6443
  ip-10-0-0-2.ec2.internal:
    advertiseAddress: 10.0.0.2
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: bar
  ip-10-0-0-3.ec2.internal:
    advertiseAddress: 10.0.0.3
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: baz
  ip-10-0-0-4.ec2.internal:
    advertiseAddress: 10.0.0.4
    bindPort: 6443
    someFieldThatIsAddedInTheFuture: fizzbuzz
apiVersion: kubeadm.k8s.io/vNbetaM
kind: ClusterStatus`,
		},
	}
	kc := kubeadmConfig{ConfigMap: original}
	g.Expect(kc.RemoveAPIEndpoint("ip-10-0-0-3.ec2.internal")).ToNot(HaveOccurred())
	g.Expect(kc.ConfigMap.Data).To(HaveKey("ClusterStatus"))
	var status struct {
		APIEndpoints map[string]interface{} `yaml:"apiEndpoints"`
		APIVersion   string                 `yaml:"apiVersion"`
		Kind         string                 `yaml:"kind"`

		Extra map[string]interface{} `yaml:",inline"`
	}
	g.Expect(yaml.UnmarshalStrict([]byte(kc.ConfigMap.Data["ClusterStatus"]), &status)).To(Succeed())
	g.Expect(status.Extra).To(BeEmpty())

	g.Expect(status.APIEndpoints).To(SatisfyAll(
		HaveLen(3),
		HaveKey("ip-10-0-0-1.ec2.internal"),
		HaveKey("ip-10-0-0-2.ec2.internal"),
		HaveKey("ip-10-0-0-4.ec2.internal"),
		WithTransform(func(ep map[string]interface{}) interface{} {
			return ep["ip-10-0-0-4.ec2.internal"]
		}, SatisfyAll(
			HaveKeyWithValue("advertiseAddress", "10.0.0.4"),
			HaveKey("bindPort"),
			HaveKey("someFieldThatIsAddedInTheFuture"),
		)),
	))
}
