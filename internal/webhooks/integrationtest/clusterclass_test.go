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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/builder"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

var (
	clusterName1                       = "cluster1"
	clusterClassName1                  = "class1"
	infrastructureMachineTemplateName1 = "inframachinetemplate1"
	workerClassName1                   = "linux-worker"
	workerClassName2                   = "windows-worker"
)

func TestClusterClassDefaulting(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()


	//ns, err := env.CreateNamespace(ctx, "test-webhooks") // FIXME:
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-webhooks",
		},
	}


	// 1) Templates for Machine, Cluster, ControlPlane and Bootstrap.
	infrastructureMachineTemplate1 := builder.InfrastructureMachineTemplate(ns.Name, infrastructureMachineTemplateName1).Build()
	infrastructureClusterTemplate1 := builder.InfrastructureClusterTemplate(ns.Name, "infraclustertemplate1").
		Build()
	controlPlaneTemplate := builder.ControlPlaneTemplate(ns.Name, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate1).
		Build()
	bootstrapTemplate := builder.BootstrapTemplate(ns.Name, "bootstraptemplate").Build()

	// 2) ClusterClass definitions including definitions of MachineDeploymentClasses used inside the ClusterClass.
	machineDeploymentClass1 := builder.MachineDeploymentClass(workerClassName1).
		WithInfrastructureTemplate(infrastructureMachineTemplate1).
		WithBootstrapTemplate(bootstrapTemplate).
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"foo": "bar"}).
		Build()
	machineDeploymentClass2 := builder.MachineDeploymentClass(workerClassName2).
		WithInfrastructureTemplate(infrastructureMachineTemplate1).
		WithBootstrapTemplate(bootstrapTemplate).
		Build()
	clusterClass := builder.ClusterClass(ns.Name, clusterClassName1).
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate1).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate1).
		WithWorkerMachineDeploymentClasses(*machineDeploymentClass1, *machineDeploymentClass2).
		Build()

	tests := []struct {
		name      string
		new       []clusterv1.ClusterClassPatch
		old       []clusterv1.ClusterClassPatch
		expectErr bool
	}{
		{
			name: "test",
			new: []clusterv1.ClusterClassPatch{
				{
					Name: "patch",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "GenericControlPlaneTemplate",
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/",
									Value: &apiextensionsv1.JSON{Raw: []byte(`null`)},
								},
							},
						},
					},
				},
			},
			old: []clusterv1.ClusterClassPatch{
				{
					Name: "patch",
					Definitions: []clusterv1.PatchDefinition{
						{
							Selector: clusterv1.PatchSelector{
								APIVersion: "controlplane.cluster.x-k8s.io/v1beta1",
								Kind:       "GenericControlPlaneTemplate",
								MatchResources: clusterv1.PatchSelectorMatch{
									ControlPlane: true,
								},
							},
							JSONPatches: []clusterv1.JSONPatch{
								{
									Op:    "add",
									Path:  "/spec/template/spec/field",
									Value: &apiextensionsv1.JSON{Raw: []byte(`null`)},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create the webhook and add the fakeClient as its client.

			cc := clusterClass.DeepCopy()
			cc.Spec.Patches = tt.old
			g.Expect(env.CreateAndWait(ctx, cc)).To(Succeed())

			cc.Spec.Patches = tt.new
			g.Expect(env.Update(ctx, cc)).To(Succeed())

			//if tt.expectErr {
			//	g.Expect(err).To(HaveOccurred())
			//	return
			//}
			//g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
