/*
Copyright 2019 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/gomega"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_newClusterClient_YamlProcessor(t *testing.T) {
	tests := []struct {
		name   string
		opts   []Option
		assert func(*WithT, yaml.Processor)
	}{
		{
			name: "it creates a cluster client with simple yaml processor by default",
			assert: func(g *WithT, p yaml.Processor) {
				_, ok := (p).(*yaml.SimpleProcessor)
				g.Expect(ok).To(BeTrue())
			},
		},
		{
			name: "it creates a cluster client with specified yaml processor",
			opts: []Option{InjectYamlProcessor(test.NewFakeProcessor())},
			assert: func(g *WithT, p yaml.Processor) {
				_, ok := (p).(*yaml.SimpleProcessor)
				g.Expect(ok).To(BeFalse())
				_, ok = (p).(*test.FakeProcessor)
				g.Expect(ok).To(BeTrue())
			},
		},
		{
			name: "it creates a cluster client with simple yaml processor even if injected with nil processor",
			opts: []Option{InjectYamlProcessor(nil)},
			assert: func(g *WithT, p yaml.Processor) {
				g.Expect(p).ToNot(BeNil())
				_, ok := (p).(*yaml.SimpleProcessor)
				g.Expect(ok).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			client := newClusterClient(Kubeconfig{}, &fakeConfigClient{}, tt.opts...)
			g.Expect(client).ToNot(BeNil())
			tt.assert(g, client.processor)
		})
	}
}
