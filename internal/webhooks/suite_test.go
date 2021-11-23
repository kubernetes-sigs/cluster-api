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

package webhooks

import (
	"fmt"
	"os"
	"testing"

	// +kubebuilder:scaffold:imports
	"sigs.k8s.io/cluster-api/internal/envtest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	env *envtest.Environment
	ctx = ctrl.SetupSignalHandler()
)

// FIXME(sbueringer): just part of this PR to prove that it works.
func TestMain(m *testing.M) {
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:        m,
		SetupEnv: func(e *envtest.Environment) { env = e },
		SetupWebhooks: func(mgr ctrl.Manager) {
			if err := SetupWebhooksWithManager(mgr); err != nil {
				panic(fmt.Sprintf("Failed to set up webhooks: %v", err))
			}
		},
	}))
}
