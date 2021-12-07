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
	"fmt"
	"log"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"

	. "github.com/onsi/gomega"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestClusterClassWebhook_Update_Performance(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-topology-clusterclass-webhook")
	g.Expect(err).ToNot(HaveOccurred())

	vars := generateRegexVariables(200, "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
	updatedVars := generateRegexVariables(200, "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

	clusterClass := builder.ClusterClass(ns.Name, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(ns.Name, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(ns.Name, "cp1").
				Build()).
		WithVariables(vars...).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("md1").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate(ns.Name, "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate(ns.Name, "bootstrap1").Build()).
				Build()).
		Build()

	clusters := generateClusters(ns.Name, clusterClass.Name, vars, 700)

	g.Expect(env.Create(ctx, clusterClass)).To(Succeed())

	for _, cluster := range clusters {
		g.Expect(env.Create(ctx, cluster)).To(Succeed())
	}
	got := &clusterv1.ClusterClass{}
	g.Expect(env.Get(ctx, client.ObjectKey{Namespace: ns.Name, Name: "class1"}, got)).To(Succeed())
	got.Spec.Variables = updatedVars

	start := time.Now()

	g.Expect(env.Update(ctx, got))
	elapsed := time.Since(start)
	log.Printf("Update took %s", elapsed)
}

func generateClusters(namespace, clusterClass string, vars []clusterv1.ClusterClassVariable, number int) []*clusterv1.Cluster {
	out := []*clusterv1.Cluster{}
	for i := 0; i < number; i++ {
		out = append(out, builder.Cluster(namespace, fmt.Sprintf("cluster%v", i)).
			WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
			WithTopology(
				builder.ClusterTopology().
					WithVersion("v1.20.1").
					WithClass(clusterClass).
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("md1").
							Build(),
					).
					WithVariables(
						clusterVariablesFromClusterClassVariables(vars)...).
					Build()).
			Build())
	}
	return out
}

func generateRegexVariables(number int, pattern string) []clusterv1.ClusterClassVariable {
	out := []clusterv1.ClusterClassVariable{}
	for i := 0; i < number; i++ {
		out = append(out,
			clusterv1.ClusterClassVariable{
				Name:     fmt.Sprintf("var%v", i),
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "string",
						Pattern: pattern,
					},
				},
			},
		)
	}
	return out
}

func clusterVariablesFromClusterClassVariables(varDefs []clusterv1.ClusterClassVariable) []clusterv1.ClusterVariable {
	out := []clusterv1.ClusterVariable{}
	for i := 0; i < len(varDefs); i++ {
		id := []byte(fmt.Sprintf("%v%v%v", "\"", uuid.NewUUID(), "\""))
		out = append(out,
			clusterv1.ClusterVariable{
				Name: varDefs[i].Name,
				Value: v1.JSON{
					Raw: id,
				},
			})
	}
	return out
}
