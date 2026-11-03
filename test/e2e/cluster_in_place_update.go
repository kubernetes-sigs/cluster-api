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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ClusterInPlaceUpdateSpecInput is the input for ClusterInPlaceUpdateSpec.
type ClusterInPlaceUpdateSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// InfrastructureProvider allows to specify the infrastructure provider to be used when looking for
	// cluster templates.
	// If not set, clusterctl will look at the infrastructure provider installed in the management cluster;
	// if only one infrastructure provider exists, it will be used, otherwise the operation will fail if more than one exists.
	InfrastructureProvider *string

	// Flavor, if specified is the template flavor used to create the cluster for testing.
	// If not specified, the default flavor for the selected infrastructure provider is used.
	Flavor *string

	// WorkerMachineCount defines number of worker machines to be added to the workload cluster.
	// If not specified, 1 will be used.
	WorkerMachineCount *int64

	// ExtensionConfigName is the name of the ExtensionConfig. Defaults to "in-place-update".
	// This value is provided to clusterctl as "EXTENSION_CONFIG_NAME" variable and can be used to template the
	// name of the ExtensionConfig into the ClusterClass.
	ExtensionConfigName string

	// ExtensionServiceNamespace is the namespace where the service for the Runtime Extension is located.
	// Note: This should only be set if a Runtime Extension is used.
	ExtensionServiceNamespace string

	// ExtensionServiceNamespace is the name where the service for the Runtime Extension is located.
	// Note: This should only be set if a Runtime Extension is used.
	ExtensionServiceName string

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// ClusterctlVariables allows injecting variables to the cluster template.
	// If not specified, this is a no-op.
	ClusterctlVariables map[string]string
}

// ClusterInPlaceUpdateSpec implements a test for in-place updates.
// Note: This test works with KCP and MachineDeployments as it tests the KCP and MachineDeployment in-place update features.
func ClusterInPlaceUpdateSpec(ctx context.Context, inputGetter func() ClusterInPlaceUpdateSpecInput) {
	var (
		specName         = "in-place-update"
		input            ClusterInPlaceUpdateSpecInput
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

		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			if input.ExtensionConfigName == "" {
				input.ExtensionConfigName = specName
			}
		}

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)

		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster", func() {
		By("Creating a workload cluster")

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		flavor := clusterctl.DefaultFlavor
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		workerMachineCount := ptr.To[int64](1)
		if input.WorkerMachineCount != nil {
			workerMachineCount = input.WorkerMachineCount
		}

		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))

		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			// NOTE: test extension is already deployed in the management cluster. If for any reason in future we want
			// to make this test more self-contained this test should be modified in order to create an additional
			// management cluster; also the E2E test configuration should be modified introducing something like
			// optional:true allowing to define which providers should not be installed by default in
			// a management cluster.
			By("Deploy Test Extension ExtensionConfig")

			// In this test we are defaulting all handlers to non-blocking because we don't expect the handlers to block the
			// cluster lifecycle by default. Setting defaultAllHandlersToBlocking to false enforces that the test-extension
			// automatically creates the ConfigMap with non-blocking preloaded responses.
			defaultAllHandlersToBlocking := false
			// select on the current namespace
			// This is necessary so in CI this test doesn't influence other tests by enabling lifecycle hooks
			// in other test namespaces.
			namespaces := []string{namespace.Name}
			extensionConfig := extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, false, defaultAllHandlersToBlocking, namespaces...)
			Expect(input.BootstrapClusterProxy.GetClient().Create(ctx,
				extensionConfig)).
				To(Succeed(), "Failed to create the ExtensionConfig")
		}

		variables := map[string]string{
			// This is used to template the name of the ExtensionConfig into the ClusterClass.
			"EXTENSION_CONFIG_NAME": input.ExtensionConfigName,
		}
		maps.Copy(variables, input.ClusterctlVariables)

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:              filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:   input.ClusterctlConfigPath,
				ClusterctlVariables:    variables,
				KubeconfigPath:         input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider: infrastructureProvider,
				Flavor:                 flavor,
				Namespace:              namespace.Name,
				ClusterName:            clusterName,
				KubernetesVersion:      input.E2EConfig.MustGetVariable(KubernetesVersion),
				// ControlPlaneMachineCount is not configurable because it has to be 3 because we want
				// to use scale-in to test in-place updates without any Machine re-creations.
				ControlPlaneMachineCount: ptr.To[int64](3),
				WorkerMachineCount:       workerMachineCount,
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		cluster := clusterResources.Cluster
		mgmtClient := input.BootstrapClusterProxy.GetClient()

		Byf("Verify Cluster is Available and Machines are Ready before starting in-place updates")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    mgmtClient,
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    mgmtClient,
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		var machineObjectsBeforeInPlaceUpdate machineObjects
		Eventually(func(g Gomega) {
			machineObjectsBeforeInPlaceUpdate = getMachineObjects(ctx, g, mgmtClient, cluster)
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		// Doing multiple in-place updates for additional coverage.
		filePath := "/tmp/test"
		for i, fileContent := range []string{
			"first in-place update",
			"second in-place update",
			"third in-place update",
			"fourth in-place update",
			"five in-place update",
		} {
			if i == 0 {
				// In the first iteration the CanUpdateMachine and CanUpdateMachineSet extensions first respond with RetryAfterSeconds
				// to test that KCP and the MachineDeployments wait for the extension's decision.
				Byf("[%d] Configure the CanUpdateMachine and CanUpdateMachineSet extensions to respond with RetryAfterSeconds", i)
				setPreloadedCanUpdateHookResponses(ctx, mgmtClient, cluster, input.ExtensionConfigName,
					`{"Status": "Success", "RetryAfterSeconds": 5, "Message": "decision is in progress"}`)

				Byf("[%d] Trigger a rollout by modifying the files variable", i)
				setFilesVariable(ctx, mgmtClient, cluster, filePath, "blocked update")

				Byf("[%d] Verify KCP is waiting for the decision of the CanUpdateMachine extension", i)
				verifyKCPRollingOutCondition(ctx, mgmtClient, cluster, controlplanev1.KubeadmControlPlaneRollingOutWaitingForRolloutDecisionReason, "decision is in progress", input.E2EConfig.GetIntervals(specName, "wait-control-plane")...)

				Byf("[%d] Verify the MachineDeployments are waiting for the decision of the CanUpdateMachineSet extension", i)
				verifyMachineDeploymentsRollingOutCondition(ctx, mgmtClient, cluster, clusterv1.MachineDeploymentRollingOutWaitingForRolloutDecisionReason, "decision is in progress", input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...)

				// Then the CanUpdateMachine and CanUpdateMachineSet extensions block the rollout by responding with an empty list of allowedOperations,
				// so neither in-place update nor replacing the Machines is allowed.
				// Note: The extensions do not have to be able to handle the change in-place for this.
				// Note: KCP and the MachineDeployments call the extensions again after the RetryAfterSeconds of the previous response expired.
				Byf("[%d] Configure the CanUpdateMachine and CanUpdateMachineSet extensions to block the rollout by responding with an empty list of allowedOperations", i)
				setPreloadedCanUpdateHookResponses(ctx, mgmtClient, cluster, input.ExtensionConfigName,
					`{"Status": "Success", "Message": "rollout is blocked by the extension", "AllowedOperations": []}`)

				Byf("[%d] Verify KCP rollout is blocked by the CanUpdateMachine extension", i)
				verifyKCPRollingOutCondition(ctx, mgmtClient, cluster, controlplanev1.KubeadmControlPlaneRollingOutRolloutBlockedReason, "rollout is blocked by the extension", input.E2EConfig.GetIntervals(specName, "wait-control-plane")...)

				Byf("[%d] Verify the MachineDeployments rollout is blocked by the CanUpdateMachineSet extension", i)
				verifyMachineDeploymentsRollingOutCondition(ctx, mgmtClient, cluster, clusterv1.MachineDeploymentRollingOutRolloutBlockedReason, "rollout is blocked by the extension", input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...)

				Byf("[%d] Verify no Machine is changed while the rollout is blocked", i)
				Consistently(func(g Gomega) {
					machineObjectsWhileBlocked := getMachineObjects(ctx, g, mgmtClient, cluster)
					g.Expect(machineNames(machineObjectsWhileBlocked.ControlPlaneMachines)).To(Equal(machineNames(machineObjectsBeforeInPlaceUpdate.ControlPlaneMachines)))
					g.Expect(machineNames(machineObjectsWhileBlocked.WorkerMachines)).To(Equal(machineNames(machineObjectsBeforeInPlaceUpdate.WorkerMachines)))
				}, 15*time.Second, 3*time.Second).Should(Succeed())

				// Finally the CanUpdateMachine and CanUpdateMachineSet extensions stop blocking the rollout.
				// Note: The blocked decision is cached until the KCP spec / the new MachineSet changes or the cache entry expires.
				// Modifying the files variable below changes the KCP spec and creates a new MachineSet again, which unblocks the rollout.
				Byf("[%d] Configure the CanUpdateMachine and CanUpdateMachineSet extensions to stop blocking the rollout", i)
				setPreloadedCanUpdateHookResponses(ctx, mgmtClient, cluster, input.ExtensionConfigName, "")

				// The UpdateMachine extension responds with the first updateStatus before the in-place update is triggered,
				// the other updateStatus are configured by verifyUpdateMachineUpdateStatuses below.
				Byf("[%d] Configure the UpdateMachine extension to respond with updateStatus %s", i, updateMachineUpdateStatusSteps[0].updateStatus)
				setPreloadedHookResponse(ctx, mgmtClient, cluster, input.ExtensionConfigName, "UpdateMachine", updateMachineUpdateStatusSteps[0].response())
			}

			Byf("[%d] Trigger in-place update by modifying the files variable", i)
			setFilesVariable(ctx, mgmtClient, cluster, filePath, fileContent)

			if i == 0 {
				// For the first control plane Machine of the first in-place update the UpdateMachine extension returns
				// InProgress, Failed, Blocked and CompletedButDirty (in this order) and we verify the conditions of the Machine.
				// Afterward the extension stops returning preloaded responses and the rollout continues as usual.
				// Note: The preloaded responses are not specific to a Machine. If other Machines are updated before the
				// preloaded response is removed, they also complete with updateStatus CompletedButDirty.
				verifyUpdateMachineUpdateStatuses(ctx, mgmtClient, cluster, input.ExtensionConfigName, i, input.E2EConfig.GetIntervals(specName, "wait-control-plane")...)
			}

			var machineObjectsAfterInPlaceUpdate machineObjects
			Eventually(func(g Gomega) {
				// Ensure the in-place update was done.
				if i == 0 {
					// After the first in-place update at least one Machine is dirty (see verifyUpdateMachineUpdateStatuses),
					// so the conditions are still true, but their messages mention the dirty Machines.
					verifyClusterConditionAllowingDirtyMachines(ctx, g, mgmtClient, cluster, clusterv1.ClusterControlPlaneMachinesUpToDateCondition)
					verifyClusterConditionAllowingDirtyMachines(ctx, g, mgmtClient, cluster, clusterv1.ClusterWorkerMachinesUpToDateCondition)
				} else {
					// The dirty state goes away when the next in-place update of a Machine completes with updateStatus Completed.
					// So from the second in-place update on no Machine is dirty anymore, which means the messages of the conditions are empty.
					framework.VerifyClusterCondition(ctx, framework.VerifyClusterConditionInput{
						Getter:        mgmtClient,
						Name:          clusterResources.Cluster.Name,
						Namespace:     clusterResources.Cluster.Namespace,
						ConditionType: clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
					})
					framework.VerifyClusterCondition(ctx, framework.VerifyClusterConditionInput{
						Getter:        mgmtClient,
						Name:          clusterResources.Cluster.Name,
						Namespace:     clusterResources.Cluster.Namespace,
						ConditionType: clusterv1.ClusterWorkerMachinesUpToDateCondition,
					})
				}

				// Ensure only in-place updates were executed and no Machine was re-created.
				machineObjectsAfterInPlaceUpdate = getMachineObjects(ctx, g, mgmtClient, cluster)
				g.Expect(machineNames(machineObjectsAfterInPlaceUpdate.ControlPlaneMachines)).To(Equal(machineNames(machineObjectsBeforeInPlaceUpdate.ControlPlaneMachines)))
				g.Expect(machineNames(machineObjectsAfterInPlaceUpdate.WorkerMachines)).To(Equal(machineNames(machineObjectsBeforeInPlaceUpdate.WorkerMachines)))

				for _, kubeadmConfig := range machineObjectsAfterInPlaceUpdate.KubeadmConfigByMachine {
					g.Expect(kubeadmConfig.Spec.Files).To(ContainElement(HaveField("Path", filePath)))
					g.Expect(kubeadmConfig.Spec.Files).To(ContainElement(HaveField("Content", fileContent)))
				}
			}, input.E2EConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

			// Update machineObjectsBeforeInPlaceUpdate for the next round of in-place update.
			machineObjectsBeforeInPlaceUpdate = machineObjectsAfterInPlaceUpdate
		}

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
		if !input.SkipCleanup {
			if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
				Eventually(func() error {
					return input.BootstrapClusterProxy.GetClient().Delete(ctx, &runtimev1.ExtensionConfig{ObjectMeta: metav1.ObjectMeta{Name: input.ExtensionConfigName}})
				}, 10*time.Second, 1*time.Second).Should(Succeed(), "Deleting ExtensionConfig failed")
			}
		}
	})
}

// updateMachineUpdateStatusStep is a step of verifyUpdateMachineUpdateStatuses.
type updateMachineUpdateStatusStep struct {
	// updateStatus is the updateStatus the UpdateMachine extension returns in this step.
	updateStatus string
	// message is the message the UpdateMachine extension returns in this step.
	message string

	// updatingStatus and updatingReason are the expected Updating condition of the Machine
	// while the extension returns the response of this step.
	updatingStatus metav1.ConditionStatus
	updatingReason string

	// upToDateStatus, upToDateReason and upToDateMessage (substring) are the expected UpToDate condition of the Machine
	// while the extension returns the response of this step.
	upToDateStatus  metav1.ConditionStatus
	upToDateReason  string
	upToDateMessage string
}

// response returns the preloaded response the UpdateMachine extension should return in this step.
func (s updateMachineUpdateStatusStep) response() string {
	return fmt.Sprintf(`{"Status": "Success", "UpdateStatus": %q, "RetryAfterSeconds": 5, "Message": %q}`, s.updateStatus, s.message)
}

// updateMachineUpdateStatusSteps are the steps verifyUpdateMachineUpdateStatuses goes through (in this order).
var updateMachineUpdateStatusSteps = []updateMachineUpdateStatusStep{
	{
		updateStatus:   string(runtimehooksv1.MachineUpdateStatusInProgress),
		message:        "extension is updating the machine",
		updatingStatus: metav1.ConditionTrue,
		updatingReason: clusterv1.MachineInPlaceUpdatingReason,
		upToDateStatus: metav1.ConditionFalse,
		upToDateReason: clusterv1.MachineUpToDateUpdatingReason,
		// The message of the extension is surfaced while the Machine is updating.
		upToDateMessage: "extension is updating the machine",
	},
	{
		updateStatus:   string(runtimehooksv1.MachineUpdateStatusFailed),
		message:        "extension failed to update the machine",
		updatingStatus: metav1.ConditionTrue,
		updatingReason: clusterv1.MachineInPlaceUpdateFailedReason,
		upToDateStatus: metav1.ConditionFalse,
		upToDateReason: clusterv1.MachineUpToDateUpdatingReason,
		// The message of the extension is surfaced while the Machine is updating.
		upToDateMessage: "extension failed to update the machine",
	},
	{
		updateStatus:   string(runtimehooksv1.MachineUpdateStatusBlocked),
		message:        "extension is blocked updating the machine",
		updatingStatus: metav1.ConditionTrue,
		updatingReason: clusterv1.MachineInPlaceUpdateBlockedReason,
		upToDateStatus: metav1.ConditionFalse,
		upToDateReason: clusterv1.MachineUpToDateUpdatingReason,
		// The message of the extension is surfaced while the Machine is updating.
		upToDateMessage: "extension is blocked updating the machine",
	},
	{
		updateStatus: string(runtimehooksv1.MachineUpdateStatusCompletedButDirty),
		message:      "dirty",
		// The in-place update is completed, so the Machine is not updating anymore.
		updatingStatus: metav1.ConditionFalse,
		updatingReason: clusterv1.MachineNotUpdatingReason,
		// A dirty Machine is still up-to-date, so the rollout can continue with the next Machine.
		upToDateStatus:  metav1.ConditionTrue,
		upToDateReason:  clusterv1.MachineUpToDateReason,
		upToDateMessage: "In-place update completed, but the Machine might still differ from the desired state",
	},
}

// verifyUpdateMachineUpdateStatuses goes through the updateMachineUpdateStatusSteps for the first control plane Machine
// which is updated in-place, and verifies the Updating and UpToDate conditions of the Machine in each step.
// Note: The extension is already configured to return the first step (see updateMachineUpdateStatusSteps[0]).
// When the last step is verified, the preloaded UpdateMachine response is removed, so the extension falls back to its
// default behavior (which completes the in-place update with updateStatus Completed) for the remaining Machines.
// Note: The preloaded response is returned for all Machines. Machines which are updated before it is removed (KCP continues
// with the next Machine as soon as a dirty Machine is up-to-date) are also dirty, so the Machine verified here
// is not necessarily the only dirty Machine.
func verifyUpdateMachineUpdateStatuses(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName string, iteration int, intervals ...interface{}) {
	// Wait until the first control plane Machine is updating.
	var machineName string
	Eventually(func(g Gomega) {
		for _, machine := range getMachineObjects(ctx, g, c, cluster).ControlPlaneMachines {
			if conditions.IsTrue(machine, clusterv1.MachineUpdatingCondition) {
				machineName = machine.Name
				return
			}
		}
		g.Expect(machineName).ToNot(BeEmpty(), "no control plane Machine is updating")
	}, intervals...).Should(Succeed())

	for i, step := range updateMachineUpdateStatusSteps {
		if i > 0 {
			Byf("[%d] Configure the UpdateMachine extension to respond with updateStatus %s", iteration, step.updateStatus)
			setPreloadedHookResponse(ctx, c, cluster, extensionConfigName, "UpdateMachine", step.response())
		}

		Byf("[%d] Verify Machine %s has Updating condition %s=%s (reason %s) and UpToDate condition %s=%s (reason %s) when the extension responds with updateStatus %s",
			iteration, machineName, clusterv1.MachineUpdatingCondition, step.updatingStatus, step.updatingReason, clusterv1.MachineUpToDateCondition, step.upToDateStatus, step.upToDateReason, step.updateStatus)
		// Note: The message of the extension is only surfaced in the Updating condition while the Machine is updating.
		updatingMessage := ""
		if step.updatingStatus == metav1.ConditionTrue {
			updatingMessage = step.message
		}
		verifyMachineCondition(ctx, c, cluster, machineName, clusterv1.MachineUpdatingCondition, step.updatingStatus, step.updatingReason, updatingMessage, intervals...)
		verifyMachineCondition(ctx, c, cluster, machineName, clusterv1.MachineUpToDateCondition, step.upToDateStatus, step.upToDateReason, step.upToDateMessage, intervals...)
	}

	// The last step is CompletedButDirty, so the Machine should also be tracked as dirty.
	Byf("[%d] Verify Machine %s is tracked as dirty", iteration, machineName)
	Eventually(func(g Gomega) {
		machine := &clusterv1.Machine{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: machineName}, machine)).To(Succeed())
		g.Expect(machine.Annotations).To(HaveKeyWithValue(clusterv1.UpdateDirtyAnnotation, ""))
		g.Expect(machine.Annotations).ToNot(HaveKey(clusterv1.UpdateInProgressAnnotation))
	}, intervals...).Should(Succeed())

	// Remove the preloaded response, so the rollout continues as usual with the other Machines.
	Byf("[%d] Remove the preloaded response of the UpdateMachine extension", iteration)
	setPreloadedHookResponse(ctx, c, cluster, extensionConfigName, "UpdateMachine", "")
}

// verifyMachineCondition verifies that the Machine has a condition with the given type, status and reason
// and that the message of the condition contains messageSubstring.
func verifyMachineCondition(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, machineName, conditionType string, status metav1.ConditionStatus, reason, messageSubstring string, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		machine := &clusterv1.Machine{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: machineName}, machine)).To(Succeed())
		condition := conditions.Get(machine, conditionType)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(status))
		g.Expect(condition.Reason).To(Equal(reason))
		g.Expect(condition.Message).To(ContainSubstring(messageSubstring))
	}, intervals...).Should(Succeed())
}

// verifyClusterConditionAllowingDirtyMachines verifies that the condition of the Cluster is true and that its message
// is either empty or only mentions dirty Machines, i.e. Machines for which the last in-place update completed with
// updateStatus CompletedButDirty. Dirty Machines are still up-to-date, but their UpToDate message is surfaced
// in the aggregated condition of the Cluster.
func verifyClusterConditionAllowingDirtyMachines(ctx context.Context, g Gomega, c client.Client, cluster *clusterv1.Cluster, conditionType string) {
	currentCluster := &clusterv1.Cluster{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(cluster), currentCluster)).To(Succeed())

	condition := conditions.Get(currentCluster, conditionType)
	g.Expect(condition).ToNot(BeNil())
	g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
	if condition.Message == "" {
		return
	}
	for line := range strings.SplitSeq(condition.Message, "\n") {
		g.Expect(line).To(ContainSubstring("In-place update completed, but the Machine might still differ from the desired state"))
	}
}

// setFilesVariable sets the files variable of the Cluster topology to a single file with the given path and content.
func setFilesVariable(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, filePath, fileContent string) {
	originalCluster := cluster.DeepCopy()
	// Ensure the files variable is set to the expected value (first remove, then add the variable).
	cluster.Spec.Topology.Variables = slices.DeleteFunc(cluster.Spec.Topology.Variables, func(v clusterv1.ClusterVariable) bool {
		return v.Name == "files"
	})
	cluster.Spec.Topology.Variables = append(cluster.Spec.Topology.Variables, clusterv1.ClusterVariable{
		Name:  "files",
		Value: apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf(`[{"path":%q,"content":%q}]`, filePath, fileContent))},
	})
	Expect(c.Patch(ctx, cluster, client.MergeFrom(originalCluster))).To(Succeed())
}

// verifyKCPRollingOutCondition verifies that the RollingOut condition of the KubeadmControlPlane has the given reason
// and that its message contains messageSubstring.
func verifyKCPRollingOutCondition(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, reason, messageSubstring string, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		kcp := framework.GetKubeadmControlPlaneByCluster(ctx, framework.GetKubeadmControlPlaneByClusterInput{
			Lister:      c,
			ClusterName: cluster.Name,
			Namespace:   cluster.Namespace,
		})
		g.Expect(kcp).ToNot(BeNil())
		rollingOutCondition := conditions.Get(kcp, controlplanev1.KubeadmControlPlaneRollingOutCondition)
		g.Expect(rollingOutCondition).ToNot(BeNil())
		g.Expect(rollingOutCondition.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(rollingOutCondition.Reason).To(Equal(reason))
		g.Expect(rollingOutCondition.Message).To(ContainSubstring(messageSubstring))
	}, intervals...).Should(Succeed())
}

// verifyMachineDeploymentsRollingOutCondition verifies that the RollingOut condition of all MachineDeployments of the Cluster
// has the given reason and that its message contains messageSubstring.
func verifyMachineDeploymentsRollingOutCondition(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, reason, messageSubstring string, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		machineDeployments := framework.GetMachineDeploymentsByCluster(ctx, framework.GetMachineDeploymentsByClusterInput{
			Lister:      c,
			ClusterName: cluster.Name,
			Namespace:   cluster.Namespace,
		})
		g.Expect(machineDeployments).ToNot(BeEmpty())
		for _, machineDeployment := range machineDeployments {
			rollingOutCondition := conditions.Get(machineDeployment, clusterv1.MachineDeploymentRollingOutCondition)
			g.Expect(rollingOutCondition).ToNot(BeNil())
			g.Expect(rollingOutCondition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(rollingOutCondition.Reason).To(Equal(reason))
			g.Expect(rollingOutCondition.Message).To(ContainSubstring(messageSubstring))
		}
	}, intervals...).Should(Succeed())
}

// setPreloadedCanUpdateHookResponses sets the same response for the CanUpdateMachine (used by KCP) and the
// CanUpdateMachineSet (used by MachineDeployments) hooks. If response is empty the preloaded responses are removed.
func setPreloadedCanUpdateHookResponses(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName, response string) {
	for _, hookName := range []string{"CanUpdateMachine", "CanUpdateMachineSet"} {
		setPreloadedHookResponse(ctx, c, cluster, extensionConfigName, hookName, response)
	}
}

// setPreloadedHookResponse sets the response the test extension returns for a hook in the hook responses ConfigMap.
// If response is empty the preloaded response is removed, so the extension falls back to its default behavior.
func setPreloadedHookResponse(ctx context.Context, c client.Client, cluster *clusterv1.Cluster, extensionConfigName, hookName, response string) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      hookResponsesConfigMapName(cluster.Name, extensionConfigName),
		},
	}
	key := hookName + "-preloadedResponse"
	// Note: Using Eventually to handle conflicts, e.g. with the test extension recording hook calls in the same ConfigMap.
	Eventually(func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, c, configMap, func() error {
			if response == "" {
				delete(configMap.Data, key)
				return nil
			}
			if configMap.Data == nil {
				configMap.Data = map[string]string{}
			}
			configMap.Data[key] = response
			return nil
		})
		return err
	}, 10*time.Second, 1*time.Second).Should(Succeed(), "Failed to set the preloaded response for hook %s in ConfigMap %s", hookName, klog.KObj(configMap))
}

type machineObjects struct {
	ControlPlaneMachines []*clusterv1.Machine
	WorkerMachines       []*clusterv1.Machine

	KubeadmConfigByMachine map[string]*bootstrapv1.KubeadmConfig
}

// getMachineObjects retrieves Machines and corresponding KubeadmConfigs.
func getMachineObjects(ctx context.Context, g Gomega, c client.Client, cluster *clusterv1.Cluster) machineObjects {
	res := machineObjects{
		KubeadmConfigByMachine: map[string]*bootstrapv1.KubeadmConfig{},
	}

	// ControlPlane Machines.
	controlPlaneMachineList := &clusterv1.MachineList{}
	g.Expect(c.List(ctx, controlPlaneMachineList, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		clusterv1.MachineControlPlaneLabel: "",
		clusterv1.ClusterNameLabel:         cluster.Name,
	})).To(Succeed())
	for _, machine := range controlPlaneMachineList.Items {
		res.ControlPlaneMachines = append(res.ControlPlaneMachines, &machine)
		kubeadmConfig := &bootstrapv1.KubeadmConfig{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: machine.Namespace, Name: machine.Spec.Bootstrap.ConfigRef.Name}, kubeadmConfig)).To(Succeed())
		res.KubeadmConfigByMachine[machine.Name] = kubeadmConfig
	}

	// MachineDeployments Machines.
	machines := framework.GetMachinesByCluster(ctx, framework.GetMachinesByClusterInput{
		Lister:      c,
		ClusterName: cluster.Name,
		Namespace:   cluster.Namespace,
	})
	for _, machine := range machines {
		res.WorkerMachines = append(res.WorkerMachines, &machine)
		kubeadmConfig := &bootstrapv1.KubeadmConfig{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: machine.Namespace, Name: machine.Spec.Bootstrap.ConfigRef.Name}, kubeadmConfig)).To(Succeed())
		res.KubeadmConfigByMachine[machine.Name] = kubeadmConfig
	}

	return res
}

func machineNames(machines []*clusterv1.Machine) sets.Set[string] {
	ret := sets.Set[string]{}
	for _, m := range machines {
		ret.Insert(m.Name)
	}
	return ret
}
