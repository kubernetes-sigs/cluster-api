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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

const (
	configMapName    = "mhc-test"
	configMapDataKey = "signal"

	failLabelValue = "fail"
)

// KCPRemediationSpecInput is the input for KCPRemediationSpec.
type KCPRemediationSpecInput struct {
	// This spec requires following intervals to be defined in order to work:
	// - wait-cluster, used when waiting for the cluster infrastructure to be provisioned.
	// - wait-machines, used when waiting for an old machine to be remediated and a new one provisioned.
	// - check-machines-stable, used when checking that the current list of machines in stable.
	// - wait-machine-provisioned, used when waiting for a machine to be provisioned after unblocking bootstrap.
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// Flavor, if specified, must refer to a template that has a MachineHealthCheck
	// - 3 node CP, no workers
	// - Control plane machines having a pre-kubeadm command that queries for a well-known ConfigMap on the management cluster,
	//   holding up bootstrap until a signal is passed via the config map.
	//   NOTE: In order for this to work communications from workload cluster to management cluster must be enabled.
	// - An MHC targeting control plane machines with the mhc-test=fail labels and
	//     nodeStartupTimeout: 30s
	// 	   unhealthyConditions:
	//     - type: e2e.remediation.condition
	//       status: "False"
	// 	     timeout: 10s
	// If not specified, "kcp-remediation" is used.
	Flavor *string
}

// KCPRemediationSpec implements a test that verifies that Machines are remediated by MHC during unhealthy conditions.
func KCPRemediationSpec(ctx context.Context, inputGetter func() KCPRemediationSpecInput) {
	var (
		specName         = "kcp-remediation"
		input            KCPRemediationSpecInput
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

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
	})

	It("Should replace unhealthy machines", func() {
		By("Creating a workload cluster")

		// NOTE: This test is quite different from other tests, because it has to trigger failures on machines in a controlled ways.

		// creates the mhc-test ConfigMap that will be used to control machines bootstrap during the remediation tests.
		createConfigMapForMachinesBootstrapSignal(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name)

		// Creates the workload cluster.
		clusterResources = createWorkloadClusterAndWait(ctx, createWorkloadClusterAndWaitInput{
			E2EConfig:            input.E2EConfig,
			clusterctlConfigPath: input.ClusterctlConfigPath,
			proxy:                input.BootstrapClusterProxy,
			artifactFolder:       input.ArtifactFolder,
			specName:             specName,
			flavor:               input.Flavor,

			// values to be injected in the template

			namespace: namespace.Name,
			// Token with credentials to use for accessing the ConfigMap on managements cluster from the workload cluster.
			// NOTE: this func also setups credentials/RBAC rules and everything necessary to get the authentication authenticationToken.
			authenticationToken: getAuthenticationToken(ctx, input.BootstrapClusterProxy, namespace.Name),
			// Address to be used for accessing the management cluster from a workload cluster.
			serverAddr: getServerAddr(input.BootstrapClusterProxy.GetKubeconfigPath()),
		})

		// The first CP machine comes up but it does not complete bootstrap

		By("FIRST CONTROL PLANE MACHINE")

		By("Wait for the cluster to get stuck with the first CP machine not completing the bootstrap")
		allMachines, newMachines := waitForMachines(ctx, waitForMachinesInput{
			lister:                          input.BootstrapClusterProxy.GetClient(),
			namespace:                       namespace.Name,
			clusterName:                     clusterResources.Cluster.Name,
			expectedReplicas:                1,
			waitForMachinesIntervals:        input.E2EConfig.GetIntervals(specName, "wait-machines"),
			checkMachineListStableIntervals: input.E2EConfig.GetIntervals(specName, "check-machines-stable"),
		})
		Expect(allMachines).To(HaveLen(1))
		Expect(newMachines).To(HaveLen(1))
		firstMachineName := newMachines[0]
		firstMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      firstMachineName,
				Namespace: namespace.Name,
			},
		}
		Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(firstMachine), firstMachine)).To(Succeed(), "Failed to get machine %d", firstMachineName)
		Expect(firstMachine.Status.NodeRef).To(BeNil())
		log.Logf("Machine %s is up but still bootstrapping", firstMachineName)

		// Intentionally trigger remediation on the first CP, and validate the first machine is deleted and a replacement should come up.

		By("REMEDIATING FIRST CONTROL PLANE MACHINE")

		Byf("Add mhc-test:fail label to machine %s so it will be immediately remediated", firstMachineName)
		firstMachineWithLabel := firstMachine.DeepCopy()
		firstMachineWithLabel.Labels["mhc-test"] = failLabelValue
		Expect(input.BootstrapClusterProxy.GetClient().Patch(ctx, firstMachineWithLabel, client.MergeFrom(firstMachine))).To(Succeed(), "Failed to patch machine %d", firstMachineName)

		log.Logf("Wait for the first CP machine to be remediated, and the replacement machine to come up, but again get stuck with the Machine not completing the bootstrap")
		allMachines, newMachines = waitForMachines(ctx, waitForMachinesInput{
			lister:                          input.BootstrapClusterProxy.GetClient(),
			namespace:                       namespace.Name,
			clusterName:                     clusterResources.Cluster.Name,
			expectedReplicas:                1,
			expectedDeletedMachines:         []string{firstMachineName},
			waitForMachinesIntervals:        input.E2EConfig.GetIntervals(specName, "wait-machines"),
			checkMachineListStableIntervals: input.E2EConfig.GetIntervals(specName, "check-machines-stable"),
		})
		Expect(allMachines).To(HaveLen(1))
		Expect(newMachines).To(HaveLen(1))
		firstMachineReplacementName := newMachines[0]
		firstMachineReplacement := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      firstMachineReplacementName,
				Namespace: namespace.Name,
			},
		}
		Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(firstMachineReplacement), firstMachineReplacement)).To(Succeed(), "Failed to get machine %d", firstMachineReplacementName)
		Expect(firstMachineReplacement.Status.NodeRef).To(BeNil())
		log.Logf("Machine %s is up but still bootstrapping", firstMachineReplacementName)

		// The firstMachine replacement is up, meaning that the test validated that remediation of the first CP machine works (note: first CP is a special case because the cluster is not initialized yet).
		// In order to test remediation of other machine while provisioning we unblock bootstrap of the first CP replacement
		// and wait for the second cp machine to come up.

		By("FIRST CONTROL PLANE MACHINE SUCCESSFULLY REMEDIATED!")

		Byf("Unblock bootstrap for Machine %s and wait for it to be provisioned", firstMachineReplacementName)
		sendSignalToBootstrappingMachine(ctx, sendSignalToBootstrappingMachineInput{
			client:    input.BootstrapClusterProxy.GetClient(),
			namespace: namespace.Name,
			machine:   firstMachineReplacementName,
			signal:    "pass",
		})
		log.Logf("Waiting for Machine %s to be provisioned", firstMachineReplacementName)
		Eventually(func() bool {
			if err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(firstMachineReplacement), firstMachineReplacement); err != nil {
				return false
			}
			return firstMachineReplacement.Status.NodeRef != nil
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-provisioned")...).Should(BeTrue(), "Machine %s failed to be provisioned", firstMachineReplacementName)

		By("FIRST CONTROL PLANE MACHINE UP AND RUNNING!")
		By("START PROVISIONING OF SECOND CONTROL PLANE MACHINE!")

		By("Wait for the cluster to get stuck with the second CP machine not completing the bootstrap")
		allMachines, newMachines = waitForMachines(ctx, waitForMachinesInput{
			lister:                          input.BootstrapClusterProxy.GetClient(),
			namespace:                       namespace.Name,
			clusterName:                     clusterResources.Cluster.Name,
			expectedReplicas:                2,
			expectedDeletedMachines:         []string{},
			expectedOldMachines:             []string{firstMachineReplacementName},
			waitForMachinesIntervals:        input.E2EConfig.GetIntervals(specName, "wait-machines"),
			checkMachineListStableIntervals: input.E2EConfig.GetIntervals(specName, "check-machines-stable"),
		})
		Expect(allMachines).To(HaveLen(2))
		Expect(newMachines).To(HaveLen(1))
		secondMachineName := newMachines[0]
		secondMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secondMachineName,
				Namespace: namespace.Name,
			},
		}
		Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(secondMachine), secondMachine)).To(Succeed(), "Failed to get machine %d", secondMachineName)
		Expect(secondMachine.Status.NodeRef).To(BeNil())
		log.Logf("Machine %s is up but still bootstrapping", secondMachineName)

		// Intentionally trigger remediation on the second CP, and validate and validate also this one is deleted and a replacement should come up.

		By("REMEDIATING SECOND CONTROL PLANE MACHINE")

		Byf("Add mhc-test:fail label to machine %s so it will be immediately remediated", firstMachineName)
		secondMachineWithLabel := secondMachine.DeepCopy()
		secondMachineWithLabel.Labels["mhc-test"] = failLabelValue
		Expect(input.BootstrapClusterProxy.GetClient().Patch(ctx, secondMachineWithLabel, client.MergeFrom(secondMachine))).To(Succeed(), "Failed to patch machine %d", secondMachineName)

		log.Logf("Wait for the second CP machine to be remediated, and the replacement machine to come up, but again get stuck with the Machine not completing the bootstrap")
		allMachines, newMachines = waitForMachines(ctx, waitForMachinesInput{
			lister:                          input.BootstrapClusterProxy.GetClient(),
			namespace:                       namespace.Name,
			clusterName:                     clusterResources.Cluster.Name,
			expectedReplicas:                2,
			expectedDeletedMachines:         []string{secondMachineName},
			expectedOldMachines:             []string{firstMachineReplacementName},
			waitForMachinesIntervals:        input.E2EConfig.GetIntervals(specName, "wait-machines"),
			checkMachineListStableIntervals: input.E2EConfig.GetIntervals(specName, "check-machines-stable"),
		})
		Expect(allMachines).To(HaveLen(2))
		Expect(newMachines).To(HaveLen(1))
		secondMachineReplacementName := newMachines[0]
		secondMachineReplacement := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secondMachineReplacementName,
				Namespace: namespace.Name,
			},
		}
		Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(secondMachineReplacement), secondMachineReplacement)).To(Succeed(), "Failed to get machine %d", secondMachineReplacementName)
		Expect(secondMachineReplacement.Status.NodeRef).To(BeNil())
		log.Logf("Machine %s is up but still bootstrapping", secondMachineReplacementName)

		// The secondMachine replacement is up, meaning that the test validated that remediation of the second CP machine works (note: this test remediation after the cluster is initialized, but not yet fully provisioned).
		// In order to test remediation after provisioning we unblock bootstrap of the second CP replacement as well as for the third CP machine.
		// and wait for the second cp machine to come up.

		By("SECOND CONTROL PLANE MACHINE SUCCESSFULLY REMEDIATED!")

		Byf("Unblock bootstrap for Machine %s and wait for it to be provisioned", secondMachineReplacementName)
		sendSignalToBootstrappingMachine(ctx, sendSignalToBootstrappingMachineInput{
			client:    input.BootstrapClusterProxy.GetClient(),
			namespace: namespace.Name,
			machine:   secondMachineReplacementName,
			signal:    "pass",
		})
		log.Logf("Waiting for Machine %s to be provisioned", secondMachineReplacementName)
		Eventually(func() bool {
			if err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(secondMachineReplacement), secondMachineReplacement); err != nil {
				return false
			}
			return secondMachineReplacement.Status.NodeRef != nil
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-provisioned")...).Should(BeTrue(), "Machine %s failed to be provisioned", secondMachineReplacementName)

		By("SECOND CONTROL PLANE MACHINE UP AND RUNNING!")
		By("START PROVISIONING OF THIRD CONTROL PLANE MACHINE!")

		By("Wait for the cluster to get stuck with the third CP machine not completing the bootstrap")
		allMachines, newMachines = waitForMachines(ctx, waitForMachinesInput{
			lister:                          input.BootstrapClusterProxy.GetClient(),
			namespace:                       namespace.Name,
			clusterName:                     clusterResources.Cluster.Name,
			expectedReplicas:                3,
			expectedDeletedMachines:         []string{},
			expectedOldMachines:             []string{firstMachineReplacementName, secondMachineReplacementName},
			waitForMachinesIntervals:        input.E2EConfig.GetIntervals(specName, "wait-machines"),
			checkMachineListStableIntervals: input.E2EConfig.GetIntervals(specName, "check-machines-stable"),
		})
		Expect(allMachines).To(HaveLen(3))
		Expect(newMachines).To(HaveLen(1))
		thirdMachineName := newMachines[0]
		thirdMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      thirdMachineName,
				Namespace: namespace.Name,
			},
		}
		Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(thirdMachine), thirdMachine)).To(Succeed(), "Failed to get machine %d", thirdMachineName)
		Expect(thirdMachine.Status.NodeRef).To(BeNil())
		log.Logf("Machine %s is up but still bootstrapping", thirdMachineName)

		Byf("Unblock bootstrap for Machine %s and wait for it to be provisioned", thirdMachineName)
		sendSignalToBootstrappingMachine(ctx, sendSignalToBootstrappingMachineInput{
			client:    input.BootstrapClusterProxy.GetClient(),
			namespace: namespace.Name,
			machine:   thirdMachineName,
			signal:    "pass",
		})
		log.Logf("Waiting for Machine %s to be provisioned", thirdMachineName)
		Eventually(func() bool {
			if err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(thirdMachine), thirdMachine); err != nil {
				return false
			}
			return thirdMachine.Status.NodeRef != nil
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-provisioned")...).Should(BeTrue(), "Machine %s failed to be provisioned", thirdMachineName)

		// All three CP machines are up.

		By("ALL THE CONTROL PLANE MACHINES SUCCESSFULLY PROVISIONED!")

		// We now want to test remediation of a CP machine already provisioned.
		// In order to do so we need to apply both mhc-test:fail as well as setting an unhealthy condition in order to trigger remediation

		By("REMEDIATING THIRD CP")

		Byf("Add mhc-test:fail label to machine %s and set an unhealthy condition on the node so it will be immediately remediated", thirdMachineName)
		thirdMachineWithLabel := thirdMachine.DeepCopy()
		thirdMachineWithLabel.Labels["mhc-test"] = failLabelValue
		Expect(input.BootstrapClusterProxy.GetClient().Patch(ctx, thirdMachineWithLabel, client.MergeFrom(thirdMachine))).To(Succeed(), "Failed to patch machine %d", thirdMachineName)

		unhealthyNodeCondition := corev1.NodeCondition{
			Type:               "e2e.remediation.condition",
			Status:             "False",
			LastTransitionTime: metav1.Time{Time: time.Now()},
		}
		framework.PatchNodeCondition(ctx, framework.PatchNodeConditionInput{
			ClusterProxy:  input.BootstrapClusterProxy,
			Cluster:       clusterResources.Cluster,
			NodeCondition: unhealthyNodeCondition,
			Machine:       *thirdMachine, // TODO: make this a pointer.
		})

		log.Logf("Wait for the third CP machine to be remediated, and the replacement machine to come up, but again get stuck with the Machine not completing the bootstrap")
		allMachines, newMachines = waitForMachines(ctx, waitForMachinesInput{
			lister:                          input.BootstrapClusterProxy.GetClient(),
			namespace:                       namespace.Name,
			clusterName:                     clusterResources.Cluster.Name,
			expectedReplicas:                3,
			expectedDeletedMachines:         []string{thirdMachineName},
			expectedOldMachines:             []string{firstMachineReplacementName, secondMachineReplacementName},
			waitForMachinesIntervals:        input.E2EConfig.GetIntervals(specName, "wait-machines"),
			checkMachineListStableIntervals: input.E2EConfig.GetIntervals(specName, "check-machines-stable"),
		})
		Expect(allMachines).To(HaveLen(3))
		Expect(newMachines).To(HaveLen(1))
		thirdMachineReplacementName := newMachines[0]
		thirdMachineReplacement := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      thirdMachineReplacementName,
				Namespace: namespace.Name,
			},
		}
		Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(thirdMachineReplacement), thirdMachineReplacement)).To(Succeed(), "Failed to get machine %d", thirdMachineReplacementName)
		Expect(thirdMachineReplacement.Status.NodeRef).To(BeNil())
		log.Logf("Machine %s is up but still bootstrapping", thirdMachineReplacementName)

		// The thirdMachine replacement is up, meaning that the test validated that remediation of the third CP machine works (note: this test remediation after the cluster is fully provisioned).

		By("THIRD CP SUCCESSFULLY REMEDIATED!")

		Byf("Unblock bootstrap for Machine %s and wait for it to be provisioned", thirdMachineReplacementName)
		sendSignalToBootstrappingMachine(ctx, sendSignalToBootstrappingMachineInput{
			client:    input.BootstrapClusterProxy.GetClient(),
			namespace: namespace.Name,
			machine:   thirdMachineReplacementName,
			signal:    "pass",
		})
		log.Logf("Waiting for Machine %s to be provisioned", thirdMachineReplacementName)
		Eventually(func() bool {
			if err := input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(thirdMachineReplacement), thirdMachineReplacement); err != nil {
				return false
			}
			return thirdMachineReplacement.Status.NodeRef != nil
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-provisioned")...).Should(BeTrue(), "Machine %s failed to be provisioned", thirdMachineReplacementName)

		// All three CP machines are up again.

		By("CP BACK TO FULL OPERATIONAL STATE!")

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

func createConfigMapForMachinesBootstrapSignal(ctx context.Context, writer client.Writer, namespace string) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			configMapDataKey: "hold",
		},
	}
	Expect(writer.Create(ctx, cm)).To(Succeed(), "failed to create mhc-test config map")
}

type createWorkloadClusterAndWaitInput struct {
	E2EConfig            *clusterctl.E2EConfig
	clusterctlConfigPath string
	proxy                framework.ClusterProxy
	artifactFolder       string
	specName             string
	flavor               *string
	namespace            string
	authenticationToken  []byte
	serverAddr           string
}

// createWorkloadClusterAndWait creates a workload cluster ard return ass soon as the cluster infrastructure is ready.
// NOTE: clusterResources is filled only partially.
func createWorkloadClusterAndWait(ctx context.Context, input createWorkloadClusterAndWaitInput) (clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult) {
	clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)

	// gets the cluster template
	log.Logf("Getting the cluster template yaml")
	clusterName := fmt.Sprintf("%s-%s", input.specName, util.RandomString(6))
	workloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
		// pass the clusterctl config file that points to the local provider repository created for this test,
		ClusterctlConfigPath: input.clusterctlConfigPath,
		// pass reference to the management cluster hosting this test
		KubeconfigPath: input.proxy.GetKubeconfigPath(),

		// select template
		Flavor: pointer.StringDeref(input.flavor, "kcp-remediation"),
		// define template variables
		Namespace:                input.namespace,
		ClusterName:              clusterName,
		KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
		ControlPlaneMachineCount: pointer.Int64(3),
		WorkerMachineCount:       pointer.Int64(0),
		InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
		// setup clusterctl logs folder
		LogFolder: filepath.Join(input.artifactFolder, "clusters", input.proxy.GetName()),
		// Adds authenticationToken, server address and namespace variables to be injected in the cluster template.
		ClusterctlVariables: map[string]string{
			"TOKEN":     string(input.authenticationToken),
			"SERVER":    input.serverAddr,
			"NAMESPACE": input.namespace,
		},
	})
	Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

	Eventually(func() error {
		return input.proxy.Apply(ctx, workloadClusterTemplate)
	}, 10*time.Second).Should(Succeed(), "Failed to apply the cluster template")

	log.Logf("Waiting for the cluster infrastructure to be provisioned")
	clusterResources.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.proxy.GetClient(),
		Namespace: input.namespace,
		Name:      clusterName,
	}, input.E2EConfig.GetIntervals(input.specName, "wait-cluster")...)

	return clusterResources
}

type sendSignalToBootstrappingMachineInput struct {
	client    client.Client
	namespace string
	machine   string
	signal    string
}

// sendSignalToBootstrappingMachine sends a signal to a machine stuck during bootstrap.
func sendSignalToBootstrappingMachine(ctx context.Context, input sendSignalToBootstrappingMachineInput) {
	log.Logf("Sending bootstrap signal %s to Machine %s", input.signal, input.machine)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: input.namespace,
		},
	}
	Expect(input.client.Get(ctx, client.ObjectKeyFromObject(cm), cm)).To(Succeed(), "failed to get mhc-test config map")

	cmWithSignal := cm.DeepCopy()
	cmWithSignal.Data[configMapDataKey] = input.signal
	Expect(input.client.Patch(ctx, cmWithSignal, client.MergeFrom(cm))).To(Succeed(), "failed to patch mhc-test config map")

	log.Logf("Waiting for Machine %s to acknowledge signal %s has been received", input.machine, input.signal)
	Eventually(func() string {
		_ = input.client.Get(ctx, client.ObjectKeyFromObject(cmWithSignal), cmWithSignal)
		return cmWithSignal.Data[configMapDataKey]
	}, "1m", "10s").Should(Equal(fmt.Sprintf("ack-%s", input.signal)), "Failed to get ack signal from machine %s", input.machine)

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.machine,
			Namespace: input.namespace,
		},
	}
	Expect(input.client.Get(ctx, client.ObjectKeyFromObject(machine), machine)).To(Succeed())

	// Resetting the signal in the config map
	cmWithSignal.Data[configMapDataKey] = "hold"
	Expect(input.client.Patch(ctx, cmWithSignal, client.MergeFrom(cm))).To(Succeed(), "failed to patch mhc-test config map")
}

type waitForMachinesInput struct {
	lister                          framework.Lister
	namespace                       string
	clusterName                     string
	expectedReplicas                int
	expectedOldMachines             []string
	expectedDeletedMachines         []string
	waitForMachinesIntervals        []interface{}
	checkMachineListStableIntervals []interface{}
}

// waitForMachines waits for machines to reach a well known state defined by number of replicas, a list of machines to exists,
// a list of machines to not exists anymore. The func also check that the state is stable for some time before
// returning the list of new machines.
func waitForMachines(ctx context.Context, input waitForMachinesInput) (allMachineNames, newMachineNames []string) {
	inClustersNamespaceListOption := client.InNamespace(input.namespace)
	matchClusterListOption := client.MatchingLabels{
		clusterv1.ClusterNameLabel:         input.clusterName,
		clusterv1.MachineControlPlaneLabel: "",
	}

	expectedOldMachines := sets.NewString(input.expectedOldMachines...)
	expectedDeletedMachines := sets.NewString(input.expectedDeletedMachines...)
	allMachines := sets.NewString()
	newMachines := sets.NewString()
	machineList := &clusterv1.MachineList{}

	// Waits for the desired set of machines to exist.
	log.Logf("Waiting for %d machines, must have %s, must not have %s", input.expectedReplicas, expectedOldMachines.List(), expectedDeletedMachines.List())
	Eventually(func() bool {
		// Gets the list of machines
		if err := input.lister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			return false
		}
		allMachines = sets.NewString()
		for i := range machineList.Items {
			allMachines.Insert(machineList.Items[i].Name)
		}

		// Compute new machines (all - old - to be deleted)
		newMachines = allMachines.Clone()
		newMachines.Delete(expectedOldMachines.List()...)
		newMachines.Delete(expectedDeletedMachines.List()...)

		log.Logf(" - expected %d, got %d: %s, of which new %s, must have check: %t, must not have check: %t", input.expectedReplicas, allMachines.Len(), allMachines.List(), newMachines.List(), allMachines.HasAll(expectedOldMachines.List()...), !allMachines.HasAny(expectedDeletedMachines.List()...))

		// Ensures all the expected old machines are still there.
		if !allMachines.HasAll(expectedOldMachines.List()...) {
			return false
		}

		// Ensures none of the machines to be deleted is still there.
		if allMachines.HasAny(expectedDeletedMachines.List()...) {
			return false
		}

		return allMachines.Len() == input.expectedReplicas
	}, input.waitForMachinesIntervals...).Should(BeTrue(), "Failed to get the expected list of machines: got %s (expected %d machines, must have %s, must not have %s)", allMachines.List(), input.expectedReplicas, expectedOldMachines.List(), expectedDeletedMachines.List())
	log.Logf("Got %d machines: %s", input.expectedReplicas, allMachines.List())

	// Ensures the desired set of machines is stable (no further machines are created or deleted).
	log.Logf("Checking the list of machines is stable")
	allMachinesNow := sets.NewString()
	Consistently(func() bool {
		// Gets the list of machines
		if err := input.lister.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			return false
		}
		allMachinesNow = sets.NewString()
		for i := range machineList.Items {
			allMachinesNow.Insert(machineList.Items[i].Name)
		}

		return allMachines.Len() == allMachinesNow.Len() && allMachines.HasAll(allMachinesNow.List()...)
	}, input.checkMachineListStableIntervals...).Should(BeTrue(), "Expected list of machines is not stable: got %s, expected %s", allMachinesNow.List(), allMachines.List())

	return allMachines.List(), newMachines.List()
}

// getServerAddr returns the address to be used for accessing the management cluster from a workload cluster.
func getServerAddr(kubeconfigPath string) string {
	kubeConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "failed to load management cluster's kubeconfig file")

	clusterName := kubeConfig.Contexts[kubeConfig.CurrentContext].Cluster
	Expect(clusterName).ToNot(BeEmpty(), "failed to identify current cluster name in management cluster's kubeconfig file")

	serverAddr := kubeConfig.Clusters[clusterName].Server
	Expect(serverAddr).ToNot(BeEmpty(), "failed to identify current server address in management cluster's kubeconfig file")

	// On CAPD, if not running on Linux, we need to use Docker's proxy to connect back to the host
	// to the CAPD cluster. Moby on Linux doesn't use the host.docker.internal DNS name.
	if runtime.GOOS != "linux" {
		serverAddr = strings.ReplaceAll(serverAddr, "127.0.0.1", "host.docker.internal")
	}
	return serverAddr
}

// getAuthenticationToken returns a bearer authenticationToken with minimal RBAC permissions to access the mhc-test ConfigMap that will be used
// to control machines bootstrap during the remediation tests.
func getAuthenticationToken(ctx context.Context, managementClusterProxy framework.ClusterProxy, namespace string) []byte {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mhc-test",
			Namespace: namespace,
		},
	}
	Expect(managementClusterProxy.GetClient().Create(ctx, sa)).To(Succeed(), "failed to create mhc-test service account")

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mhc-test",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"get", "list", "patch"},
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{"mhc-test"},
			},
		},
	}
	Expect(managementClusterProxy.GetClient().Create(ctx, role)).To(Succeed(), "failed to create mhc-test role")

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mhc-test",
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				APIGroup:  "",
				Name:      "mhc-test",
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     "mhc-test",
		},
	}
	Expect(managementClusterProxy.GetClient().Create(ctx, roleBinding)).To(Succeed(), "failed to create mhc-test role binding")

	cmd := exec.CommandContext(ctx, "kubectl", fmt.Sprintf("--kubeconfig=%s", managementClusterProxy.GetKubeconfigPath()), fmt.Sprintf("--namespace=%s", namespace), "create", "token", "mhc-test") //nolint:gosec
	stdout, err := cmd.StdoutPipe()
	Expect(err).ToNot(HaveOccurred(), "failed to get stdout for kubectl create authenticationToken")
	stderr, err := cmd.StderrPipe()
	Expect(err).ToNot(HaveOccurred(), "failed to get stderr for kubectl create authenticationToken")

	Expect(cmd.Start()).To(Succeed(), "failed to run kubectl create authenticationToken")

	output, err := io.ReadAll(stdout)
	Expect(err).ToNot(HaveOccurred(), "failed to read stdout from kubectl create authenticationToken")
	errout, err := io.ReadAll(stderr)
	Expect(err).ToNot(HaveOccurred(), "failed to read stderr from kubectl create authenticationToken")

	Expect(cmd.Wait()).To(Succeed(), "failed to wait kubectl create authenticationToken")
	Expect(errout).To(BeEmpty())

	return output
}
