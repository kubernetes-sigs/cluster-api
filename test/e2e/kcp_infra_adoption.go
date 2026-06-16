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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/secret"
)

// KCPInfraAdoptionSpecInput is the input for KCPInfraAdoptionSpec.
type KCPInfraAdoptionSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// Cluster name allows to specify a deterministic clusterName.
	// If not set, a random one will be generated.
	ClusterName *string

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	// Flavor, if specified, must refer to a template that is
	// specially crafted with FIXME: describe requirements for the template
	Flavor *string

	// ExtensionConfigName is the name of the ExtensionConfig. Defaults to "kcp-infra-adoption".
	// This value is provided to clusterctl as "EXTENSION_CONFIG_NAME" variable and can be used to template the
	// name of the ExtensionConfig into the ClusterClass.
	ExtensionConfigName string

	// ExtensionServiceNamespace is the namespace where the service for the Runtime SDK is located
	// and is used to configure in the test-namespace scoped ExtensionConfig.
	ExtensionServiceNamespace string

	// ExtensionServiceName is the name of the service to configure in the test-namespace scoped ExtensionConfig.
	ExtensionServiceName string

	// ControlPlaneMachineCount defines the number of control plane machines to be added to the workload cluster.
	// If not specified, 1 will be used.
	ControlPlaneMachineCount *int64

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Allows to inject a function to be run after the cluster is paused and before to force delete a cluster and its object (to orphan the corresponding infra).
	// If not specified, this is a no-op.
	// Note: the func must be idempotent, because it is called in an Eventually loop.
	BeforeForceDelete func(ctx context.Context, managementClusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster, objects ClusterObjects) error

	// Allows to inject a function to be run BeforeAdoption of the orphaned infra; the function also allows
	// to choose which one between the boostrap management cluster and the cluster on the orphaned infra should be used for adoption
	// by returning the corresponding ClusterProxy.
	//
	// In case the cluster on the orphaned infra should be used for adoption, this function must be used to
	// install the required CAPI providers; as a consequence, once adoption completes, this will become
	// a self-hosted management cluster.
	// Note: It is also responsibility of the implementers of this func to set up everything else required for adoption,
	// e.g. cluster secrets, clusterclass and templates, extensions configs.
	//
	// If not specified, this is a no-op and the boostrap management cluster will be used for adoption.
	BeforeAdoption func(ctx context.Context, managementClusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster) (framework.ClusterProxy, error)

	// SkipRollout allows to skip the rollout after the Adoption.
	SkipRollout bool
}

// KCPInfraAdoptionSpec implements a test that verifies KCP to properly adopt existing control plane Machines.
func KCPInfraAdoptionSpec(ctx context.Context, inputGetter func() KCPInfraAdoptionSpecInput) {
	var (
		specName                  = "kcp-infra-adoption"
		input                     KCPInfraAdoptionSpecInput
		namespace                 *corev1.Namespace
		cancelWatches             context.CancelFunc
		clusterResources          *clusterctl.ApplyClusterTemplateAndWaitResult
		newManagementClusterProxy framework.ClusterProxy
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

	It("Should adopt up-to-date control plane Machines without modification", func() {
		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			By("Deploy Test Extension ExtensionConfig")
			Expect(input.BootstrapClusterProxy.GetClient().Create(ctx,
				extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, true, false, namespace.Name))).
				To(Succeed(), "Failed to create the extension config")
		}

		By("Creating a workload cluster")

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		flavor := "topology"
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		controlPlaneMachineCount := ptr.To[int64](1)
		if input.ControlPlaneMachineCount != nil {
			controlPlaneMachineCount = input.ControlPlaneMachineCount
		}

		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		if input.ClusterName != nil {
			clusterName = *input.ClusterName
		}

		variables := map[string]string{
			// This is used to template the name of the ExtensionConfig into the ClusterClass.
			"EXTENSION_CONFIG_NAME": input.ExtensionConfigName,
		}

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersion),
				ControlPlaneMachineCount: controlPlaneMachineCount,
				WorkerMachineCount:       ptr.To[int64](0),
				ClusterctlVariables:      variables,
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		By("Orphaning the Cluster infrastructure and control plane Machines")

		log.Logf("Pause the Cluster")
		originalCluster := clusterResources.Cluster.DeepCopy()

		clusterBeforePatch := originalCluster.DeepCopy()
		originalCluster.Spec.Paused = ptr.To(true)
		Expect(input.BootstrapClusterProxy.GetClient().Patch(ctx, originalCluster, ctrlclient.MergeFrom(clusterBeforePatch))).To(Succeed(), "failed to patch Cluster")

		log.Logf("Remove owner refs from cluster certificates")
		secrets := &corev1.SecretList{}
		Expect(input.BootstrapClusterProxy.GetClient().List(ctx, secrets, ctrlclient.InNamespace(originalCluster.Namespace), ctrlclient.MatchingLabels(map[string]string{clusterv1.ClusterNameLabel: originalCluster.Name}))).To(Succeed(), "failed to list secrets")
		for _, s := range secrets.Items {
			if secret.HasPurposeSuffix(s.Name) {
				original := s.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{})
				Expect(input.BootstrapClusterProxy.GetClient().Patch(ctx, &s, ctrlclient.MergeFrom(original))).To(Succeed(), "failed to patch Secret")
			}
		}

		log.Logf("Force delete Cluster and cluster resources")
		originalControlPlaneMachines := []*clusterv1.Machine{}
		originalInfrastructureMachineByMachine := map[string]*unstructured.Unstructured{}
		Eventually(func(g Gomega) error {
			objects := getClusterObjects(ctx, g, input.BootstrapClusterProxy, clusterResources.Cluster)

			if input.BeforeForceDelete != nil {
				log.Logf("Calling BeforeForceDelete for cluster %s", klog.KObj(clusterResources.Cluster))
				if err := input.BeforeForceDelete(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, objects); err != nil {
					return fmt.Errorf("failed to run BeforeForceDelete for cluster %s: %v", klog.KObj(clusterResources.Cluster), err)
				}
			}

			for _, m := range objects.ControlPlaneMachines {
				originalControlPlaneMachines = append(originalControlPlaneMachines, m.DeepCopy())
				im, ok := objects.InfrastructureMachineByMachine[m.Name]
				if !ok {
					return fmt.Errorf("missing infrastructure machine for machine %s", m.Name)
				}
				originalInfrastructureMachineByMachine[m.Name] = im.DeepCopy()
			}

			return forceDeleteClusterObjects(ctx, input.BootstrapClusterProxy, clusterResources.Cluster, objects)
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		newManagementClusterProxy = input.BootstrapClusterProxy
		if input.BeforeAdoption != nil {
			log.Logf("Calling BeforeAdoption for cluster %s", klog.KObj(clusterResources.Cluster))
			var err error
			newManagementClusterProxy, err = input.BeforeAdoption(ctx, input.BootstrapClusterProxy, clusterResources.Cluster)
			Expect(err).NotTo(HaveOccurred(), "failed to run BeforeAdoption for cluster %s: %v", klog.KObj(clusterResources.Cluster), err)
		}

		By("Re-creating the workload cluster")

		log.Logf("Create the Cluster")
		newCluster := originalCluster.DeepCopy()
		newCluster.SetResourceVersion("")
		newCluster.SetUID("")
		newCluster.SetFinalizers([]string{})
		newCluster.Spec.Paused = ptr.To(false)
		newCluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{}
		newCluster.Spec.ControlPlaneRef = clusterv1.ContractVersionedObjectReference{}
		newCluster.Status = clusterv1.ClusterStatus{}
		if newCluster.Spec.Topology.ControlPlane.Metadata.Annotations == nil {
			newCluster.Spec.Topology.ControlPlane.Metadata.Annotations = map[string]string{}
		}
		newCluster.Spec.Topology.ControlPlane.Metadata.Annotations[clusterv1.PausedAnnotation] = ""
		newCluster.Spec.Topology.ControlPlane.HealthCheck.Enabled = ptr.To(false)
		Expect(newManagementClusterProxy.GetClient().Create(ctx, newCluster)).To(Succeed(), "failed to create new Cluster")

		log.Logf("Wait for the control plane to exist")
		newKcp := &controlplanev1.KubeadmControlPlane{}
		Eventually(func(_ Gomega) error {
			err := newManagementClusterProxy.GetClient().Get(ctx, ctrlclient.ObjectKeyFromObject(newCluster), newCluster)
			if err != nil {
				return err
			}
			if !newCluster.Spec.ControlPlaneRef.IsDefined() {
				return fmt.Errorf("control plane for Cluster %s is not defined", klog.KObj(newCluster))
			}
			if newCluster.Spec.ControlPlaneRef.Kind != "KubeadmControlPlane" {
				return fmt.Errorf("control plane for Cluster %s is not a KubeadmControlPlane", klog.KObj(newCluster))
			}

			err = newManagementClusterProxy.GetClient().Get(ctx, ctrlclient.ObjectKey{Namespace: newCluster.Namespace, Name: newCluster.Spec.ControlPlaneRef.Name}, newKcp)
			if err != nil {
				return err
			}
			if _, ok := newKcp.GetAnnotations()[clusterv1.PausedAnnotation]; !ok {
				return fmt.Errorf("control plane %s is not paused", klog.KObj(newKcp))
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		log.Logf(" - KubeadmControlPlane, %s", klog.KObj(newKcp))

		By("Create stand-alone control plane machines")

		newControlPlaneMachinesNames := sets.New[string]()
		newControlPlaneMachines := []*clusterv1.Machine{}
		newInfrastructureMachineByMachine := map[string]*unstructured.Unstructured{}
		for _, m := range originalControlPlaneMachines {
			// Re-create a bootstrap data secret.
			// Note: create an empty secret, machines are already bootstrapped.
			newSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      m.Spec.Bootstrap.ConfigRef.Name,
					Namespace: m.Namespace,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel: newCluster.Name,
					},
				},
				Type: clusterv1.ClusterSecretType,
			}
			Expect(newManagementClusterProxy.GetClient().Create(ctx, newSecret)).To(Succeed(), "failed to create new KubeadmConfig")
			log.Logf(" - Secret, %s", klog.KObj(newSecret))

			// Re-create bootstrap config.
			// Note: Start from KCP spec; fixup InitConfiguration and JoinConfiguration so the object is aligned to what we have for joined machines.
			newBC := &bootstrapv1.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      m.Spec.Bootstrap.ConfigRef.Name,
					Namespace: m.Namespace,
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:         newCluster.Name,
						clusterv1.MachineControlPlaneLabel: "",
					},
				},
				Spec: newKcp.Spec.KubeadmConfigSpec,
			}
			newBC.Spec.InitConfiguration = bootstrapv1.InitConfiguration{}
			if newBC.Spec.JoinConfiguration.ControlPlane == nil {
				newBC.Spec.JoinConfiguration.ControlPlane = &bootstrapv1.JoinControlPlane{}
			}
			Expect(newManagementClusterProxy.GetClient().Create(ctx, newBC)).To(Succeed(), "failed to create new KubeadmConfig")
			log.Logf(" - KubeadmConfig, %s", klog.KObj(newBC))

			// Re-create infrastructure machine
			newIM := originalInfrastructureMachineByMachine[m.Name].DeepCopy()
			newIM.SetResourceVersion("")
			newIM.SetUID("")
			newIM.SetLabels(map[string]string{
				clusterv1.ClusterNameLabel:         newCluster.Name,
				clusterv1.MachineControlPlaneLabel: "",
			})
			newIM.SetAnnotations(map[string]string{})
			newIM.SetOwnerReferences([]metav1.OwnerReference{})
			newIM.SetFinalizers([]string{})
			delete(newIM.Object, "status")
			Expect(newManagementClusterProxy.GetClient().Create(ctx, newIM)).To(Succeed(), "failed to create new InfrastructureMachine")
			log.Logf(" - %s %s", newIM.GetKind(), klog.KObj(newIM))

			newM := m.DeepCopy()
			newM.SetResourceVersion("")
			newM.SetUID("")
			newM.Labels = map[string]string{
				clusterv1.ClusterNameLabel:         newCluster.Name,
				clusterv1.MachineControlPlaneLabel: "",
			}
			newM.Annotations = map[string]string{}
			newM.SetOwnerReferences([]metav1.OwnerReference{})
			newM.SetFinalizers([]string{})
			newM.Spec.Bootstrap.ConfigRef.Name = newBC.Name
			newM.Spec.Bootstrap.DataSecretName = new(newSecret.Name)
			newM.Spec.InfrastructureRef.Name = newIM.GetName()
			newM.Status = clusterv1.MachineStatus{}
			Expect(newManagementClusterProxy.GetClient().Create(ctx, newM)).To(Succeed(), "failed to create new Machine")

			newControlPlaneMachinesNames.Insert(newM.Name)
			newControlPlaneMachines = append(newControlPlaneMachines, newM)
			newInfrastructureMachineByMachine[newM.Name] = newIM
			log.Logf(" - Machine, %s", klog.KObj(newM))
		}

		log.Logf("Wait for Machine -> InfrastructureMachine reconcile")
		Eventually(func(_ Gomega) error {
			for _, m := range newControlPlaneMachines {
				err := newManagementClusterProxy.GetClient().Get(ctx, ctrlclient.ObjectKeyFromObject(m), m)
				if err != nil {
					return fmt.Errorf("failed to get Machine %s", klog.KObj(m))
				}
				if len(m.Status.Addresses) == 0 {
					return fmt.Errorf("machine %s does not have status.addresses yet", klog.KObj(m))
				}

				im := newInfrastructureMachineByMachine[m.Name].DeepCopy()
				err = newManagementClusterProxy.GetClient().Get(ctx, ctrlclient.ObjectKey{Namespace: m.Namespace, Name: m.Spec.InfrastructureRef.Name}, im)
				if err != nil {
					return fmt.Errorf("failed to get %s %s", im.GetKind(), klog.KObj(m))
				}
				if !util.IsControlledBy(im, m, clusterv1.GroupVersion.WithKind("Machine").GroupKind()) {
					return fmt.Errorf("%s %s is not controlled by Machine, %s", im.GetKind(), klog.KObj(im), klog.KObj(m))
				}
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		By("Un-pausing the KubeadmControlPlane")
		clusterBeforePatch = newCluster.DeepCopy()
		delete(newCluster.Spec.Topology.ControlPlane.Metadata.Annotations, clusterv1.PausedAnnotation)
		if len(newCluster.Spec.Topology.ControlPlane.Metadata.Annotations) == 0 {
			newCluster.Spec.Topology.ControlPlane.Metadata.Annotations = nil
		}
		err := newManagementClusterProxy.GetClient().Patch(ctx, newCluster, ctrlclient.MergeFrom(clusterBeforePatch))
		Expect(err).To(Succeed(), "failed to patch Cluster")

		log.Logf("Wait for KubeadmControlPlane -> Machine reconcile")
		Eventually(func(_ Gomega) error {
			for _, m := range newControlPlaneMachines {
				err := newManagementClusterProxy.GetClient().Get(ctx, ctrlclient.ObjectKeyFromObject(m), m)
				if err != nil {
					return fmt.Errorf("failed to get Machine %s", klog.KObj(m))
				}

				if !util.IsControlledBy(m, newKcp, controlplanev1.GroupVersion.WithKind("KubeadmControlPlane").GroupKind()) {
					return fmt.Errorf("machine, %s is not controlled by KubeadmControlPlane, %s", klog.KObj(m), klog.KObj(newKcp))
				}

				if !conditions.IsTrue(m, clusterv1.MachineAvailableCondition) {
					return fmt.Errorf("machine, %s does not have Available condition true", klog.KObj(m))
				}
			}
			return nil
		}, 60*time.Second, 5*time.Second).Should(Succeed())

		By("Verifying there are no unexpected rollouts after adoption")
		Consistently(func(g Gomega) {
			machinesAfterUpgrade := getMachinesByCluster(ctx, newManagementClusterProxy.GetClient(), newCluster)
			g.Expect(machinesAfterUpgrade.Equal(newControlPlaneMachinesNames)).To(BeTrue(), "Machines must not be replaced after adoption")
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    newManagementClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		Byf("Verify Machines Ready condition is true")
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    newManagementClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		if input.SkipRollout {
			By("PASSED!")
			return
		}

		By("Trigger a control plane rollout")
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: newManagementClusterProxy,
			Cluster:      newCluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.Rollout.After = metav1.NewTime(time.Now())
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		Byf("Verify control plane Machines have been rolled out")
		Eventually(func(g Gomega) {
			machinesAfterRollout := getMachinesByCluster(ctx, newManagementClusterProxy.GetClient(), newCluster)
			g.Expect(machinesAfterRollout.HasAny(newControlPlaneMachinesNames.UnsortedList()...)).To(BeFalse(), "Machines must be replaced with rollout")
		}, input.E2EConfig.GetIntervals(specName, "wait-control-plane")...).Should(Succeed())

		Byf("Verify control plane Machines Ready condition is true")
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    newManagementClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    newManagementClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		By("PASSED!")
	})

	AfterEach(func() {
		if newManagementClusterProxy != nil && newManagementClusterProxy.GetName() != input.BootstrapClusterProxy.GetName() {
			// Dump all the resources in the spec namespace and the workload cluster.
			// NOTE: In this case cleanup is deferred to the Janitor (otherwise it would require to pivot back to the bootstrap management cluster).
			framework.DumpAllResourcesAndLogs(ctx, newManagementClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, clusterResources.Cluster)
		}

		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
		cancelWatches()

		if !input.SkipCleanup {
			if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
				Eventually(func() error {
					return input.BootstrapClusterProxy.GetClient().Delete(ctx, &runtimev1.ExtensionConfig{ObjectMeta: metav1.ObjectMeta{Name: input.ExtensionConfigName}})
				}, 10*time.Second, 1*time.Second).Should(Succeed(), "Deleting ExtensionConfig failed")
			}
		}
	})
}

func forceDeleteClusterObjects(ctx context.Context, proxy framework.ClusterProxy, cluster *clusterv1.Cluster, objects ClusterObjects) error {
	objectsToDelete := []ctrlclient.Object{}

	for _, im := range objects.InfrastructureMachineByMachine {
		objectsToDelete = append(objectsToDelete, im)
	}
	for _, bc := range objects.BootstrapConfigByMachine {
		objectsToDelete = append(objectsToDelete, bc)
	}

	for _, it := range objects.InfrastructureMachinePoolByMachinePool {
		objectsToDelete = append(objectsToDelete, it)
	}
	for _, bt := range objects.BootstrapConfigByMachinePool {
		objectsToDelete = append(objectsToDelete, bt)
	}
	for _, mp := range objects.MachinePools {
		mp.Kind = "MachinePool" // adding for logging purposes.
		objectsToDelete = append(objectsToDelete, mp)
	}

	for _, l := range objects.MachinesByMachineSet {
		for _, m := range l {
			m.Kind = "Machine" // adding for logging purposes.
			objectsToDelete = append(objectsToDelete, m)
		}
	}
	for _, l := range objects.MachineSetsByMachineDeployment {
		for _, ms := range l {
			ms.Kind = "MachineSet" // adding for logging purposes.
			objectsToDelete = append(objectsToDelete, ms)
		}
	}
	for _, it := range objects.InfrastructureMachineTemplateByMachineDeployment {
		objectsToDelete = append(objectsToDelete, it)
	}
	for _, bt := range objects.BootstrapConfigTemplateByMachineDeployment {
		objectsToDelete = append(objectsToDelete, bt)
	}
	for _, md := range objects.MachineDeployments {
		md.Kind = "MachineDeployment" // adding for logging purposes.
		objectsToDelete = append(objectsToDelete, md)
	}

	for _, m := range objects.ControlPlaneMachines {
		m.Kind = "Machine" // adding for logging purposes.
		objectsToDelete = append(objectsToDelete, m)
	}
	objectsToDelete = append(objectsToDelete, objects.ControlPlaneInfrastructureMachineTemplate, objects.ControlPlane, objects.InfrastructureCluster)

	cluster.Kind = "Cluster" // adding for logging purposes.
	objectsToDelete = append(objectsToDelete, cluster)

	for _, obj := range objectsToDelete {
		log.Logf(" - %s %s", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj))
		if err := proxy.GetClient().Delete(ctx, obj); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to delete %s %s: %v", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj), err)
		}

		original := obj.DeepCopyObject().(ctrlclient.Object)
		obj.SetFinalizers([]string{})
		if err := proxy.GetClient().Patch(ctx, obj, ctrlclient.MergeFrom(original)); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("failed to remove finalizers from %s %s: %v", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj), err)
		}
	}

	var errs []error
	_ = wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
		errs = []error{}
		for _, obj := range objectsToDelete {
			if err := proxy.GetClient().Get(ctx, ctrlclient.ObjectKeyFromObject(obj), obj); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				if !apierrors.IsNotFound(err) {
					return false, fmt.Errorf("failed to get %s %s: %v", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj), err)
				}
				errs = append(errs, fmt.Errorf("%s %s still exist", obj.GetObjectKind().GroupVersionKind().Kind, klog.KObj(obj)))
			}
		}
		return len(errs) == 0, nil
	})
	return kerrors.NewAggregate(errs)
}
