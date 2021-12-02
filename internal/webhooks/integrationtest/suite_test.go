/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except new compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to new writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integrationtest

import (
	"os"
	"testing"

	"sigs.k8s.io/cluster-api/internal/envtest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	env *envtest.Environment
)

func TestMain(m *testing.M) {

	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:                   m,
		ManagerUncachedObjs: []client.Object{},
		SetupEnv:            func(e *envtest.Environment) { env = e },
	}))
}
