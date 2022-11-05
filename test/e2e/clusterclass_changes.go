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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
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
		specName         = "clusterclass-changes"
		input            ClusterClassChangesSpecInput
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
		Expect(input.ModifyControlPlaneFields).ToNot(BeEmpty(), "Invalid argument. input.ModifyControlPlaneFields can't be empty when calling %s spec", specName)

		// Set up a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should successfully rollout the managed topology upon changes to the ClusterClass", func() {
		By("Creating a workload cluster")
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
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

		By("Modifying the control plane configuration in ClusterClass and wait for changes to be applied to the control plane object")
		modifyControlPlaneViaClusterClassAndWait(ctx, modifyClusterClassControlPlaneAndWaitInput{
			ClusterProxy:             input.BootstrapClusterProxy,
			ClusterClass:             clusterResources.ClusterClass,
			Cluster:                  clusterResources.Cluster,
			ModifyControlPlaneFields: input.ModifyControlPlaneFields,
			WaitForControlPlane:      input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		By("Modifying the MachineDeployment configuration in ClusterClass and wait for changes to be applied to the MachineDeployment objects")
		modifyMachineDeploymentViaClusterClassAndWait(ctx, modifyMachineDeploymentViaClusterClassAndWaitInput{
			ClusterProxy:                        input.BootstrapClusterProxy,
			ClusterClass:                        clusterResources.ClusterClass,
			Cluster:                             clusterResources.Cluster,
			ModifyBootstrapConfigTemplateFields: input.ModifyMachineDeploymentBootstrapConfigTemplateFields,
			ModifyInfrastructureMachineTemplateFields: input.ModifyMachineDeploymentInfrastructureMachineTemplateFields,
			WaitForMachineDeployments:                 input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Rebasing the Cluster to a ClusterClass with a modified label for MachineDeployments and wait for changes to be applied to the MachineDeployment objects")
		rebaseClusterClassAndWait(ctx, rebaseClusterClassAndWaitInput{
			ClusterProxy:              input.BootstrapClusterProxy,
			ClusterClass:              clusterResources.ClusterClass,
			Cluster:                   clusterResources.Cluster,
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
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
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
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
	Expect(ctx).NotTo(BeNil(), "ctx is required for ModifyControlPlaneViaClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ModifyControlPlaneViaClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling ModifyControlPlaneViaClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling ModifyControlPlaneViaClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	log.Logf("Modifying the ControlPlaneTemplate of ClusterClass %s", klog.KObj(input.ClusterClass))

	// Get ControlPlaneTemplate object.
	controlPlaneTemplateRef := input.ClusterClass.Spec.ControlPlane.Ref
	controlPlaneTemplate, err := external.Get(ctx, mgmtClient, controlPlaneTemplateRef, input.ClusterClass.Namespace)
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

	i, j := InterfaceToDuration(input.WaitForControlPlane)
	// NOTE: We only wait until the change is rolled out to the control plane object and not to the control plane machines
	// to speed up the test and focus the test on the ClusterClass feature.
	log.Logf("Waiting for ControlPlane rollout to complete.")
	Eventually(func() error {
		// Get the ControlPlane.
		controlPlaneRef := input.Cluster.Spec.ControlPlaneRef
		controlPlane, err := external.Get(ctx, mgmtClient, controlPlaneRef, input.ClusterClass.Namespace)
		Expect(err).ToNot(HaveOccurred())

		// Verify that ModifyControlPlaneFields have been set.
		for fieldPath, expectedValue := range input.ModifyControlPlaneFields {
			currentValue, ok, err := unstructured.NestedFieldNoCopy(controlPlane.Object, strings.Split(fieldPath, ".")...)
			if err != nil || !ok {
				return errors.Errorf("failed to get field %q", fieldPath)
			}
			if currentValue != expectedValue {
				return errors.Errorf("field %q should be equal to %q, but is %q", fieldPath, expectedValue, currentValue)
			}
		}
		return nil
	}, i, j).Should(BeNil())
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
	Expect(ctx).NotTo(BeNil(), "ctx is required for ModifyMachineDeploymentViaClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling ModifyMachineDeploymentViaClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling ModifyMachineDeploymentViaClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling ModifyMachineDeploymentViaClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	i, j := InterfaceToDuration(input.WaitForMachineDeployments)
	for _, mdClass := range input.ClusterClass.Spec.Workers.MachineDeployments {
		// Continue if the MachineDeploymentClass is not using a BootstrapConfigTemplate.
		if mdClass.Template.Bootstrap.Ref == nil {
			continue
		}

		log.Logf("Modifying the BootstrapConfigTemplate of MachineDeploymentClass %q of ClusterClass %s", mdClass.Class, klog.KObj(input.ClusterClass))

		// Retrieve BootstrapConfigTemplate object.
		bootstrapConfigTemplateRef := mdClass.Template.Bootstrap.Ref
		bootstrapConfigTemplate, err := external.Get(ctx, mgmtClient, bootstrapConfigTemplateRef, input.Cluster.Namespace)
		Expect(err).ToNot(HaveOccurred())
		// Create a new BootstrapConfigTemplate object with a new name and ModifyBootstrapConfigTemplateFields set.
		newBootstrapConfigTemplate := bootstrapConfigTemplate.DeepCopy()
		newBootstrapConfigTemplateName := fmt.Sprintf("%s-%s", bootstrapConfigTemplateRef.Name, util.RandomString(6))
		newBootstrapConfigTemplate.SetName(newBootstrapConfigTemplateName)
		newBootstrapConfigTemplate.SetResourceVersion("")
		for fieldPath, value := range input.ModifyBootstrapConfigTemplateFields {
			Expect(unstructured.SetNestedField(newBootstrapConfigTemplate.Object, value, strings.Split(fieldPath, ".")...)).To(Succeed())
		}
		Expect(mgmtClient.Create(ctx, newBootstrapConfigTemplate)).To(Succeed())

		// Retrieve InfrastructureMachineTemplate object.
		infrastructureMachineTemplateRef := mdClass.Template.Infrastructure.Ref
		infrastructureMachineTemplate, err := external.Get(ctx, mgmtClient, infrastructureMachineTemplateRef, input.Cluster.Namespace)
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

		// Patch the BootstrapConfigTemplate ref of the MachineDeploymentClass to reference the new BootstrapConfigTemplate.
		patchHelper, err := patch.NewHelper(input.ClusterClass, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		bootstrapConfigTemplateRef.Name = newBootstrapConfigTemplateName
		infrastructureMachineTemplateRef.Name = newInfrastructureMachineTemplateName
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
			Eventually(func() error {
				// Get MachineDeployment for the current MachineDeploymentTopology.
				mdList := &clusterv1.MachineDeploymentList{}
				Expect(mgmtClient.List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
					clusterv1.ClusterTopologyMachineDeploymentLabelName: mdTopology.Name,
				})).To(Succeed())
				if len(mdList.Items) != 1 {
					return errors.Errorf("expected one MachineDeployment for topology %q, but got %d", mdTopology.Name, len(mdList.Items))
				}
				md := mdList.Items[0]

				// Get the corresponding BootstrapConfigTemplate.
				bootstrapConfigTemplateRef := md.Spec.Template.Spec.Bootstrap.ConfigRef
				bootstrapConfigTemplate, err := external.Get(ctx, mgmtClient, bootstrapConfigTemplateRef, input.Cluster.Namespace)
				Expect(err).ToNot(HaveOccurred())

				// Verify that ModifyBootstrapConfigTemplateFields have been set.
				for fieldPath, expectedValue := range input.ModifyBootstrapConfigTemplateFields {
					currentValue, ok, err := unstructured.NestedFieldNoCopy(bootstrapConfigTemplate.Object, strings.Split(fieldPath, ".")...)
					if err != nil || !ok {
						return errors.Errorf("failed to get field %q", fieldPath)
					}
					if !reflect.DeepEqual(currentValue, expectedValue) {
						return errors.Errorf("field %q should be equal to %q, but is %q", fieldPath, expectedValue, currentValue)
					}
				}

				// Get the corresponding InfrastructureMachineTemplate.
				infrastructureMachineTemplateRef := md.Spec.Template.Spec.InfrastructureRef
				infrastructureMachineTemplate, err := external.Get(ctx, mgmtClient, &infrastructureMachineTemplateRef, input.Cluster.Namespace)
				Expect(err).ToNot(HaveOccurred())

				// Verify that ModifyInfrastructureMachineTemplateFields have been set.
				for fieldPath, expectedValue := range input.ModifyInfrastructureMachineTemplateFields {
					currentValue, ok, err := unstructured.NestedFieldNoCopy(infrastructureMachineTemplate.Object, strings.Split(fieldPath, ".")...)
					if err != nil || !ok {
						return errors.Errorf("failed to get field %q", fieldPath)
					}
					if !reflect.DeepEqual(currentValue, expectedValue) {
						return errors.Errorf("field %q should be equal to %q, but is %q", fieldPath, expectedValue, currentValue)
					}
				}
				return nil
			}, i, j).Should(BeNil())
		}
	}
}

// rebaseClusterClassAndWaitInput is the input type for rebaseClusterClassAndWait.
type rebaseClusterClassAndWaitInput struct {
	ClusterProxy              framework.ClusterProxy
	ClusterClass              *clusterv1.ClusterClass
	Cluster                   *clusterv1.Cluster
	WaitForMachineDeployments []interface{}
}

// rebaseClusterClassAndWait rebases the cluster to a copy of the ClusterClass with different worker labels
// and waits until the changes are rolled out to the MachineDeployments of the Cluster.
// NOTE: This helper is really specific to this test, so we are keeping this private vs. adding it to the framework.
func rebaseClusterClassAndWait(ctx context.Context, input rebaseClusterClassAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for RebaseClusterClassAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling RebaseClusterClassAndWait")
	Expect(input.ClusterClass).ToNot(BeNil(), "Invalid argument. input.ClusterClass can't be nil when calling RebaseClusterClassAndWait")
	Expect(input.Cluster).ToNot(BeNil(), "Invalid argument. input.Cluster can't be nil when calling RebaseClusterClassAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	var testWorkerLabelName = "rebase-diff"

	// Create a new ClusterClass with a new name and the new worker label set.
	newClusterClass := input.ClusterClass.DeepCopy()
	newClusterClassName := fmt.Sprintf("%s-%s", input.ClusterClass.Name, util.RandomString(6))
	newClusterClass.SetName(newClusterClassName)
	newClusterClass.SetResourceVersion("")
	for i, mdClass := range newClusterClass.Spec.Workers.MachineDeployments {
		if mdClass.Template.Metadata.Labels == nil {
			mdClass.Template.Metadata.Labels = map[string]string{}
		}
		mdClass.Template.Metadata.Labels[testWorkerLabelName] = mdClass.Class
		newClusterClass.Spec.Workers.MachineDeployments[i] = mdClass
	}
	Expect(mgmtClient.Create(ctx, newClusterClass)).To(Succeed())

	// Get the current ControlPlane, we will later verify that it has not changed.
	controlPlaneRef := input.Cluster.Spec.ControlPlaneRef
	beforeControlPlane, err := external.Get(ctx, mgmtClient, controlPlaneRef, input.ClusterClass.Namespace)
	Expect(err).ToNot(HaveOccurred())

	// Rebase the Cluster to the new ClusterClass.
	patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
	Expect(err).ToNot(HaveOccurred())
	input.Cluster.Spec.Topology.Class = newClusterClassName
	Expect(patchHelper.Patch(ctx, input.Cluster)).To(Succeed())

	i, j := InterfaceToDuration(input.WaitForMachineDeployments)
	log.Logf("Waiting for MachineDeployment rollout to complete.")
	for _, mdTopology := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
		// NOTE: We only wait until the change is rolled out to the MachineDeployment objects and not to the worker machines
		// to speed up the test and focus the test on the ClusterClass feature.
		log.Logf("Waiting for MachineDeployment rollout for MachineDeploymentTopology %q (class %q) to complete.", mdTopology.Name, mdTopology.Class)
		Eventually(func() error {
			// Get MachineDeployment for the current MachineDeploymentTopology.
			mdList := &clusterv1.MachineDeploymentList{}
			Expect(mgmtClient.List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
				clusterv1.ClusterTopologyMachineDeploymentLabelName: mdTopology.Name,
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
		}, i, j).Should(BeNil())
	}

	// Verify that the ControlPlane has not been changed.
	// NOTE: MachineDeployments are rolled out before the ControlPlane. Thus, we know that the
	// ControlPlane would have been updated by now, if there have been any changes.
	afterControlPlane, err := external.Get(ctx, mgmtClient, controlPlaneRef, input.ClusterClass.Namespace)
	Expect(err).ToNot(HaveOccurred())
	Expect(afterControlPlane.GetGeneration()).To(Equal(beforeControlPlane.GetGeneration()),
		"ControlPlane generation should not be incremented during the rebase because ControlPlane should not be affected.")
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
	Expect(len(input.Cluster.Spec.Topology.Workers.MachineDeployments)).To(BeNumerically(">", 0),
		"Invalid Cluster. deleteMachineDeploymentTopologyAndWait requires at least one MachineDeploymentTopology to be defined in the Cluster topology")

	log.Logf("Removing MachineDeploymentTopology from the Cluster Topology.")
	patchHelper, err := patch.NewHelper(input.Cluster, input.ClusterProxy.GetClient())
	Expect(err).ToNot(HaveOccurred())

	// Remove the first MachineDeploymentTopology under input.Cluster.Spec.Topology.Workers.MachineDeployments
	mdTopologyToDelete := input.Cluster.Spec.Topology.Workers.MachineDeployments[0]
	input.Cluster.Spec.Topology.Workers.MachineDeployments = input.Cluster.Spec.Topology.Workers.MachineDeployments[1:]
	Expect(patchHelper.Patch(ctx, input.Cluster)).To(Succeed())

	i, j := InterfaceToDuration(input.WaitForMachineDeployments)
	log.Logf("Waiting for MachineDeployment to be deleted.")
	Eventually(func() error {
		// Get MachineDeployment for the current MachineDeploymentTopology.
		mdList := &clusterv1.MachineDeploymentList{}
		Expect(input.ClusterProxy.GetClient().List(ctx, mdList, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels{
			clusterv1.ClusterTopologyMachineDeploymentLabelName: mdTopologyToDelete.Name,
		})).To(Succeed())
		if len(mdList.Items) != 0 {
			return errors.Errorf("expected no MachineDeployment for topology %q, but got %d", mdTopologyToDelete.Name, len(mdList.Items))
		}
		return nil
	}, i, j).Should(BeNil())
}
