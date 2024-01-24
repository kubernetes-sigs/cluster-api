/*
Copyright 2023 The Kubernetes Authors.

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

package scope

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	ctx := context.Background()

	t.Run("should return the names of the upgrading MachineDeployments", func(t *testing.T) {
		stableMD := builder.MachineDeployment("ns", "stableMD").
			WithClusterName("cluster1").
			WithVersion("v1.2.3").
			Build()
		stableMDMachine := builder.Machine("ns", "stableMD-machine1").
			WithClusterName("cluster1").
			WithVersion("v1.2.3").
			Build()

		upgradingMD := builder.MachineDeployment("ns", "upgradingMD").
			WithClusterName("cluster2").
			WithVersion("v1.2.3").
			Build()
		upgradingMDMachine := builder.Machine("ns", "upgradingMD-machine1").
			WithClusterName("cluster2").
			WithVersion("v1.2.2").
			Build()

		objs := []client.Object{stableMD, stableMDMachine, upgradingMD, upgradingMDMachine}
		fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithScheme(scheme).Build()

		mdsStateMap := MachineDeploymentsStateMap{
			"stableMD":    {Object: stableMD},
			"upgradingMD": {Object: upgradingMD},
		}
		want := []string{"upgradingMD"}

		got, err := mdsStateMap.Upgrading(ctx, fakeClient)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeComparableTo(want))
	})
}
