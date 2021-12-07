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

package test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"k8s.io/component-base/featuregate"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/test/envtest"
)

var (
	ctx = ctrl.SetupSignalHandler()
	env *envtest.Environment
)

func TestMain(m *testing.M) {
	if err := feature.Gates.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", feature.ClusterTopology, true)); err != nil {
		panic(fmt.Sprintf("unable to set ClusterTopology feature gate: %v", err))
	}
	setupIndexes := func(ctx context.Context, mgr ctrl.Manager) {
		if err := index.AddDefaultIndexes(ctx, mgr); err != nil {
			panic(fmt.Sprintf("unable to setup index: %v", err))
		}
	}

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:            m,
		SetupEnv:     func(e *envtest.Environment) { env = e },
		SetupIndexes: setupIndexes,
	}))
}
