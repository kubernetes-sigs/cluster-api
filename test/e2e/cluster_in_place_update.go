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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
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
// Note: This test works with KCP as it tests the KCP in-place update feature.
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
			Byf("[%d] Trigger in-place update by modifying the files variable", i)

			originalCluster := cluster.DeepCopy()
			// Ensure the files variable is set to the expected value (first remove, then add the variable).
			cluster.Spec.Topology.Variables = slices.DeleteFunc(cluster.Spec.Topology.Variables, func(v clusterv1.ClusterVariable) bool {
				return v.Name == "files"
			})
			cluster.Spec.Topology.Variables = append(cluster.Spec.Topology.Variables, clusterv1.ClusterVariable{
				Name:  "files",
				Value: apiextensionsv1.JSON{Raw: []byte(fmt.Sprintf(`[{"path":%q,"content":%q}]`, filePath, fileContent))},
			})
			Expect(mgmtClient.Patch(ctx, cluster, client.MergeFrom(originalCluster))).To(Succeed())

			var machineObjectsAfterInPlaceUpdate machineObjects
			Eventually(func(g Gomega) {
				// Ensure the in-place update was done.
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
