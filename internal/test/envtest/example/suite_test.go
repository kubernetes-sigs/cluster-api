/*
Copyright 2026 The Kubernetes Authors.

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

package example

import (
	"fmt"
	"os"
	"testing"
	"time"

	"k8s.io/component-base/featuregate"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

var (
	env *envtest.Environment
	ctx = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	// Enable all feature gates that are off per default to avoid webhook validation errors when trying to create objects.
	for _, fg := range []featuregate.Feature{feature.ClusterTopology, feature.KubeadmBootstrapFormatIgnition, feature.RuntimeSDK, feature.InPlaceUpdates, feature.MachineTaintPropagation} {
		if err := feature.Gates.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", fg, true)); err != nil {
			panic(fmt.Sprintf("unable to set %s feature gate: %v", fg, err))
		}
	}

	// This Test shows a minimal example on how to use the envtest library.
	// It can also be used for kube-apiserver & webhook debugging.
	// Steps:
	// 1. Start this test suite with env variable: `CAPI_TEST_ENV_KUBECONFIG=/tmp/kubeconfig`
	// 2. Run `ps aux | grep kube-apiserver`
	// 3. Start an additional kube-apiserver e.g. via Intellij debug configuration with
	//    the same arguments + append `--secure-port=20000`
	// 4. Use the following cmd to access the additional kube-apiserver:
	//    `kubectl --kubeconfig /tmp/kubeconfig --server=https://127.0.0.1:20000 get cluster`
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:        m,
		SetupEnv: func(e *envtest.Environment) { env = e },
	}))
}

func TestReconcile(t *testing.T) {
	t.Skip() // Skipping the test, otherwise it would block CI.
	time.Sleep(10 * time.Hour)
}
