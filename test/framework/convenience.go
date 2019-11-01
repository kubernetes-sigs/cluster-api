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

package framework

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// InstallComponents is a helper function that applies components, generally to a management cluster.
func InstallComponents(ctx context.Context, mgmt Applier, components ...ComponentGenerator) {
	Describe("Installing the provider components", func() {
		for _, component := range components {
			By(fmt.Sprintf("installing %s", component.GetName()))
			c, err := component.Manifests(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgmt.Apply(ctx, c)).NotTo(HaveOccurred())
		}
	})
}

// WaitForAPIServiceAvailable will wait for an an APIService to be available.
// For example, kubectl wait --for=condition=Available --timeout=300s apiservice v1beta1.webhook.cert-manager.io
func WaitForAPIServiceAvailable(ctx context.Context, mgmt Waiter, serviceName string) {
	err := mgmt.Wait(ctx, "--for", "condition=Available", "--timeout", "300s", "apiservice", serviceName)
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)
}
