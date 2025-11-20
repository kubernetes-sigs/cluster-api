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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ClusterClassChangesSpecInput is the input for ClusterClassChangesSpec.
type ClusterClassChangesSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string
	// Flavor is the cluster-template flavor used to create the Cluster for testing.
	// NOTE: The template must be using a ClusterClass.
	Flavor string

	// ModifyControlPlaneFields are the ControlPlane fields which will be set on the
	// ControlPlaneTemplate of the ClusterClass after the initial Cluster creation.
	// The test verifies that these fields are rolled out to the ControlPlane.
	// NOTE: The fields are configured in the following format: (without ".spec.template")
	// map[string]interface{}{
	//   "spec.path.to.field": <value>,
	// }
	ModifyControlPlaneFields map[string]interface{}

	// ModifyMachineDeploymentBootstrapConfigTemplateFields are the fields which will be set on the
	// BootstrapConfigTemplate of all MachineDeploymentClasses of the ClusterClass after the initial Cluster creation.
	// The test verifies that these fields are rolled out to the MachineDeployments.
	// NOTE: The fields are configured in the following format:
	// map[string]interface{}{
	//   "spec.template.spec.path.to.field": <value>,
	// }
	ModifyMachineDeploymentBootstrapConfigTemplateFields map[string]interface{}

	// ModifyMachineDeploymentInfrastructureMachineTemplateFields are the fields which will be set on the
	// InfrastructureMachineTemplate of all MachineDeploymentClasses of the ClusterClass after the initial Cluster creation.
	// The test verifies that these fields are rolled out to the MachineDeployments.
	// NOTE: The fields are configured in the following format:
	// map[string]interface{}{
	//   "spec.template.spec.path.to.field": <value>,
	// }
	ModifyMachineDeploymentInfrastructureMachineTemplateFields map[string]interface{}

	// ModifyMachinePoolBootstrapConfigTemplateFields are the fields which will be set on the
	// BootstrapConfigTemplate of all MachinePoolClasses of the ClusterClass after the initial Cluster creation.
	// The test verifies that these fields are rolled out to the MachinePools.
	// NOTE: The fields are configured in the following format:
	// map[string]interface{}{
	//   "spec.template.spec.path.to.field": <value>,
	// }
	ModifyMachinePoolBootstrapConfigTemplateFields map[string]interface{}

	// ModifyMachinePoolInfrastructureMachinePoolTemplateFields are the fields which will be set on the
	// InfrastructureMachinePoolTemplate of all MachinePoolClasses of the ClusterClass after the initial Cluster creation.
	// The test verifies that these fields are rolled out to the MachinePools.
	// NOTE: The fields are configured in the following format:
	// map[string]interface{}{
	//   "spec.template.spec.path.to.field": <value>,
	// }
	ModifyMachinePoolInfrastructureMachinePoolTemplateFields map[string]interface{}

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// ClusterClassChangesSpec implements a test that verifies that ClusterClass changes are rolled out successfully.
// Thus, the test consists of the following steps:
//   - Deploy Cluster using a ClusterClass and wait until it is fully provisioned.
//   - Modify the ControlPlaneTemplate of the ClusterClass by setting ModifyControlPlaneFields
//     and wait until the change has been rolled out to the ControlPlane of the Cluster.
//   - Modify the BootstrapTemplate of all MachineDeploymentClasses of the ClusterClass by setting
//     ModifyMachineDeploymentBootstrapConfigTemplateFields and wait until the change has been rolled out
//     to the MachineDeployments of the Cluster.
//   - Modify the InfrastructureMachineTemplate of all MachineDeploymentClasses of the ClusterClass by setting
//     ModifyMachineDeploymentInfrastructureMachineTemplateFields and wait until the change has been rolled out
//     to the MachineDeployments of the Cluster.
//   - Rebase the Cluster to a copy of the ClusterClass which has an additional worker label set. Then wait
//     until the change has been rolled out to the MachineDeployments of the Cluster and verify the ControlPlane
//     has not been changed.
//
// NOTE: The ClusterClass can be changed in many ways (as documented in the ClusterClass Operations doc).
// This test verifies a subset of the possible operations and aims to test the most complicated rollouts
// (template changes, label propagation, rebase), everything else will be covered by unit or integration tests.
// NOTE: Changing the ClusterClass or rebasing to another ClusterClass is semantically equivalent from the point of
// view of a Cluster and the Cluster topology reconciler does not handle those cases differently. Thus we have
// indirect test coverage of this from other tests as well.
func ClusterClassChangesSpec(ctx context.Context, inputGetter func() ClusterClassChangesSpecInput) {
	var (
		specName                         = "clusterclass-changes"
		input                            ClusterClassChangesSpecInput
		namespace, clusterClassNamespace *corev1.Namespace
		cancelWatches                    context.CancelFunc
		clusterResources                 *clusterctl.ApplyClusterTemplateAndWaitResult
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveValidVersion(input.E2EConfig.MustGetVariable(KubernetesVersion)))
		Expect(input.ModifyControlPlaneFields).ToNot(BeEmpty(), "Invalid argument. input.ModifyControlPlaneFields can't be empty when calling %s spec", specName)

		// Set up a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterClassNamespace = framework.CreateNamespace(ctx, framework.CreateNamespaceInput{Creator: input.BootstrapClusterProxy.GetClient(), Name: fmt.Sprintf("%s-clusterclass", namespace.Name)}, "40s", "10s")
		Expect(clusterClassNamespace).ToNot(BeNil(), "Failed to create namespace")
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
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersion),
				ControlPlaneMachineCount: ptr.To[int64](1),
				WorkerMachineCount:       ptr.To[int64](1),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		originalClusterClassState := clusterResources.ClusterClass.DeepCopy()

		By("Rebasing the Cluster to a ClusterClass with a modified label for MachineDeployments and wait for changes to be applied to the MachineDeployment objects")
		rebasedClusterClass := rebaseClusterClassAndWait(ctx, rebaseClusterClassAndWaitInput{
			ClusterProxy:                 input.BootstrapClusterProxy,
			ClusterClass:                 clusterResources.ClusterClass,
			Cluster:                      clusterResources.Cluster,
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		By("Modifying the control plane configuration in ClusterClass and wait for changes to be applied to the control plane object")
		modifyControlPlaneViaClusterClassAndWait(ctx, modifyClusterClassControlPlaneAndWaitInput{
			ClusterProxy:             input.BootstrapClusterProxy,
			ClusterClass:             rebasedClusterClass,
			Cluster:                  clusterResources.Cluster,
			ModifyControlPlaneFields: input.ModifyControlPlaneFields,
			WaitForControlPlane:      input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		By("Modifying the MachineDeployment configuration in ClusterClass and wait for changes to be applied to the MachineDeployment objects")
		modifyMachineDeploymentViaClusterClassAndWait(ctx, modifyMachineDeploymentViaClusterClassAndWaitInput{
			ClusterProxy:                        input.BootstrapClusterProxy,
			ClusterClass:                        rebasedClusterClass,
			Cluster:                             clusterResources.Cluster,
			ModifyBootstrapConfigTemplateFields: input.ModifyMachineDeploymentBootstrapConfigTemplateFields,
			ModifyInfrastructureMachineTemplateFields: input.ModifyMachineDeploymentInfrastructureMachineTemplateFields,
			WaitForMachineDeployments:                 input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Modifying the MachinePool configuration in ClusterClass and wait for changes to be applied to the MachinePool objects")
		modifyMachinePoolViaClusterClassAndWait(ctx, modifyMachinePoolViaClusterClassAndWaitInput{
			ClusterProxy:                        input.BootstrapClusterProxy,
			ClusterClass:                        rebasedClusterClass,
			Cluster:                             clusterResources.Cluster,
			ModifyBootstrapConfigTemplateFields: input.ModifyMachinePoolBootstrapConfigTemplateFields,
			ModifyInfrastructureMachinePoolTemplateFields: input.ModifyMachinePoolInfrastructureMachinePoolTemplateFields,
			WaitForMachinePools:                           input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		})

		By("Rebasing the Cluster to a copy of original ClusterClass in a different namespace")
		rebasedClusterClass = rebaseClusterClassAndWait(ctx, rebaseClusterClassAndWaitInput{
			ClusterProxy:          input.BootstrapClusterProxy,
			ClusterClassNamespace: clusterClassNamespace.Name,
			ClusterClass:          originalClusterClassState,
			Cluster:               clusterResources.Cluster,
			// This rebase will revert the CP back to the original state. If we modified CP fields via
			// modifyControlPlaneViaClusterClassAndWait this rebase will lead to changes in the CP.
			ControlPlaneChanged:          input.ModifyControlPlaneFields != nil,
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		By("Performing modifications on the referenced ClusterClass templates in a different namespace")

		By("Modifying the control plane configuration in second ClusterClass and wait for changes to be applied to the control plane object")
		modifyControlPlaneViaClusterClassAndWait(ctx, modifyClusterClassControlPlaneAndWaitInput{
			ClusterProxy:             input.BootstrapClusterProxy,
			ClusterClass:             rebasedClusterClass,
			Cluster:                  clusterResources.Cluster,
			ModifyControlPlaneFields: input.ModifyControlPlaneFields,
			WaitForControlPlane:      input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		By("Modifying the MachineDeployment configuration in ClusterClass and wait for changes to be applied to the MachineDeployment objects")
		modifyMachineDeploymentViaClusterClassAndWait(ctx, modifyMachineDeploymentViaClusterClassAndWaitInput{
			ClusterProxy:                        input.BootstrapClusterProxy,
			ClusterClass:                        rebasedClusterClass,
			Cluster:                             clusterResources.Cluster,
			ModifyBootstrapConfigTemplateFields: input.ModifyMachineDeploymentBootstrapConfigTemplateFields,
			ModifyInfrastructureMachineTemplateFields: input.ModifyMachineDeploymentInfrastructureMachineTemplateFields,
			WaitForMachineDeployments:                 input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Modifying the MachinePool configuration in ClusterClass and wait for changes to be applied to the MachinePool objects")
		modifyMachinePoolViaClusterClassAndWait(ctx, modifyMachinePoolViaClusterClassAndWaitInput{
			ClusterProxy:                        input.BootstrapClusterProxy,
			ClusterClass:                        rebasedClusterClass,
			Cluster:                             clusterResources.Cluster,
			ModifyBootstrapConfigTemplateFields: input.ModifyMachinePoolBootstrapConfigTemplateFields,
			ModifyInfrastructureMachinePoolTemplateFields: input.ModifyMachinePoolInfrastructureMachinePoolTemplateFields,
			WaitForMachinePools:                           input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		})

		Byf("Wait for all Machine Ready conditions are true")
		framework.WaitForMachinesReady(ctx, framework.WaitForMachinesReadyInput{
			Lister:    input.BootstrapClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
			Intervals: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
		})

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    input.BootstrapClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		By("Deleting a MachineDeploymentTopology in the Cluster Topology and wait for associated MachineDeployment to be deleted")
		deleteMachineDeploymentTopologyAndWait(ctx, deleteMachineDeploymentTopologyAndWaitInput{
			ClusterProxy:              input.BootstrapClusterProxy,
			Cluster:                   clusterResources.Cluster,
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})
		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)

		if !input.SkipCleanup {
			Byf("Deleting namespace used for hosting the %q test spec ClusterClass", specName)
			framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
				Deleter: input.BootstrapClusterProxy.GetClient(),
				Name:    clusterClassNamespace.Name,
			})
		}
	})
}

// modifyClusterClassControlPlaneAndWaitInput is the input type for modifyControlPlaneViaClusterClassAndWait.
type modifyClusterClassControlPlaneAndWaitInput struct {
	ClusterProxy             framework.ClusterProxy
	ClusterClass             *clusterv1.ClusterClass
	Cluster                  *clusterv1.Cluster
	ModifyControlPlaneFields map[string]interface{}
	WaitForControlPlane      []interface{}
}

// modifyControlPlaneViaClusterClassAndWait modifies the ControlPlaneTemplate of a ClusterClass by setting ModifyControlPlaneFields
// and waits until the changes are rolled out to the ControlPlane of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func modifyControlPlaneViaClusterClassAndWait(ctx context.Context, input modifyClusterClassControlPlaneAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for modifyControlPlaneViaClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling modifyControlPlaneViaClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling modifyControlPlaneViaClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling modifyControlPlaneViaClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	log.Logf("Modifying the ControlPlaneTemplate of ClusterClass %s", klog.KObj(input.ClusterClass))

	// Get ControlPlaneTemplate object.
	controlPlaneTemplateRef := &input.ClusterClass.Spec.ControlPlane.TemplateRef
	controlPlaneTemplate, err := external.Get(ctx, mgmtClient, controlPlaneTemplateRef.ToObjectReference(input.ClusterClass.Namespace))
	Expect(err).ToNot(HaveOccurred())

	// Create a new ControlPlaneTemplate object with a new name and ModifyControlPlaneFields set.
	newControlPlaneTemplate := controlPlaneTemplate.DeepCopy()
	newControlPlaneTemplateName := fmt.Sprintf("%s-%s", controlPlaneTemplateRef.Name, util.RandomString(6))
	newControlPlaneTemplate.SetName(newControlPlaneTemplateName)
	newControlPlaneTemplate.SetResourceVersion("")
	for fieldPath, value := range input.ModifyControlPlaneFields {
		templateFieldPath := append([]string{"spec", "template"}, strings.Split(fieldPath, ".")...)
		Expect(unstructured.SetNestedField(newControlPlaneTemplate.Object, value, templateFieldPath...)).To(Succeed())
	}
	Expect(mgmtClient.Create(ctx, newControlPlaneTemplate)).To(Succeed())

	// Patch the ClusterClass ControlPlaneTemplate ref to reference the new ControlPlaneTemplate.
	patchHelper, err := patch.NewHelper(input.ClusterClass, mgmtClient)
	Expect(err).ToNot(HaveOccurred())
	controlPlaneTemplateRef.Name = newControlPlaneTemplateName
	Expect(patchHelper.Patch(ctx, input.ClusterClass)).To(Succeed())

	// NOTE: We only wait until the change is rolled out to the control plane object and not to the control plane machines
	// to speed up the test and focus the test on the ClusterClass feature.
	log.Logf("Waiting for ControlPlane rollout to complete.")
	Eventually(func(g Gomega) error {
		// Get the ControlPlane.
		controlPlaneRef := input.Cluster.Spec.ControlPlaneRef
		controlPlaneTopology := input.Cluster.Spec.Topology.ControlPlane
		controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, controlPlaneRef, input.Cluster.Namespace)
		g.Expect(err).ToNot(HaveOccurred())

		contractVersion, err := contract.GetContractVersion(ctx, mgmtClient, controlPlane.GroupVersionKind().GroupKind())
		g.Expect(err).ToNot(HaveOccurred())

		// Verify that the fields from Cluster topology are set on the control plane.
		assertControlPlaneTopologyFields(g, contractVersion, controlPlane, controlPlaneTopology)

		// Verify that ModifyControlPlaneFields have been set.
		for fieldPath, expectedValue := range input.ModifyControlPlaneFields {
			currentValue, ok, err := unstructured.NestedFieldNoCopy(controlPlane.Object, strings.Split(fieldPath, ".")...)
			g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get field %q", fieldPath))
			g.Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get field %q", fieldPath))
			g.Expect(currentValue).To(Equal(expectedValue), fmt.Sprintf("field %q should be equal", fieldPath))
		}

		// Ensure KCP recognized the change and finished scaling.
		// Note: This is to ensure to not hit https://github.com/kubernetes-sigs/cluster-api/issues/11772
		observedGeneration, ok, err := unstructured.NestedInt64(controlPlane.Object, "status", "observedGeneration")
		g.Expect(err).ToNot(HaveOccurred())
		if ok {
			g.Expect(controlPlane.GetGeneration()).To(BeComparableTo(observedGeneration))
		}
		scaling, err := contract.ControlPlane().IsScaling(controlPlane, contractVersion)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(scaling).To(BeFalse())

		return nil
	}, input.WaitForControlPlane...).Should(Succeed())
}

// assertControlPlaneTopologyFields asserts that all fields set in the ControlPlaneTopology have been set on the ControlPlane.
// Note: We intentionally focus on the fields set in the ControlPlaneTopology and ignore the ones set through ClusterClass or
// ControlPlane template as we want to validate that the fields of the ControlPlaneTopology have been propagated correctly.
func assertControlPlaneTopologyFields(g Gomega, contractVersion string, controlPlane *unstructured.Unstructured, controlPlaneTopology clusterv1.ControlPlaneTopology) {
	metadata, err := contract.ControlPlane().MachineTemplate().Metadata().Get(controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
	for k, v := range controlPlaneTopology.Metadata.Labels {
		g.Expect(metadata.Labels).To(HaveKeyWithValue(k, v))
	}
	for k, v := range controlPlaneTopology.Metadata.Annotations {
		g.Expect(metadata.Annotations).To(HaveKeyWithValue(k, v))
	}

	if controlPlaneTopology.Deletion.NodeDrainTimeoutSeconds != nil {
		if contractVersion == "v1beta1" {
			nodeDrainTimeout, err := contract.ControlPlane().MachineTemplate().NodeDrainTimeout().Get(controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*nodeDrainTimeout).To(Equal(metav1.Duration{Duration: time.Duration(*controlPlaneTopology.Deletion.NodeDrainTimeoutSeconds) * time.Second}))
		} else {
			nodeDrainTimeout, err := contract.ControlPlane().MachineTemplate().NodeDrainTimeoutSeconds().Get(controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*nodeDrainTimeout).To(Equal(*controlPlaneTopology.Deletion.NodeDrainTimeoutSeconds))
		}
	}

	if controlPlaneTopology.Deletion.NodeDeletionTimeoutSeconds != nil {
		if contractVersion == "v1beta1" {
			nodeDeletionTimeout, err := contract.ControlPlane().MachineTemplate().NodeDeletionTimeout().Get(controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*nodeDeletionTimeout).To(Equal(metav1.Duration{Duration: time.Duration(*controlPlaneTopology.Deletion.NodeDeletionTimeoutSeconds) * time.Second}))
		} else {
			nodeDeletionTimeout, err := contract.ControlPlane().MachineTemplate().NodeDeletionTimeoutSeconds().Get(controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*nodeDeletionTimeout).To(Equal(*controlPlaneTopology.Deletion.NodeDeletionTimeoutSeconds))
		}
	}

	if controlPlaneTopology.Deletion.NodeVolumeDetachTimeoutSeconds != nil {
		if contractVersion == "v1beta1" {
			nodeVolumeDetachTimeout, err := contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Get(controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*nodeVolumeDetachTimeout).To(Equal(metav1.Duration{Duration: time.Duration(*controlPlaneTopology.Deletion.NodeVolumeDetachTimeoutSeconds) * time.Second}))
		} else {
			nodeVolumeDetachTimeout, err := contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeoutSeconds().Get(controlPlane)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(*nodeVolumeDetachTimeout).To(Equal(*controlPlaneTopology.Deletion.NodeVolumeDetachTimeoutSeconds))
		}
	}
}

// modifyMachineDeploymentViaClusterClassAndWaitInput is the input type for modifyMachineDeploymentViaClusterClassAndWait.
type modifyMachineDeploymentViaClusterClassAndWaitInput struct {
	ClusterProxy                              framework.ClusterProxy
	ClusterClass                              *clusterv1.ClusterClass
	Cluster                                   *clusterv1.Cluster
	ModifyBootstrapConfigTemplateFields       map[string]interface{}
	ModifyInfrastructureMachineTemplateFields map[string]interface{}
	WaitForMachineDeployments                 []interface{}
}

// modifyMachineDeploymentViaClusterClassAndWait modifies the BootstrapConfigTemplate of MachineDeploymentClasses of a ClusterClass
// by setting ModifyBootstrapConfigTemplateFields and waits until the changes are rolled out to the MachineDeployments of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func modifyMachineDeploymentViaClusterClassAndWait(ctx context.Context, input modifyMachineDeploymentViaClusterClassAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for modifyMachineDeploymentViaClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling modifyMachineDeploymentViaClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling modifyMachineDeploymentViaClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling modifyMachineDeploymentViaClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	for i, mdClass := range input.ClusterClass.Spec.Workers.MachineDeployments {
		// Only try to modify the BootstrapConfigTemplate if the MachineDeploymentClass is using a BootstrapConfigTemplate.
		var newBootstrapConfigTemplateName string
		log.Logf("Modifying the BootstrapConfigTemplate of MachineDeploymentClass %q of ClusterClass %s", mdClass.Class, klog.KObj(input.ClusterClass))

		// Retrieve BootstrapConfigTemplate object.
		bootstrapConfigTemplateRef := mdClass.Bootstrap.TemplateRef.ToObjectReference(input.ClusterClass.Namespace)
		bootstrapConfigTemplate, err := external.Get(ctx, mgmtClient, bootstrapConfigTemplateRef)
		Expect(err).ToNot(HaveOccurred())
		// Create a new BootstrapConfigTemplate object with a new name and ModifyBootstrapConfigTemplateFields set.
		newBootstrapConfigTemplate := bootstrapConfigTemplate.DeepCopy()
		newBootstrapConfigTemplateName = fmt.Sprintf("%s-%s", bootstrapConfigTemplateRef.Name, util.RandomString(6))
		newBootstrapConfigTemplate.SetName(newBootstrapConfigTemplateName)
		newBootstrapConfigTemplate.SetResourceVersion("")
		for fieldPath, value := range input.ModifyBootstrapConfigTemplateFields {
			Expect(unstructured.SetNestedField(newBootstrapConfigTemplate.Object, value, strings.Split(fieldPath, ".")...)).To(Succeed())
		}
		Expect(mgmtClient.Create(ctx, newBootstrapConfigTemplate)).To(Succeed())

		log.Logf("Modifying the InfrastructureMachineTemplate of MachineDeploymentClass %q of ClusterClass %s", mdClass.Class, klog.KObj(input.ClusterClass))

		// Retrieve InfrastructureMachineTemplate object.
		infrastructureMachineTemplateRef := mdClass.Infrastructure.TemplateRef.ToObjectReference(input.ClusterClass.Namespace)
		infrastructureMachineTemplate, err := external.Get(ctx, mgmtClient, infrastructureMachineTemplateRef)
		Expect(err).ToNot(HaveOccurred())
		// Create a new InfrastructureMachineTemplate object with a new name and ModifyInfrastructureMachineTemplateFields set.
		newInfrastructureMachineTemplate := infrastructureMachineTemplate.DeepCopy()
		newInfrastructureMachineTemplateName := fmt.Sprintf("%s-%s", infrastructureMachineTemplateRef.Name, util.RandomString(6))
		newInfrastructureMachineTemplate.SetName(newInfrastructureMachineTemplateName)
		newInfrastructureMachineTemplate.SetResourceVersion("")
		for fieldPath, value := range input.ModifyInfrastructureMachineTemplateFields {
			Expect(unstructured.SetNestedField(newInfrastructureMachineTemplate.Object, value, strings.Split(fieldPath, ".")...)).To(Succeed())
		}
		Expect(mgmtClient.Create(ctx, newInfrastructureMachineTemplate)).To(Succeed())

		// Patch the refs of the MachineDeploymentClass to reference the new templates.
		patchHelper, err := patch.NewHelper(input.ClusterClass, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		input.ClusterClass.Spec.Workers.MachineDeployments[i].Bootstrap.TemplateRef.Name = newBootstrapConfigTemplateName
		input.ClusterClass.Spec.Workers.MachineDeployments[i].Infrastructure.TemplateRef.Name = newInfrastructureMachineTemplateName
		Expect(patchHelper.Patch(ctx, input.ClusterClass)).To(Succeed())

		log.Logf("Waiting for MachineDeployment rollout for MachineDeploymentClass %q to complete.", mdClass.Class)
		for _, mdTopology := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
			// Continue if the MachineDeploymentTopology belongs to another MachineDeploymentClass.
			if mdTopology.Class != mdClass.Class {
				continue
			}

			// NOTE: We only wait until the change is rolled out to the MachineDeployment objects and not to the worker machines
			// to speed up the test and focus the test on the ClusterClass feature.
			log.Logf("Waiting for MachineDeployment rollout for MachineDeploymentTopology %q (class %q) to complete.", mdTopology.Name, mdTopology.Class)
			Eventually(func(g Gomega) error {
				// Get MachineDeployment for the current MachineDeploymentTopology.
				mdList := &clusterv1.MachineDeploymentList{}
				g.Expect(mgmtClient.List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
				})).To(Succeed())
				g.Expect(mdList.Items).To(HaveLen(1), fmt.Sprintf("expected one MachineDeployment for topology %q, but got %d", mdTopology.Name, len(mdList.Items)))
				md := mdList.Items[0]

				// Verify that the fields from Cluster topology are set on the MachineDeployment.
				assertMachineDeploymentTopologyFields(g, md, mdTopology)

				// Get the corresponding BootstrapConfigTemplate.
				bootstrapConfigTemplateRef := md.Spec.Template.Spec.Bootstrap.ConfigRef
				bootstrapConfigTemplate, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, bootstrapConfigTemplateRef, md.Namespace)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that ModifyBootstrapConfigTemplateFields have been set.
				for fieldPath, expectedValue := range input.ModifyBootstrapConfigTemplateFields {
					currentValue, ok, err := unstructured.NestedFieldNoCopy(bootstrapConfigTemplate.Object, strings.Split(fieldPath, ".")...)
					g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(currentValue).To(Equal(expectedValue), fmt.Sprintf("field %q should be equal", fieldPath))
				}

				// Get the corresponding InfrastructureMachineTemplate.
				infrastructureMachineTemplateRef := md.Spec.Template.Spec.InfrastructureRef
				infrastructureMachineTemplate, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, infrastructureMachineTemplateRef, md.Namespace)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that ModifyInfrastructureMachineTemplateFields have been set.
				for fieldPath, expectedValue := range input.ModifyInfrastructureMachineTemplateFields {
					currentValue, ok, err := unstructured.NestedFieldNoCopy(infrastructureMachineTemplate.Object, strings.Split(fieldPath, ".")...)
					g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(currentValue).To(Equal(expectedValue), fmt.Sprintf("field %q should be equal", fieldPath))
				}
				return nil
			}, input.WaitForMachineDeployments...).Should(Succeed())
		}
	}
}

// modifyMachinePoolViaClusterClassAndWaitInput is the input type for modifyMachinePoolViaClusterClassAndWait.
type modifyMachinePoolViaClusterClassAndWaitInput struct {
	ClusterProxy                                  framework.ClusterProxy
	ClusterClass                                  *clusterv1.ClusterClass
	Cluster                                       *clusterv1.Cluster
	ModifyBootstrapConfigTemplateFields           map[string]interface{}
	ModifyInfrastructureMachinePoolTemplateFields map[string]interface{}
	WaitForMachinePools                           []interface{}
}

// modifyMachinePoolViaClusterClassAndWait modifies the BootstrapConfigTemplate of MachinePoolClasses of a ClusterClass
// by setting ModifyBootstrapConfigTemplateFields and waits until the changes are rolled out to the MachinePools of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func modifyMachinePoolViaClusterClassAndWait(ctx context.Context, input modifyMachinePoolViaClusterClassAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for modifyMachinePoolViaClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling modifyMachinePoolViaClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling modifyMachinePoolViaClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling modifyMachinePoolViaClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	for i, mpClass := range input.ClusterClass.Spec.Workers.MachinePools {
		// Only try to modify the BootstrapConfigTemplate if the MachinePoolClass is using a BootstrapConfigTemplate.
		var newBootstrapConfigTemplateName string
		log.Logf("Modifying the BootstrapConfigTemplate of MachinePoolClass %q of ClusterClass %s", mpClass.Class, klog.KObj(input.ClusterClass))

		// Retrieve BootstrapConfigTemplate object.
		bootstrapConfigTemplateRef := mpClass.Bootstrap.TemplateRef.ToObjectReference(input.ClusterClass.Namespace)
		bootstrapConfigTemplate, err := external.Get(ctx, mgmtClient, bootstrapConfigTemplateRef)
		Expect(err).ToNot(HaveOccurred())
		// Create a new BootstrapConfigTemplate object with a new name and ModifyBootstrapConfigTemplateFields set.
		newBootstrapConfigTemplate := bootstrapConfigTemplate.DeepCopy()
		newBootstrapConfigTemplateName = fmt.Sprintf("%s-%s", bootstrapConfigTemplateRef.Name, util.RandomString(6))
		newBootstrapConfigTemplate.SetName(newBootstrapConfigTemplateName)
		newBootstrapConfigTemplate.SetResourceVersion("")
		for fieldPath, value := range input.ModifyBootstrapConfigTemplateFields {
			Expect(unstructured.SetNestedField(newBootstrapConfigTemplate.Object, value, strings.Split(fieldPath, ".")...)).To(Succeed())
		}
		Expect(mgmtClient.Create(ctx, newBootstrapConfigTemplate)).To(Succeed())

		log.Logf("Modifying the InfrastructureMachinePoolTemplate of MachinePoolClass %q of ClusterClass %s", mpClass.Class, klog.KObj(input.ClusterClass))

		// Retrieve InfrastructureMachineTemplate object.
		infrastructureMachinePoolTemplateRef := mpClass.Infrastructure.TemplateRef.ToObjectReference(input.ClusterClass.Namespace)
		infrastructureMachinePoolTemplate, err := external.Get(ctx, mgmtClient, infrastructureMachinePoolTemplateRef)
		Expect(err).ToNot(HaveOccurred())
		// Create a new InfrastructureMachinePoolTemplate object with a new name and ModifyInfrastructureMachinePoolTemplateFields set.
		newInfrastructureMachinePoolTemplate := infrastructureMachinePoolTemplate.DeepCopy()
		newInfrastructureMachinePoolTemplateName := fmt.Sprintf("%s-%s", infrastructureMachinePoolTemplateRef.Name, util.RandomString(6))
		newInfrastructureMachinePoolTemplate.SetName(newInfrastructureMachinePoolTemplateName)
		newInfrastructureMachinePoolTemplate.SetResourceVersion("")
		for fieldPath, value := range input.ModifyInfrastructureMachinePoolTemplateFields {
			Expect(unstructured.SetNestedField(newInfrastructureMachinePoolTemplate.Object, value, strings.Split(fieldPath, ".")...)).To(Succeed())
		}
		Expect(mgmtClient.Create(ctx, newInfrastructureMachinePoolTemplate)).To(Succeed())

		// Patch the refs of the MachinePoolClass to reference the new templates.
		patchHelper, err := patch.NewHelper(input.ClusterClass, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		input.ClusterClass.Spec.Workers.MachinePools[i].Bootstrap.TemplateRef.Name = newBootstrapConfigTemplateName
		input.ClusterClass.Spec.Workers.MachinePools[i].Infrastructure.TemplateRef.Name = newInfrastructureMachinePoolTemplateName
		Expect(patchHelper.Patch(ctx, input.ClusterClass)).To(Succeed())

		log.Logf("Waiting for MachinePool rollout for MachinePoolClass %q to complete.", mpClass.Class)
		for _, mpTopology := range input.Cluster.Spec.Topology.Workers.MachinePools {
			// Continue if the MachinePoolTopology belongs to another MachinePoolClass.
			if mpTopology.Class != mpClass.Class {
				continue
			}

			// NOTE: We only wait until the change is rolled out to the MachinePool objects and not to the worker machines
			// to speed up the test and focus the test on the ClusterClass feature.
			log.Logf("Waiting for MachinePool rollout for MachinePoolTopology %q (class %q) to complete.", mpTopology.Name, mpTopology.Class)
			Eventually(func(g Gomega) error {
				// Get MachinePool for the current MachinePoolTopology.
				mpList := &clusterv1.MachinePoolList{}
				g.Expect(mgmtClient.List(ctx, mpList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
					clusterv1.ClusterTopologyMachinePoolNameLabel: mpTopology.Name,
				})).To(Succeed())
				g.Expect(mpList.Items).To(HaveLen(1), fmt.Sprintf("expected one MachinePool for topology %q, but got %d", mpTopology.Name, len(mpList.Items)))
				mp := mpList.Items[0]

				// Verify that the fields from Cluster topology are set on the MachinePool.
				assertMachinePoolTopologyFields(g, mp, mpTopology)

				// Get the corresponding BootstrapConfig object.
				bootstrapConfigObjectRef := mp.Spec.Template.Spec.Bootstrap.ConfigRef
				bootstrapConfigObject, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, bootstrapConfigObjectRef, mp.Namespace)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that ModifyBootstrapConfigTemplateFields have been set and propagates to the BootstrapConfig.
				for fieldPath, expectedValue := range input.ModifyBootstrapConfigTemplateFields {
					// MachinePools have a BootstrapConfig, not a BootstrapConfigTemplate, so we need to convert the fieldPath so it can find it on the object.
					fieldPath = strings.TrimPrefix(fieldPath, "spec.template.")
					currentValue, ok, err := unstructured.NestedFieldNoCopy(bootstrapConfigObject.Object, strings.Split(fieldPath, ".")...)
					g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(currentValue).To(Equal(expectedValue), fmt.Sprintf("field %q should be equal", fieldPath))
				}

				// Get the corresponding InfrastructureMachinePoolTemplate.
				infrastructureMachinePoolRef := mp.Spec.Template.Spec.InfrastructureRef
				infrastructureMachinePool, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, infrastructureMachinePoolRef, mp.Namespace)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify that ModifyInfrastructureMachinePoolTemplateFields have been set.
				for fieldPath, expectedValue := range input.ModifyInfrastructureMachinePoolTemplateFields {
					fieldPath = strings.TrimPrefix(fieldPath, "spec.template.")
					currentValue, ok, err := unstructured.NestedFieldNoCopy(infrastructureMachinePool.Object, strings.Split(fieldPath, ".")...)
					g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(ok).To(BeTrue(), fmt.Sprintf("failed to get field %q", fieldPath))
					g.Expect(currentValue).To(Equal(expectedValue), fmt.Sprintf("field %q should be equal", fieldPath))
				}
				return nil
			}, input.WaitForMachinePools...).Should(Succeed())
		}
	}
}

// assertMachineDeploymentTopologyFields asserts that all fields set in the MachineDeploymentTopology have been set on the MachineDeployment.
// Note: We intentionally focus on the fields set in the MachineDeploymentTopology and ignore the ones set through ClusterClass
// as we want to validate that the fields of the MachineDeploymentTopology have been propagated correctly.
func assertMachineDeploymentTopologyFields(g Gomega, md clusterv1.MachineDeployment, mdTopology clusterv1.MachineDeploymentTopology) {
	// Note: We only verify that all labels and annotations from the Cluster topology exist to keep it simple here.
	// This is fully covered by the ClusterClass rollout test.
	for k, v := range mdTopology.Metadata.Labels {
		g.Expect(md.Labels).To(HaveKeyWithValue(k, v))
	}
	for k, v := range mdTopology.Metadata.Annotations {
		g.Expect(md.Annotations).To(HaveKeyWithValue(k, v))
	}

	if mdTopology.Deletion.NodeDrainTimeoutSeconds != nil {
		g.Expect(md.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).To(Equal(mdTopology.Deletion.NodeDrainTimeoutSeconds))
	}

	if mdTopology.Deletion.NodeDeletionTimeoutSeconds != nil {
		g.Expect(md.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(mdTopology.Deletion.NodeDeletionTimeoutSeconds))
	}

	if mdTopology.Deletion.NodeVolumeDetachTimeoutSeconds != nil {
		g.Expect(md.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).To(Equal(mdTopology.Deletion.NodeVolumeDetachTimeoutSeconds))
	}

	if mdTopology.MinReadySeconds != nil {
		g.Expect(md.Spec.Template.Spec.MinReadySeconds).To(Equal(mdTopology.MinReadySeconds))
	}

	if !reflect.DeepEqual(mdTopology.Rollout.Strategy, clusterv1.MachineDeploymentTopologyRolloutStrategy{}) {
		g.Expect(md.Spec.Rollout.Strategy.Type).To(BeComparableTo(mdTopology.Rollout.Strategy.Type))
		g.Expect(md.Spec.Rollout.Strategy.RollingUpdate.MaxUnavailable).To(BeComparableTo(mdTopology.Rollout.Strategy.RollingUpdate.MaxUnavailable))
		g.Expect(md.Spec.Rollout.Strategy.RollingUpdate.MaxSurge).To(BeComparableTo(mdTopology.Rollout.Strategy.RollingUpdate.MaxSurge))
	}

	if mdTopology.FailureDomain != "" {
		g.Expect(md.Spec.Template.Spec.FailureDomain).To(Equal(mdTopology.FailureDomain))
	}
}

// assertMachinePoolTopologyFields asserts that all fields set in the MachinePoolTopology have been set on the MachinePool.
// Note: We intentionally focus on the fields set in the MachinePoolTopology and ignore the ones set through ClusterClass
// as we want to validate that the fields of the MachinePoolTopology have been propagated correctly.
func assertMachinePoolTopologyFields(g Gomega, mp clusterv1.MachinePool, mpTopology clusterv1.MachinePoolTopology) {
	// Note: We only verify that all labels and annotations from the Cluster topology exist to keep it simple here.
	// This is fully covered by the ClusterClass rollout test.
	for k, v := range mpTopology.Metadata.Labels {
		g.Expect(mp.Labels).To(HaveKeyWithValue(k, v))
	}
	for k, v := range mpTopology.Metadata.Annotations {
		g.Expect(mp.Annotations).To(HaveKeyWithValue(k, v))
	}

	if mpTopology.Deletion.NodeDrainTimeoutSeconds != nil {
		g.Expect(mp.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).To(Equal(mpTopology.Deletion.NodeDrainTimeoutSeconds))
	}

	if mpTopology.Deletion.NodeDeletionTimeoutSeconds != nil {
		g.Expect(mp.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(mpTopology.Deletion.NodeDeletionTimeoutSeconds))
	}

	if mpTopology.Deletion.NodeVolumeDetachTimeoutSeconds != nil {
		g.Expect(mp.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).To(Equal(mpTopology.Deletion.NodeVolumeDetachTimeoutSeconds))
	}

	if mpTopology.MinReadySeconds != nil {
		g.Expect(mp.Spec.Template.Spec.MinReadySeconds).To(Equal(mpTopology.MinReadySeconds))
	}

	if mpTopology.FailureDomains != nil {
		g.Expect(mp.Spec.FailureDomains).NotTo(BeNil())
		g.Expect(mpTopology.FailureDomains).To(Equal(mp.Spec.FailureDomains))
	}
}

// rebaseClusterClassAndWaitInput is the input type for rebaseClusterClassAndWait.
type rebaseClusterClassAndWaitInput struct {
	ClusterProxy                 framework.ClusterProxy
	ClusterClass                 *clusterv1.ClusterClass
	ClusterClassNamespace        string
	Cluster                      *clusterv1.Cluster
	ControlPlaneChanged          bool
	WaitForMachineDeployments    []interface{}
	WaitForControlPlaneIntervals []interface{}
}

// rebaseClusterClassAndWait rebases the cluster to a copy of the ClusterClass with different worker labels
// and waits until the changes are rolled out to the MachineDeployments of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func rebaseClusterClassAndWait(ctx context.Context, input rebaseClusterClassAndWaitInput) *clusterv1.ClusterClass {
	Expect(ctx).NotTo(BeNil(), "ctx is required for RebaseClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling RebaseClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling RebaseClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling RebaseClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	var testWorkerLabelName = "rebase-diff"

	sourceClusterClass := input.ClusterClass.DeepCopy()
	Expect(mgmtClient.Get(ctx, client.ObjectKeyFromObject(sourceClusterClass), sourceClusterClass)).To(Succeed())

	// Create a new ClusterClass with a new name and the new worker label set.
	newClusterClass := sourceClusterClass.DeepCopy()
	newClusterClassName := fmt.Sprintf("%s-%s", input.ClusterClass.Name, util.RandomString(6))
	newClusterClass.SetName(newClusterClassName)
	if input.ClusterClassNamespace != "" {
		newClusterClass.SetNamespace(input.ClusterClassNamespace)
	}
	newClusterClass.SetResourceVersion("")

	for i, mdClass := range newClusterClass.Spec.Workers.MachineDeployments {
		if mdClass.Metadata.Labels == nil {
			mdClass.Metadata.Labels = map[string]string{}
		}
		mdClass.Metadata.Labels[testWorkerLabelName] = mdClass.Class
		newClusterClass.Spec.Workers.MachineDeployments[i] = mdClass
	}

	// Copy ClusterClass templates to the new namespace
	for i, mdClass := range newClusterClass.Spec.Workers.MachineDeployments {
		cloneTemplateAndUpdateRef(ctx, mgmtClient, &mdClass.Infrastructure.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)
		cloneTemplateAndUpdateRef(ctx, mgmtClient, &mdClass.Bootstrap.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)
		newClusterClass.Spec.Workers.MachineDeployments[i] = mdClass
	}

	for i, mpClass := range newClusterClass.Spec.Workers.MachinePools {
		cloneTemplateAndUpdateRef(ctx, mgmtClient, &mpClass.Infrastructure.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)
		cloneTemplateAndUpdateRef(ctx, mgmtClient, &mpClass.Bootstrap.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)
		newClusterClass.Spec.Workers.MachinePools[i] = mpClass
	}

	cloneTemplateAndUpdateRef(ctx, mgmtClient, &newClusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)
	cloneTemplateAndUpdateRef(ctx, mgmtClient, &newClusterClass.Spec.ControlPlane.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)
	cloneTemplateAndUpdateRef(ctx, mgmtClient, &newClusterClass.Spec.Infrastructure.TemplateRef, input.ClusterClass.Namespace, input.ClusterClassNamespace)

	Expect(mgmtClient.Create(ctx, newClusterClass)).To(Succeed())

	// Get the current ControlPlane, we will later verify that it has not changed.
	controlPlaneRef := input.Cluster.Spec.ControlPlaneRef
	beforeControlPlane, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, controlPlaneRef, input.Cluster.Namespace)
	Expect(err).ToNot(HaveOccurred())

	// Rebase the Cluster to the new ClusterClass.
	patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
	Expect(err).ToNot(HaveOccurred())
	input.Cluster.Spec.Topology.ClassRef.Name = newClusterClassName
	input.Cluster.Spec.Topology.ClassRef.Namespace = input.ClusterClassNamespace
	// We have to retry the patch. The ClusterClass was just created so the client cache in the
	// controller/webhook might not be aware of it yet. If the webhook is not aware of the ClusterClass
	// we get a "Cluster ... can't be validated. ClusterClass ... can not be retrieved" error.
	Eventually(func() error {
		return patchHelper.Patch(ctx, input.Cluster)
	}, "1m", "5s").Should(Succeed(), "Failed to patch Cluster")

	log.Logf("Waiting for MachineDeployment rollout to complete.")
	for _, mdTopology := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
		// NOTE: We only wait until the change is rolled out to the MachineDeployment objects and not to the worker machines
		// to speed up the test and focus the test on the ClusterClass feature.
		log.Logf("Waiting for MachineDeployment rollout for MachineDeploymentTopology %q (class %q) to complete.", mdTopology.Name, mdTopology.Class)
		Eventually(func(g Gomega) error {
			// Get MachineDeployment for the current MachineDeploymentTopology.
			mdList := &clusterv1.MachineDeploymentList{}
			g.Expect(mgmtClient.List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
				clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopology.Name,
			})).To(Succeed())
			if len(mdList.Items) != 1 {
				return errors.Errorf("expected one MachineDeployment for topology %q, but got %d", mdTopology.Name, len(mdList.Items))
			}
			md := mdList.Items[0]

			// Verify the label has been set.
			labelValue, ok := md.Spec.Template.Labels[testWorkerLabelName]
			if !ok {
				return errors.Errorf("label %q should be set", testWorkerLabelName)
			}
			if labelValue != mdTopology.Class {
				return errors.Errorf("label %q should be %q, but is %q", testWorkerLabelName, mdTopology.Class, labelValue)
			}

			return nil
		}, input.WaitForMachineDeployments...).Should(Succeed())
	}

	if input.ControlPlaneChanged {
		Eventually(func(g Gomega) {
			controlPlane, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, controlPlaneRef, input.Cluster.Namespace)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(controlPlane.GetGeneration()).ToNot(Equal(beforeControlPlane.GetGeneration()),
				"ControlPlane generation should be incremented during the rebase because ControlPlane expected to be changed.")

			contractVersion, err := contract.GetContractVersion(ctx, mgmtClient, controlPlane.GroupVersionKind().GroupKind())
			g.Expect(err).ToNot(HaveOccurred())

			// Ensure KCP recognized the change and finished scaling.
			// Note: This is to ensure to not hit https://github.com/kubernetes-sigs/cluster-api/issues/11772
			observedGeneration, ok, err := unstructured.NestedInt64(controlPlane.Object, "status", "observedGeneration")
			g.Expect(err).ToNot(HaveOccurred())
			if ok {
				g.Expect(controlPlane.GetGeneration()).To(BeComparableTo(observedGeneration))
			}

			scaling, err := contract.ControlPlane().IsScaling(controlPlane, contractVersion)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(scaling).To(BeFalse())
		}, input.WaitForControlPlaneIntervals...).Should(Succeed())
	} else {
		Consistently(func(g Gomega) {
			// Verify that the ControlPlane has not been changed.
			// NOTE: MachineDeployments are rolled out before the ControlPlane. Thus, we know that the
			// ControlPlane would have been updated by now, if there have been any changes.
			afterControlPlane, err := external.GetObjectFromContractVersionedRef(ctx, mgmtClient, controlPlaneRef, input.Cluster.Namespace)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(afterControlPlane.GetGeneration()).To(Equal(beforeControlPlane.GetGeneration()),
				"ControlPlane generation should not be incremented during the rebase because ControlPlane should not be affected.")
		}, "5s", "100ms").Should(Succeed())
	}

	return newClusterClass
}

// cloneTemplateAndUpdateRef performs required modifications to avoid conflict, and create a copy of the referenced object, updating the ref in-place.
func cloneTemplateAndUpdateRef(ctx context.Context, cl client.Client, templateRef *clusterv1.ClusterClassTemplateReference, currentNamespace, targetNamespace string) {
	templateObject, err := external.Get(ctx, cl, templateRef.ToObjectReference(currentNamespace))
	Expect(err).ToNot(HaveOccurred())
	if targetNamespace != "" {
		templateObject.SetNamespace(targetNamespace)
	} else {
		templateObject.SetGenerateName(templateObject.GetName() + "-")
		templateObject.SetName("")
	}
	templateObject.SetResourceVersion("")
	templateObject.SetOwnerReferences(nil)
	Expect(cl.Create(ctx, templateObject)).To(Succeed())

	*templateRef = clusterv1.ClusterClassTemplateReference{
		Kind:       templateObject.GetKind(),
		Name:       templateObject.GetName(),
		APIVersion: templateObject.GetAPIVersion(),
	}
}

// deleteMachineDeploymentTopologyAndWaitInput is the input type for deleteMachineDeploymentTopologyAndWaitInput.
type deleteMachineDeploymentTopologyAndWaitInput struct {
	ClusterProxy              framework.ClusterProxy
	Cluster                   *clusterv1.Cluster
	WaitForMachineDeployments []interface{}
}

// deleteMachineDeploymentTopologyAndWait deletes a MachineDeploymentTopology from the Cluster and waits until the changes
// are rolled out by ensuring the associated MachineDeployment is correctly deleted.
func deleteMachineDeploymentTopologyAndWait(ctx context.Context, input deleteMachineDeploymentTopologyAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for deleteMachineDeploymentTopologyAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling deleteMachineDeploymentTopologyAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling deleteMachineDeploymentTopologyAndWait")
	Expect(input.Cluster.Spec.Topology.Workers.MachineDeployments).ToNot(BeEmpty(),
		"Invalid Cluster. deleteMachineDeploymentTopologyAndWait requires at least one MachineDeploymentTopology to be defined in the Cluster topology")

	log.Logf("Removing MachineDeploymentTopology from the Cluster Topology.")
	patchHelper, err := patch.NewHelper(input.Cluster, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())

	// Remove the first MachineDeploymentTopology under input.Cluster.Spec.Topology.Workers.MachineDeployments
	mdTopologyToDelete := input.Cluster.Spec.Topology.Workers.MachineDeployments[0]
	input.Cluster.Spec.Topology.Workers.MachineDeployments = input.Cluster.Spec.Topology.Workers.MachineDeployments[1:]
	Expect(patchHelper.Patch(ctx, input.Cluster)).To(Succeed())

	log.Logf("Waiting for MachineDeployment to be deleted.")
	Eventually(func() error {
		// Get MachineDeployment for the current MachineDeploymentTopology.
		mdList := &clusterv1.MachineDeploymentList{}
		Expect(input.ClusterProxy.GetClient().List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopologyToDelete.Name,
		})).To(Succeed())
		if len(mdList.Items) != 0 {
			return errors.Errorf("expected no MachineDeployment for topology %q, but got %d", mdTopologyToDelete.Name, len(mdList.Items))
		}
		return nil
	}, input.WaitForMachineDeployments...).Should(Succeed())
}
