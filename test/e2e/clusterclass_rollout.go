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

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/machineset"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ClusterClassRolloutSpecInput is the input for ClusterClassRolloutSpec.
type ClusterClassRolloutSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers (ex: CAPD + in-memory) are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	// Flavor is the cluster-template flavor used to create the Cluster for testing.
	// NOTE: The template must be using ClusterClass, KCP and CABPK as this test is specifically
	// testing ClusterClass and KCP rollout behavior.
	Flavor string
}

// ClusterClassRolloutSpec implements a test that verifies the ClusterClass rollout behavior.
// It specifically covers the in-place propagation behavior from ClusterClass / Cluster topology to all
// objects of the Cluster topology (e.g. KCP, MD) and even tests label propagation to the Nodes of the
// workload cluster.
// Thus, the test consists of the following steps:
//   - Deploy Cluster using a ClusterClass and wait until it is fully provisioned.
//   - Assert cluster objects
//   - Modify in-place mutable fields of KCP and the MachineDeployments
//   - Verify that fields were mutated in-place and assert cluster objects
//   - Modify fields in KCP and MachineDeployments to trigger a full rollout of all Machines
//   - Verify that all Machines have been replaced and assert cluster objects
//   - Set RolloutAfter on KCP and MachineDeployments to trigger a full rollout of all Machines
//   - Verify that all Machines have been replaced and assert cluster objects
//
// While asserting cluster objects we check that all objects have the right labels, annotations and selectors.
func ClusterClassRolloutSpec(ctx context.Context, inputGetter func() ClusterClassRolloutSpecInput) {
	var (
		specName         = "clusterclass-rollout"
		input            ClusterClassRolloutSpecInput
		namespace        *corev1.Namespace
		cancelWatches    context.CancelFunc
		clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveValidVersion(input.E2EConfig.GetVariable(KubernetesVersion)))

		// Set up a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should successfully rollout the managed topology upon changes to the ClusterClass", func() {
		By("Creating a workload cluster")
		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   input.Flavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64(1),
				WorkerMachineCount:       pointer.Int64(1),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)
		assertClusterObjects(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, clusterResources.ClusterClass)

		By("Rolling out changes to control plane and MachineDeployments (in-place)")
		machinesBeforeUpgrade := getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
		By("Modifying the control plane configuration via Cluster topology and wait for changes to be applied to the control plane object (in-place)")
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      clusterResources.Cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				// Drop existing labels and annotations and set new ones.
				topology.Metadata.Labels = map[string]string{
					"Cluster.topology.controlPlane.newLabel": "Cluster.topology.controlPlane.newLabelValue",
				}
				topology.Metadata.Annotations = map[string]string{
					"Cluster.topology.controlPlane.newAnnotation": "Cluster.topology.controlPlane.newAnnotationValue",
				}
				topology.NodeDrainTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second}        //nolint:gosec
				topology.NodeDeletionTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second}     //nolint:gosec
				topology.NodeVolumeDetachTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second} //nolint:gosec
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})
		By("Modifying the MachineDeployments configuration via Cluster topology and wait for changes to be applied to the MachineDeployments (in-place)")
		modifyMachineDeploymentViaClusterAndWait(ctx, modifyMachineDeploymentViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      clusterResources.Cluster,
			ModifyMachineDeploymentTopology: func(topology *clusterv1.MachineDeploymentTopology) {
				// Drop existing labels and annotations and set new ones.
				topology.Metadata.Labels = map[string]string{
					"Cluster.topology.machineDeployment.newLabel": "Cluster.topology.machineDeployment.newLabelValue",
				}
				topology.Metadata.Annotations = map[string]string{
					"Cluster.topology.machineDeployment.newAnnotation": "Cluster.topology.machineDeployment.newAnnotationValue",
				}
				topology.NodeDrainTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second}        //nolint:gosec
				topology.NodeDeletionTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second}     //nolint:gosec
				topology.NodeVolumeDetachTimeout = &metav1.Duration{Duration: time.Duration(rand.Intn(20)) * time.Second} //nolint:gosec
				topology.MinReadySeconds = pointer.Int32(rand.Int31n(20))                                                 //nolint:gosec
				topology.Strategy = &clusterv1.MachineDeploymentStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
						MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 5 + rand.Int31n(20)}, //nolint:gosec
						DeletePolicy:   pointer.String(string(clusterv1.NewestMachineSetDeletePolicy)),
					},
				}
			},
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})
		By("Verifying there are no unexpected rollouts through in-place rollout")
		Consistently(func(g Gomega) {
			machinesAfterUpgrade := getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
			g.Expect(machinesAfterUpgrade.Equal(machinesBeforeUpgrade)).To(BeTrue(), "Machines must not be replaced through in-place rollout")
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		assertClusterObjects(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, clusterResources.ClusterClass)

		By("Rolling out changes to control plane and MachineDeployments (rollout)")
		machinesBeforeUpgrade = getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
		By("Modifying the control plane configuration via ClusterClass and wait for changes to be applied to the control plane object (rollout)")
		modifyControlPlaneViaClusterClassAndWait(ctx, modifyClusterClassControlPlaneAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ClusterClass: clusterResources.ClusterClass,
			Cluster:      clusterResources.Cluster,
			ModifyControlPlaneFields: map[string]interface{}{
				"spec.kubeadmConfigSpec.verbosity": int64(4),
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})
		By("Modifying the MachineDeployment configuration via ClusterClass and wait for changes to be applied to the MachineDeployments (rollout)")
		modifyMachineDeploymentViaClusterClassAndWait(ctx, modifyMachineDeploymentViaClusterClassAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ClusterClass: clusterResources.ClusterClass,
			Cluster:      clusterResources.Cluster,
			ModifyBootstrapConfigTemplateFields: map[string]interface{}{
				"spec.template.spec.verbosity": int64(4),
			},
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})
		By("Verifying all Machines are replaced through rollout")
		Eventually(func(g Gomega) {
			machinesAfterUpgrade := getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
			g.Expect(machinesAfterUpgrade.HasAny(machinesBeforeUpgrade.UnsortedList()...)).To(BeFalse(), "All Machines must be replaced through rollout")
		}, input.E2EConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())
		assertClusterObjects(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, clusterResources.ClusterClass)

		By("Rolling out control plane and MachineDeployment (rolloutAfter)")
		machinesBeforeUpgrade = getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
		By("Setting rolloutAfter on control plane")
		Eventually(func(g Gomega) {
			kcp := clusterResources.ControlPlane
			g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(kcp), kcp)).To(Succeed())
			patchHelper, err := patch.NewHelper(kcp, input.BootstrapClusterProxy.GetClient())
			g.Expect(err).ToNot(HaveOccurred())
			kcp.Spec.RolloutAfter = &metav1.Time{Time: time.Now()}
			g.Expect(patchHelper.Patch(ctx, kcp)).To(Succeed())
		}, 10*time.Second, 1*time.Second).Should(Succeed())
		By("Setting rolloutAfter on MachineDeployments")
		for _, md := range clusterResources.MachineDeployments {
			Eventually(func(g Gomega) {
				g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(md), md)).To(Succeed())
				patchHelper, err := patch.NewHelper(md, input.BootstrapClusterProxy.GetClient())
				g.Expect(err).ToNot(HaveOccurred())
				md.Spec.RolloutAfter = &metav1.Time{Time: time.Now()}
				g.Expect(patchHelper.Patch(ctx, md)).To(Succeed())
			}, 10*time.Second, 1*time.Second).Should(Succeed())
		}
		By("Verifying all Machines are replaced through rolloutAfter")
		Eventually(func(g Gomega) {
			machinesAfterUpgrade := getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
			g.Expect(machinesAfterUpgrade.HasAny(machinesBeforeUpgrade.UnsortedList()...)).To(BeFalse(), "All Machines must be replaced through rollout with rolloutAfter")
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade")...).Should(Succeed())
		assertClusterObjects(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, clusterResources.ClusterClass)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

// assertClusterObjects asserts cluster objects by checking that all objects have the right labels, annotations and selectors.
func assertClusterObjects(ctx context.Context, clusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) {
	By("Checking cluster objects have the right labels, annotations and selectors")

	Eventually(func(g Gomega) {
		// Get Cluster and ClusterClass objects.
		clusterObjects := getClusterObjects(ctx, g, clusterProxy, cluster)
		clusterClassObjects := getClusterClassObjects(ctx, g, clusterProxy, clusterClass)

		// InfrastructureCluster
		assertInfrastructureCluster(g, clusterClassObjects, clusterObjects, cluster, clusterClass)

		// ControlPlane
		assertControlPlane(g, clusterClassObjects, clusterObjects, cluster, clusterClass)
		assertControlPlaneMachines(g, clusterObjects, cluster)

		// MachineDeployments
		assertMachineDeployments(g, clusterClassObjects, clusterObjects, cluster, clusterClass)
		assertMachineSets(g, clusterObjects, cluster)
		assertMachineSetsMachines(g, clusterObjects, cluster)

		By("All cluster objects have the right labels, annotations and selectors")
	}, 30*time.Second, 1*time.Second).Should(Succeed())
}

func assertInfrastructureCluster(g Gomega, clusterClassObjects clusterClassObjects, clusterObjects clusterObjects, cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) {
	ccInfrastructureClusterTemplateTemplateMetadata := mustMetadata(contract.InfrastructureClusterTemplate().Template().Metadata().Get(clusterClassObjects.InfrastructureClusterTemplate))

	// InfrastructureCluster.metadata
	g.Expect(clusterObjects.InfrastructureCluster.GetLabels()).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.ClusterNameLabel:          cluster.Name,
				clusterv1.ClusterTopologyOwnedLabel: "",
			},
			ccInfrastructureClusterTemplateTemplateMetadata.Labels,
		),
	))
	g.Expect(clusterObjects.InfrastructureCluster.GetAnnotations()).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(clusterClass.Spec.Infrastructure.Ref),
				clusterv1.TemplateClonedFromNameAnnotation:      clusterClass.Spec.Infrastructure.Ref.Name,
			},
			ccInfrastructureClusterTemplateTemplateMetadata.Annotations,
		),
	))
}

func assertControlPlane(g Gomega, clusterClassObjects clusterClassObjects, clusterObjects clusterObjects, cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) {
	ccControlPlaneTemplateTemplateMetadata := mustMetadata(contract.ControlPlaneTemplate().Template().Metadata().Get(clusterClassObjects.ControlPlaneTemplate))
	ccControlPlaneTemplateMachineTemplateMetadata := mustMetadata(contract.ControlPlaneTemplate().Template().MachineTemplate().Metadata().Get(clusterClassObjects.ControlPlaneTemplate))
	ccControlPlaneInfrastructureMachineTemplateTemplateMetadata := mustMetadata(contract.InfrastructureMachineTemplate().Template().Metadata().Get(clusterClassObjects.ControlPlaneInfrastructureMachineTemplate))
	controlPlaneMachineTemplateMetadata := mustMetadata(contract.ControlPlane().MachineTemplate().Metadata().Get(clusterObjects.ControlPlane))
	controlPlaneInfrastructureMachineTemplateTemplateMetadata := mustMetadata(contract.InfrastructureMachineTemplate().Template().Metadata().Get(clusterObjects.ControlPlaneInfrastructureMachineTemplate))

	// ControlPlane.metadata
	g.Expect(clusterObjects.ControlPlane.GetLabels()).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.ClusterNameLabel:          cluster.Name,
				clusterv1.ClusterTopologyOwnedLabel: "",
			},
			cluster.Spec.Topology.ControlPlane.Metadata.Labels,
			clusterClass.Spec.ControlPlane.Metadata.Labels,
			ccControlPlaneTemplateTemplateMetadata.Labels,
		),
	))
	g.Expect(clusterObjects.ControlPlane.GetAnnotations()).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(clusterClass.Spec.ControlPlane.Ref),
				clusterv1.TemplateClonedFromNameAnnotation:      clusterClass.Spec.ControlPlane.Ref.Name,
			},
			cluster.Spec.Topology.ControlPlane.Metadata.Annotations,
			clusterClass.Spec.ControlPlane.Metadata.Annotations,
			ccControlPlaneTemplateTemplateMetadata.Annotations,
		),
	))

	// ControlPlane.spec.machineTemplate.metadata
	g.Expect(controlPlaneMachineTemplateMetadata.Labels).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.ClusterNameLabel:          cluster.Name,
				clusterv1.ClusterTopologyOwnedLabel: "",
			},
			cluster.Spec.Topology.ControlPlane.Metadata.Labels,
			clusterClass.Spec.ControlPlane.Metadata.Labels,
			ccControlPlaneTemplateMachineTemplateMetadata.Labels,
		),
	))
	g.Expect(controlPlaneMachineTemplateMetadata.Annotations).To(BeEquivalentTo(
		union(
			cluster.Spec.Topology.ControlPlane.Metadata.Annotations,
			clusterClass.Spec.ControlPlane.Metadata.Annotations,
			ccControlPlaneTemplateMachineTemplateMetadata.Annotations,
		),
	))

	// ControlPlane InfrastructureMachineTemplate.metadata
	g.Expect(clusterObjects.ControlPlaneInfrastructureMachineTemplate.GetLabels()).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.ClusterNameLabel:          cluster.Name,
				clusterv1.ClusterTopologyOwnedLabel: "",
			},
			clusterClassObjects.ControlPlaneInfrastructureMachineTemplate.GetLabels(),
		),
	))
	g.Expect(clusterObjects.ControlPlaneInfrastructureMachineTemplate.GetAnnotations()).To(BeEquivalentTo(
		union(
			map[string]string{
				clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref),
				clusterv1.TemplateClonedFromNameAnnotation:      clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref.Name,
			},
			clusterClassObjects.ControlPlaneInfrastructureMachineTemplate.GetAnnotations(),
		).without(g, corev1.LastAppliedConfigAnnotation),
	))

	// ControlPlane InfrastructureMachineTemplate.spec.template.metadata
	g.Expect(controlPlaneInfrastructureMachineTemplateTemplateMetadata.Labels).To(BeEquivalentTo(
		ccControlPlaneInfrastructureMachineTemplateTemplateMetadata.Labels,
	))
	g.Expect(controlPlaneInfrastructureMachineTemplateTemplateMetadata.Annotations).To(BeEquivalentTo(
		ccControlPlaneInfrastructureMachineTemplateTemplateMetadata.Annotations,
	))
}

func assertControlPlaneMachines(g Gomega, clusterObjects clusterObjects, cluster *clusterv1.Cluster) {
	controlPlaneMachineTemplateMetadata := mustMetadata(contract.ControlPlane().MachineTemplate().Metadata().Get(clusterObjects.ControlPlane))
	controlPlaneInfrastructureMachineTemplateTemplateMetadata := mustMetadata(contract.InfrastructureMachineTemplate().Template().Metadata().Get(clusterObjects.ControlPlaneInfrastructureMachineTemplate))

	for _, machine := range clusterObjects.ControlPlaneMachines {
		// ControlPlane Machine.metadata
		g.Expect(machine.Labels).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:             cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:    "",
					clusterv1.MachineControlPlaneLabel:     "",
					clusterv1.MachineControlPlaneNameLabel: clusterObjects.ControlPlane.GetName(),
				},
				controlPlaneMachineTemplateMetadata.Labels,
			),
		))
		g.Expect(
			union(
				machine.Annotations,
			).without(g, controlplanev1.KubeadmClusterConfigurationAnnotation),
		).To(BeEquivalentTo(
			controlPlaneMachineTemplateMetadata.Annotations,
		))

		// ControlPlane Machine InfrastructureMachine.metadata
		infrastructureMachine := clusterObjects.InfrastructureMachineByMachine[machine.Name]
		controlPlaneMachineTemplateInfrastructureRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(clusterObjects.ControlPlane)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(infrastructureMachine.GetLabels()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:             cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:    "",
					clusterv1.MachineControlPlaneLabel:     "",
					clusterv1.MachineControlPlaneNameLabel: clusterObjects.ControlPlane.GetName(),
				},
				controlPlaneMachineTemplateMetadata.Labels,
				controlPlaneInfrastructureMachineTemplateTemplateMetadata.Labels,
			),
		))
		g.Expect(infrastructureMachine.GetAnnotations()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(controlPlaneMachineTemplateInfrastructureRef),
					clusterv1.TemplateClonedFromNameAnnotation:      controlPlaneMachineTemplateInfrastructureRef.Name,
				},
				controlPlaneMachineTemplateMetadata.Annotations,
				controlPlaneInfrastructureMachineTemplateTemplateMetadata.Annotations,
			),
		))

		// ControlPlane Machine BootstrapConfig.metadata
		bootstrapConfig := clusterObjects.BootstrapConfigByMachine[machine.Name]
		g.Expect(bootstrapConfig.GetLabels()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:             cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:    "",
					clusterv1.MachineControlPlaneLabel:     "",
					clusterv1.MachineControlPlaneNameLabel: clusterObjects.ControlPlane.GetName(),
				},
				controlPlaneMachineTemplateMetadata.Labels,
			),
		))
		g.Expect(
			union(
				bootstrapConfig.GetAnnotations(),
			).without(g, clusterv1.MachineCertificatesExpiryDateAnnotation),
		).To(BeEquivalentTo(
			controlPlaneMachineTemplateMetadata.Annotations,
		))

		// ControlPlane Machine Node.metadata
		node := clusterObjects.NodesByMachine[machine.Name]
		for k, v := range getManagedLabels(machine.Labels) {
			g.Expect(node.GetLabels()).To(HaveKeyWithValue(k, v))
		}
	}
}

func assertMachineDeployments(g Gomega, clusterClassObjects clusterClassObjects, clusterObjects clusterObjects, cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) {
	for _, machineDeployment := range clusterObjects.MachineDeployments {
		mdTopology := getMDTopology(cluster, machineDeployment)
		mdClass := getMDClass(cluster, clusterClass, machineDeployment)

		// MachineDeployment.metadata
		g.Expect(machineDeployment.Labels).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:                          cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				},
				mdTopology.Metadata.Labels,
				mdClass.Template.Metadata.Labels,
			),
		))
		g.Expect(
			union(
				machineDeployment.Annotations,
			).without(g, clusterv1.RevisionAnnotation),
		).To(BeEquivalentTo(
			union(
				mdTopology.Metadata.Annotations,
				mdClass.Template.Metadata.Annotations,
			),
		))

		// MachineDeployment.spec.selector
		g.Expect(machineDeployment.Spec.Selector.MatchLabels).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:                          cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				},
			),
		))
		// MachineDeployment.spec.template.metadata
		g.Expect(machineDeployment.Spec.Template.Labels).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:                          cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				},
				mdTopology.Metadata.Labels,
				mdClass.Template.Metadata.Labels,
			),
		))
		g.Expect(machineDeployment.Spec.Template.Annotations).To(BeEquivalentTo(
			union(
				mdTopology.Metadata.Annotations,
				mdClass.Template.Metadata.Annotations,
			),
		))

		// MachineDeployment InfrastructureMachineTemplate.metadata
		ccInfrastructureMachineTemplate := clusterClassObjects.InfrastructureMachineTemplateByMachineDeploymentClass[mdClass.Class]
		ccInfrastructureMachineTemplateTemplateMetadata := mustMetadata(contract.InfrastructureMachineTemplate().Template().Metadata().Get(ccInfrastructureMachineTemplate))
		infrastructureMachineTemplate := clusterObjects.InfrastructureMachineTemplateByMachineDeployment[machineDeployment.Name]
		infrastructureMachineTemplateTemplateMetadata := mustMetadata(contract.InfrastructureMachineTemplate().Template().Metadata().Get(infrastructureMachineTemplate))
		g.Expect(infrastructureMachineTemplate.GetLabels()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:                          cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				},
				ccInfrastructureMachineTemplate.GetLabels(),
			),
		))
		g.Expect(infrastructureMachineTemplate.GetAnnotations()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(mdClass.Template.Infrastructure.Ref),
					clusterv1.TemplateClonedFromNameAnnotation:      mdClass.Template.Infrastructure.Ref.Name,
				},
				ccInfrastructureMachineTemplate.GetAnnotations(),
			).without(g, corev1.LastAppliedConfigAnnotation),
		))
		// MachineDeployment InfrastructureMachineTemplate.spec.template.metadata
		g.Expect(infrastructureMachineTemplateTemplateMetadata.Labels).To(BeEquivalentTo(
			ccInfrastructureMachineTemplateTemplateMetadata.Labels,
		))
		g.Expect(infrastructureMachineTemplateTemplateMetadata.Annotations).To(BeEquivalentTo(
			ccInfrastructureMachineTemplateTemplateMetadata.Annotations,
		))

		// MachineDeployment BootstrapConfigTemplate.metadata
		ccBootstrapConfigTemplate := clusterClassObjects.BootstrapConfigTemplateByMachineDeploymentClass[mdClass.Class]
		ccBootstrapConfigTemplateTemplateMetadata := mustMetadata(contract.BootstrapConfigTemplate().Template().Metadata().Get(ccBootstrapConfigTemplate))
		bootstrapConfigTemplate := clusterObjects.BootstrapConfigTemplateByMachineDeployment[machineDeployment.Name]
		bootstrapConfigTemplateTemplateMetadata := mustMetadata(contract.BootstrapConfigTemplate().Template().Metadata().Get(bootstrapConfigTemplate))
		g.Expect(bootstrapConfigTemplate.GetLabels()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.ClusterNameLabel:                          cluster.Name,
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				},
				ccBootstrapConfigTemplate.GetLabels(),
			),
		))
		g.Expect(bootstrapConfigTemplate.GetAnnotations()).To(BeEquivalentTo(
			union(
				map[string]string{
					clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(mdClass.Template.Bootstrap.Ref),
					clusterv1.TemplateClonedFromNameAnnotation:      mdClass.Template.Bootstrap.Ref.Name,
				},
				ccBootstrapConfigTemplate.GetAnnotations(),
			).without(g, corev1.LastAppliedConfigAnnotation),
		))
		// MachineDeployment BootstrapConfigTemplate.spec.template.metadata
		g.Expect(bootstrapConfigTemplateTemplateMetadata.Labels).To(BeEquivalentTo(
			ccBootstrapConfigTemplateTemplateMetadata.Labels,
		))
		g.Expect(bootstrapConfigTemplateTemplateMetadata.Annotations).To(BeEquivalentTo(
			ccBootstrapConfigTemplateTemplateMetadata.Annotations,
		))
	}
}

func assertMachineSets(g Gomega, clusterObjects clusterObjects, cluster *clusterv1.Cluster) {
	for _, machineDeployment := range clusterObjects.MachineDeployments {
		mdTopology := getMDTopology(cluster, machineDeployment)

		for _, machineSet := range clusterObjects.MachineSetsByMachineDeployment[machineDeployment.Name] {
			machineTemplateHash := machineSet.Labels[clusterv1.MachineDeploymentUniqueLabel]

			// MachineDeployment MachineSet.metadata
			g.Expect(machineSet.Labels).To(BeEquivalentTo(
				union(
					map[string]string{
						clusterv1.ClusterNameLabel:                          cluster.Name,
						clusterv1.ClusterTopologyOwnedLabel:                 "",
						clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
						clusterv1.MachineDeploymentNameLabel:                machineDeployment.Name,
						clusterv1.MachineDeploymentUniqueLabel:              machineTemplateHash,
					},
					machineDeployment.Spec.Template.Labels,
				),
			))
			g.Expect(
				union(
					machineSet.Annotations,
				).without(g, clusterv1.DesiredReplicasAnnotation, clusterv1.MaxReplicasAnnotation, clusterv1.RevisionAnnotation),
			).To(BeEquivalentTo(
				union(
					machineDeployment.Annotations,
				).without(g, clusterv1.RevisionAnnotation),
			))
			// MachineDeployment MachineSet.spec.selector
			g.Expect(machineSet.Spec.Selector.MatchLabels).To(BeEquivalentTo(
				union(
					map[string]string{
						clusterv1.ClusterNameLabel:                          cluster.Name,
						clusterv1.ClusterTopologyOwnedLabel:                 "",
						clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
						clusterv1.MachineDeploymentUniqueLabel:              machineTemplateHash,
					},
				),
			))
			// MachineDeployment MachineSet.spec.template.metadata
			g.Expect(machineSet.Spec.Template.Labels).To(BeEquivalentTo(
				union(
					map[string]string{
						clusterv1.ClusterNameLabel:                          cluster.Name,
						clusterv1.ClusterTopologyOwnedLabel:                 "",
						clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
						clusterv1.MachineDeploymentUniqueLabel:              machineTemplateHash,
					},
					machineDeployment.Spec.Template.Labels,
				),
			))
			g.Expect(machineSet.Spec.Template.Annotations).To(BeEquivalentTo(
				machineDeployment.Spec.Template.Annotations,
			))
		}
	}
}

func assertMachineSetsMachines(g Gomega, clusterObjects clusterObjects, cluster *clusterv1.Cluster) {
	for _, machineDeployment := range clusterObjects.MachineDeployments {
		mdTopology := getMDTopology(cluster, machineDeployment)
		infrastructureMachineTemplate := clusterObjects.InfrastructureMachineTemplateByMachineDeployment[machineDeployment.Name]
		infrastructureMachineTemplateTemplateMetadata := mustMetadata(contract.InfrastructureMachineTemplate().Template().Metadata().Get(infrastructureMachineTemplate))
		bootstrapConfigTemplate := clusterObjects.BootstrapConfigTemplateByMachineDeployment[machineDeployment.Name]
		bootstrapConfigTemplateTemplateMetadata := mustMetadata(contract.BootstrapConfigTemplate().Template().Metadata().Get(bootstrapConfigTemplate))

		for _, machineSet := range clusterObjects.MachineSetsByMachineDeployment[machineDeployment.Name] {
			machineTemplateHash := machineSet.Labels[clusterv1.MachineDeploymentUniqueLabel]

			for _, machine := range clusterObjects.MachinesByMachineSet[machineSet.Name] {
				// MachineDeployment MachineSet Machine.metadata
				g.Expect(machine.Labels).To(BeEquivalentTo(
					union(
						map[string]string{
							clusterv1.ClusterNameLabel:                          cluster.Name,
							clusterv1.ClusterTopologyOwnedLabel:                 "",
							clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
							clusterv1.MachineDeploymentNameLabel:                machineDeployment.Name,
							clusterv1.MachineSetNameLabel:                       machineSet.Name,
							clusterv1.MachineDeploymentUniqueLabel:              machineTemplateHash,
						},
						machineSet.Spec.Template.Labels,
					),
				))
				g.Expect(machine.Annotations).To(BeEquivalentTo(
					machineSet.Spec.Template.Annotations,
				))

				// MachineDeployment MachineSet Machine InfrastructureMachine.metadata
				infrastructureMachine := clusterObjects.InfrastructureMachineByMachine[machine.Name]
				g.Expect(infrastructureMachine.GetLabels()).To(BeEquivalentTo(
					union(
						map[string]string{
							clusterv1.ClusterNameLabel:                          cluster.Name,
							clusterv1.ClusterTopologyOwnedLabel:                 "",
							clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
							clusterv1.MachineDeploymentNameLabel:                machineDeployment.Name,
							clusterv1.MachineSetNameLabel:                       machineSet.Name,
							clusterv1.MachineDeploymentUniqueLabel:              machineTemplateHash,
						},
						machineSet.Spec.Template.Labels,
						infrastructureMachineTemplateTemplateMetadata.Labels,
					),
				))
				g.Expect(infrastructureMachine.GetAnnotations()).To(BeEquivalentTo(
					union(
						map[string]string{
							clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(&machineSet.Spec.Template.Spec.InfrastructureRef),
							clusterv1.TemplateClonedFromNameAnnotation:      machineSet.Spec.Template.Spec.InfrastructureRef.Name,
						},
						machineSet.Spec.Template.Annotations,
						infrastructureMachineTemplateTemplateMetadata.Annotations,
					),
				))

				// MachineDeployment MachineSet Machine BootstrapConfig.metadata
				bootstrapConfig := clusterObjects.BootstrapConfigByMachine[machine.Name]
				g.Expect(bootstrapConfig.GetLabels()).To(BeEquivalentTo(
					union(
						map[string]string{
							clusterv1.ClusterNameLabel:                          cluster.Name,
							clusterv1.ClusterTopologyOwnedLabel:                 "",
							clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
							clusterv1.MachineDeploymentNameLabel:                machineDeployment.Name,
							clusterv1.MachineSetNameLabel:                       machineSet.Name,
							clusterv1.MachineDeploymentUniqueLabel:              machineTemplateHash,
						},
						machineSet.Spec.Template.Labels,
						bootstrapConfigTemplateTemplateMetadata.Labels,
					),
				))
				g.Expect(bootstrapConfig.GetAnnotations()).To(BeEquivalentTo(
					union(
						map[string]string{
							clusterv1.TemplateClonedFromGroupKindAnnotation: groupKind(machineSet.Spec.Template.Spec.Bootstrap.ConfigRef),
							clusterv1.TemplateClonedFromNameAnnotation:      machineSet.Spec.Template.Spec.Bootstrap.ConfigRef.Name,
						},
						machineSet.Spec.Template.Annotations,
						bootstrapConfigTemplateTemplateMetadata.Annotations,
					),
				))

				// MachineDeployment MachineSet Machine Node.metadata
				node := clusterObjects.NodesByMachine[machine.Name]
				for k, v := range getManagedLabels(machine.Labels) {
					g.Expect(node.GetLabels()).To(HaveKeyWithValue(k, v))
				}
			}
		}
	}
}

func mustMetadata(metadata *clusterv1.ObjectMeta, err error) *clusterv1.ObjectMeta {
	if err != nil {
		panic(err)
	}
	return metadata
}

// getMachinesByCluster gets the Machines of a Cluster and returns them as a Set of Machine names.
func getMachinesByCluster(ctx context.Context, client client.Client, cluster *clusterv1.Cluster) sets.Set[string] {
	machines := sets.Set[string]{}
	machinesByCluster := framework.GetMachinesByCluster(ctx, framework.GetMachinesByClusterInput{
		Lister:      client,
		ClusterName: cluster.Name,
		Namespace:   cluster.Namespace,
	})
	for _, m := range machinesByCluster {
		machines.Insert(m.Name)
	}
	return machines
}

// getMDClass looks up the MachineDeploymentClass for a md in the ClusterClass.
func getMDClass(cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass, md *clusterv1.MachineDeployment) *clusterv1.MachineDeploymentClass {
	mdTopology := getMDTopology(cluster, md)

	for _, mdClass := range clusterClass.Spec.Workers.MachineDeployments {
		if mdClass.Class == mdTopology.Class {
			return &mdClass
		}
	}
	Fail(fmt.Sprintf("could not find MachineDeployment class %q", mdTopology.Class))
	return nil
}

// getMDTopology looks up the MachineDeploymentTopology for a md in the Cluster.
func getMDTopology(cluster *clusterv1.Cluster, md *clusterv1.MachineDeployment) *clusterv1.MachineDeploymentTopology {
	for _, mdTopology := range cluster.Spec.Topology.Workers.MachineDeployments {
		if mdTopology.Name == md.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel] {
			return &mdTopology
		}
	}
	Fail(fmt.Sprintf("could not find MachineDeployment topology %q", md.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]))
	return nil
}

// groupKind returns the GroupKind string of a ref.
func groupKind(ref *corev1.ObjectReference) string {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	Expect(err).ToNot(HaveOccurred())

	gk := metav1.GroupKind{
		Group: gv.Group,
		Kind:  ref.Kind,
	}
	return gk.String()
}

type unionMap map[string]string

// union merges maps.
// NOTE: In case a key exists in multiple maps, the value of the first map is preserved.
func union(maps ...map[string]string) unionMap {
	res := make(map[string]string)

	for i := len(maps) - 1; i >= 0; i-- {
		for k, v := range maps[i] {
			res[k] = v
		}
	}

	return res
}

// without removes keys from a unionMap.
// Note: This allows ignoring specific keys while comparing maps.
func (m unionMap) without(g Gomega, keys ...string) unionMap {
	for _, key := range keys {
		// Expect key to exist in the map to ensure without is only used for keys that actually exist.
		_, ok := m[key]
		g.Expect(ok).To(BeTrue(), fmt.Sprintf("key %q does not exist in map %s", key, m))
		delete(m, key)
	}
	return m
}

// getManagedLabels gets a map[string]string and returns another map[string]string
// filtering out labels not managed by CAPI.
// Note: This is replicated from machine_controller_noderef.go.
// TODO: Consider moving this func to an internal util or the API package.
func getManagedLabels(labels map[string]string) map[string]string {
	managedLabels := make(map[string]string)
	for key, value := range labels {
		dnsSubdomainOrName := strings.Split(key, "/")[0]
		if dnsSubdomainOrName == clusterv1.NodeRoleLabelPrefix {
			managedLabels[key] = value
		}
		if dnsSubdomainOrName == clusterv1.NodeRestrictionLabelDomain || strings.HasSuffix(dnsSubdomainOrName, "."+clusterv1.NodeRestrictionLabelDomain) {
			managedLabels[key] = value
		}
		if dnsSubdomainOrName == clusterv1.ManagedNodeLabelDomain || strings.HasSuffix(dnsSubdomainOrName, "."+clusterv1.ManagedNodeLabelDomain) {
			managedLabels[key] = value
		}
	}

	return managedLabels
}

type clusterClassObjects struct {
	InfrastructureClusterTemplate             *unstructured.Unstructured
	ControlPlaneTemplate                      *unstructured.Unstructured
	ControlPlaneInfrastructureMachineTemplate *unstructured.Unstructured

	InfrastructureMachineTemplateByMachineDeploymentClass map[string]*unstructured.Unstructured
	BootstrapConfigTemplateByMachineDeploymentClass       map[string]*unstructured.Unstructured
}

// getClusterClassObjects retrieves objects from the ClusterClass.
func getClusterClassObjects(ctx context.Context, g Gomega, clusterProxy framework.ClusterProxy, clusterClass *clusterv1.ClusterClass) clusterClassObjects {
	mgmtClient := clusterProxy.GetClient()

	res := clusterClassObjects{
		InfrastructureMachineTemplateByMachineDeploymentClass: map[string]*unstructured.Unstructured{},
		BootstrapConfigTemplateByMachineDeploymentClass:       map[string]*unstructured.Unstructured{},
	}
	var err error

	res.InfrastructureClusterTemplate, err = external.Get(ctx, mgmtClient, clusterClass.Spec.Infrastructure.Ref, clusterClass.Namespace)
	g.Expect(err).ToNot(HaveOccurred())

	res.ControlPlaneTemplate, err = external.Get(ctx, mgmtClient, clusterClass.Spec.ControlPlane.Ref, clusterClass.Namespace)
	g.Expect(err).ToNot(HaveOccurred())

	res.ControlPlaneInfrastructureMachineTemplate, err = external.Get(ctx, mgmtClient, clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref, clusterClass.Namespace)
	g.Expect(err).ToNot(HaveOccurred())

	for _, mdClass := range clusterClass.Spec.Workers.MachineDeployments {
		infrastructureMachineTemplate, err := external.Get(ctx, mgmtClient, mdClass.Template.Infrastructure.Ref, clusterClass.Namespace)
		g.Expect(err).ToNot(HaveOccurred())
		res.InfrastructureMachineTemplateByMachineDeploymentClass[mdClass.Class] = infrastructureMachineTemplate

		bootstrapConfigTemplate, err := external.Get(ctx, mgmtClient, mdClass.Template.Bootstrap.Ref, clusterClass.Namespace)
		g.Expect(err).ToNot(HaveOccurred())
		res.BootstrapConfigTemplateByMachineDeploymentClass[mdClass.Class] = bootstrapConfigTemplate
	}

	return res
}

type clusterObjects struct {
	InfrastructureCluster *unstructured.Unstructured

	ControlPlane                              *unstructured.Unstructured
	ControlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	ControlPlaneMachines                      []*clusterv1.Machine

	MachineDeployments             []*clusterv1.MachineDeployment
	MachineSetsByMachineDeployment map[string][]*clusterv1.MachineSet
	MachinesByMachineSet           map[string][]*clusterv1.Machine
	NodesByMachine                 map[string]*corev1.Node

	InfrastructureMachineTemplateByMachineDeployment map[string]*unstructured.Unstructured
	BootstrapConfigTemplateByMachineDeployment       map[string]*unstructured.Unstructured

	InfrastructureMachineByMachine map[string]*unstructured.Unstructured
	BootstrapConfigByMachine       map[string]*unstructured.Unstructured
}

// getClusterObjects retrieves objects from the Cluster topology.
func getClusterObjects(ctx context.Context, g Gomega, clusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster) clusterObjects {
	mgmtClient := clusterProxy.GetClient()
	workloadClient := clusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name).GetClient()

	res := clusterObjects{
		MachineSetsByMachineDeployment:                   map[string][]*clusterv1.MachineSet{},
		MachinesByMachineSet:                             map[string][]*clusterv1.Machine{},
		NodesByMachine:                                   map[string]*corev1.Node{},
		BootstrapConfigTemplateByMachineDeployment:       map[string]*unstructured.Unstructured{},
		InfrastructureMachineTemplateByMachineDeployment: map[string]*unstructured.Unstructured{},
		BootstrapConfigByMachine:                         map[string]*unstructured.Unstructured{},
		InfrastructureMachineByMachine:                   map[string]*unstructured.Unstructured{},
	}
	var err error

	// InfrastructureCluster
	res.InfrastructureCluster, err = external.Get(ctx, mgmtClient, cluster.Spec.InfrastructureRef, cluster.Namespace)
	g.Expect(err).ToNot(HaveOccurred())

	// ControlPlane
	res.ControlPlane, err = external.Get(ctx, mgmtClient, cluster.Spec.ControlPlaneRef, cluster.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	controlPlaneInfrastructureMachineTemplateRef, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(res.ControlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	res.ControlPlaneInfrastructureMachineTemplate, err = external.Get(ctx, mgmtClient, controlPlaneInfrastructureMachineTemplateRef, cluster.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	controlPlaneMachineList := &clusterv1.MachineList{}
	g.Expect(mgmtClient.List(ctx, controlPlaneMachineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.MachineControlPlaneLabel: "",
		clusterv1.ClusterNameLabel:         cluster.Name,
	})).To(Succeed())
	// Check all control plane machines already exist.
	replicas, err := contract.ControlPlane().Replicas().Get(res.ControlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(controlPlaneMachineList.Items).To(HaveLen(int(*replicas)))
	for _, machine := range controlPlaneMachineList.Items {
		machine := machine
		res.ControlPlaneMachines = append(res.ControlPlaneMachines, &machine)
		addMachineObjects(ctx, mgmtClient, workloadClient, g, res, cluster, &machine)
	}

	// MachineDeployments.
	for _, mdTopology := range cluster.Spec.Topology.Workers.MachineDeployments {
		// Get MachineDeployment for the current MachineDeploymentTopology.
		mdList := &clusterv1.MachineDeploymentList{}
		g.Expect(mgmtClient.List(ctx, mdList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
		})).To(Succeed())
		g.Expect(mdList.Items).To(HaveLen(1), fmt.Sprintf("expected one MachineDeployment for topology %q, but got %d", mdTopology.Name, len(mdList.Items)))
		md := mdList.Items[0]
		res.MachineDeployments = append(res.MachineDeployments, &md)

		bootstrapConfigTemplate, err := external.Get(ctx, mgmtClient, md.Spec.Template.Spec.Bootstrap.ConfigRef, cluster.Namespace)
		g.Expect(err).ToNot(HaveOccurred())
		res.BootstrapConfigTemplateByMachineDeployment[md.Name] = bootstrapConfigTemplate

		infrastructureMachineTemplate, err := external.Get(ctx, mgmtClient, &md.Spec.Template.Spec.InfrastructureRef, cluster.Namespace)
		g.Expect(err).ToNot(HaveOccurred())
		res.InfrastructureMachineTemplateByMachineDeployment[md.Name] = infrastructureMachineTemplate

		machineSets, err := machineset.GetMachineSetsForDeployment(ctx, mgmtClient, client.ObjectKeyFromObject(&md))
		g.Expect(err).ToNot(HaveOccurred())
		res.MachineSetsByMachineDeployment[md.Name] = machineSets

		machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
			Lister:            mgmtClient,
			ClusterName:       cluster.Name,
			Namespace:         cluster.Namespace,
			MachineDeployment: md,
		})
		// Check all MachineDeployment machines already exist.
		g.Expect(machines).To(HaveLen(int(*md.Spec.Replicas)))
		for _, machine := range machines {
			machine := machine
			res.MachinesByMachineSet[machine.Labels[clusterv1.MachineSetNameLabel]] = append(
				res.MachinesByMachineSet[machine.Labels[clusterv1.MachineSetNameLabel]], &machine)
			addMachineObjects(ctx, mgmtClient, workloadClient, g, res, cluster, &machine)
		}
	}

	return res
}

// addMachineObjects adds objects related to the Machine (BootstrapConfig, InfraMachine, Node) to clusterObjects.
func addMachineObjects(ctx context.Context, mgmtClient, workloadClient client.Client, g Gomega, res clusterObjects, cluster *clusterv1.Cluster, machine *clusterv1.Machine) {
	bootstrapConfig, err := external.Get(ctx, mgmtClient, machine.Spec.Bootstrap.ConfigRef, cluster.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	res.BootstrapConfigByMachine[machine.Name] = bootstrapConfig

	infrastructureMachine, err := external.Get(ctx, mgmtClient, &machine.Spec.InfrastructureRef, cluster.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	res.InfrastructureMachineByMachine[machine.Name] = infrastructureMachine

	g.Expect(machine.Status.NodeRef).ToNot(BeNil())
	node := &corev1.Node{}
	g.Expect(workloadClient.Get(ctx, client.ObjectKey{Namespace: "", Name: machine.Status.NodeRef.Name}, node)).To(Succeed())
	res.NodesByMachine[machine.Name] = node
}

// modifyControlPlaneViaClusterAndWaitInput is the input type for modifyControlPlaneViaClusterAndWait.
type modifyControlPlaneViaClusterAndWaitInput struct {
	ClusterProxy               framework.ClusterProxy
	Cluster                    *clusterv1.Cluster
	ModifyControlPlaneTopology func(topology *clusterv1.ControlPlaneTopology)
	WaitForControlPlane        []interface{}
}

// modifyControlPlaneViaClusterAndWait modifies the ControlPlaneTopology of a Cluster topology via ModifyControlPlaneTopology.
// It then waits until the changes are rolled out to the ControlPlane of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func modifyControlPlaneViaClusterAndWait(ctx context.Context, input modifyControlPlaneViaClusterAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for modifyControlPlaneViaClusterAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling modifyControlPlaneViaClusterAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling modifyControlPlaneViaClusterAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	log.Logf("Modifying the control plane topology of Cluster %s", klog.KObj(input.Cluster))

	// Patch the control plane topology in the Cluster.
	patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
	Expect(err).ToNot(HaveOccurred())
	input.ModifyControlPlaneTopology(&input.Cluster.Spec.Topology.ControlPlane)
	Expect(patchHelper.Patch(ctx, input.Cluster)).To(Succeed())

	// NOTE: We only wait until the change is rolled out to the control plane object and not to the control plane machines.
	log.Logf("Waiting for control plane rollout to complete.")
	Eventually(func(g Gomega) {
		// Get the ControlPlane.
		controlPlaneRef := input.Cluster.Spec.ControlPlaneRef
		controlPlaneTopology := input.Cluster.Spec.Topology.ControlPlane
		controlPlane, err := external.Get(ctx, mgmtClient, controlPlaneRef, input.Cluster.Namespace)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify that the fields from Cluster topology are set on the control plane.
		assertControlPlaneTopologyFields(g, controlPlane, controlPlaneTopology)
	}, input.WaitForControlPlane...).Should(Succeed())
}

// modifyMachineDeploymentViaClusterAndWaitInput is the input type for modifyMachineDeploymentViaClusterAndWait.
type modifyMachineDeploymentViaClusterAndWaitInput struct {
	ClusterProxy                    framework.ClusterProxy
	Cluster                         *clusterv1.Cluster
	ModifyMachineDeploymentTopology func(topology *clusterv1.MachineDeploymentTopology)
	WaitForMachineDeployments       []interface{}
}

// modifyMachineDeploymentViaClusterAndWait modifies the MachineDeploymentTopology of a Cluster topology via ModifyMachineDeploymentTopology.
// It then waits until the changes are rolled out to the MachineDeployments of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func modifyMachineDeploymentViaClusterAndWait(ctx context.Context, input modifyMachineDeploymentViaClusterAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for modifyMachineDeploymentViaClusterAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling modifyMachineDeploymentViaClusterAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling modifyMachineDeploymentViaClusterAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	for i, mdTopology := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
		log.Logf("Modifying the MachineDeployment topology %q of ClusterClass %s", mdTopology.Name, klog.KObj(input.Cluster))

		// Patch the MachineDeployment topology in the Cluster.
		patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		input.ModifyMachineDeploymentTopology(&input.Cluster.Spec.Topology.Workers.MachineDeployments[i])
		Expect(patchHelper.Patch(ctx, input.Cluster)).To(Succeed())

		for _, mdTopology := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
			// NOTE: We only wait until the change is rolled out to the MachineDeployments and not to the worker machines.
			log.Logf("Waiting for MachineDeployment rollout for MachineDeploymentTopology %q to complete.", mdTopology.Name)
			Eventually(func(g Gomega) {
				// Get MachineDeployment for the current MachineDeploymentTopology.
				mdList := &clusterv1.MachineDeploymentList{}
				g.Expect(mgmtClient.List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				})).To(Succeed())
				g.Expect(mdList.Items).To(HaveLen(1), fmt.Sprintf("expected one MachineDeployment for topology %q, but got %d", mdTopology.Name, len(mdList.Items)))
				md := mdList.Items[0]

				// Verify that the fields from Cluster topology are set on the MachineDeployment.
				assertMachineDeploymentTopologyFields(g, md, mdTopology)
			}, input.WaitForMachineDeployments...).Should(BeNil())
		}
	}
}
