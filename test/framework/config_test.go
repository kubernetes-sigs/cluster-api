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

package framework_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/framework"
)

func TestMustDefaultConfig(t *testing.T) {
	g := NewWithT(t)
	config := framework.MustDefaultConfig()
	config.Defaults()
	g.Expect(config.Validate()).To(Succeed())
	g.Expect(config.Components).To(HaveLen(4))
	g.Expect(config.Components[0].Waiters).To(HaveLen(2))
	g.Expect(config.Components[0].Waiters[1].Type).To(Equal(framework.PodsWaiter))
}
