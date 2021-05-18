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

package v1alpha3

import (
	"fmt"
	"os"
	"testing"

	// +kubebuilder:scaffold:imports
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/test/helpers"
)

var (
	testEnv *helpers.TestEnvironment
	ctx     = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	// Bootstrapping test environment
	utilruntime.Must(AddToScheme(scheme.Scheme))
	testEnv = helpers.NewTestEnvironment()
	go func() {
		if err := testEnv.StartManager(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the envtest manager: %v", err))
		}
	}()
	<-testEnv.Manager.Elected()
	testEnv.WaitForWebhooks()

	// Run tests
	code := m.Run()
	// Tearing down the test environment
	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the envtest: %v", err))
	}

	// Report exit code
	os.Exit(code)
}
