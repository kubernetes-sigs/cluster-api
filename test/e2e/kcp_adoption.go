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
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/utils/pointer"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// KCPAdoptionSpecInput is the input for KCPAdoptionSpec.
type KCPAdoptionSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// Flavor, if specified, must refer to a template that is
	// specially crafted with individual control plane machines
	// and a KubeadmControlPlane resource configured for adoption.
	// The initial Cluster, InfraCluster, Machine, InfraMachine,
	// KubeadmConfig, and any other resources that should exist
	// prior to adoption must have the kcp-adoption.step1: "" label
	// applied to them. The updated Cluster (with controlPlaneRef
	// configured), InfraMachineTemplate, and KubeadmControlPlane
	// resources must have the kcp-adoption.step2: "" applied to them.
	// If not specified, "kcp-adoption" is used.
	Flavor *string
}

type ClusterProxy interface {
	framework.ClusterProxy

	ApplyWithArgs(context.Context, []byte, ...string) error
}

// KCPAdoptionSpec implements a test that verifies KCP to properly adopt existing control plane Machines.
func KCPAdoptionSpec(ctx context.Context, inputGetter func() KCPAdoptionSpecInput) {
	var (
		specName      = "kcp-adoption"
		input         KCPAdoptionSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		cluster       *clusterv1.Cluster
		replicas      = pointer.Int64Ptr(1)
	)

	SetDefaultEventuallyTimeout(15 * time.Minute)
	SetDefaultEventuallyPollingInterval(10 * time.Second)

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

	It("Should adopt up-to-date control plane Machines without modification", func() {
		By("Creating a workload cluster")

		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		client := input.BootstrapClusterProxy.GetClient()
		WaitForClusterIntervals := input.E2EConfig.GetIntervals(specName, "wait-cluster")
		WaitForControlPlaneIntervals := input.E2EConfig.GetIntervals(specName, "wait-control-plane")

		workloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: input.BootstrapClusterProxy.GetKubeconfigPath(),
			// pass the clusterctl config file that points to the local provider repository created for this test,
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			// select template
			Flavor: pointer.StringDeref(input.Flavor, "kcp-adoption"),
			// define template variables
			Namespace:                namespace.Name,
			ClusterName:              clusterName,
			KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
			InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
			ControlPlaneMachineCount: replicas,
			WorkerMachineCount:       pointer.Int64Ptr(0),
			// setup clusterctl logs folder
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
		})
		Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

		By("Applying the cluster template yaml to the cluster with the 'initial' selector")
		Expect(input.BootstrapClusterProxy.Apply(ctx, workloadClusterTemplate, "--selector", "kcp-adoption.step1")).ShouldNot(HaveOccurred())

		cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
			Getter:    client,
			Namespace: namespace.Name,
			Name:      clusterName,
		}, WaitForClusterIntervals...)

		framework.WaitForClusterMachineNodeRefs(ctx, framework.WaitForClusterMachineNodeRefsInput{
			GetLister: client,
			Cluster:   cluster,
		}, WaitForControlPlaneIntervals...)

		workloadCluster := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name)
		framework.WaitForClusterMachinesReady(ctx, framework.WaitForClusterMachinesReadyInput{
			GetLister:  input.BootstrapClusterProxy.GetClient(),
			NodeGetter: workloadCluster.GetClient(),
			Cluster:    cluster,
		}, WaitForControlPlaneIntervals...)

		By("Applying the cluster template yaml to the cluster with the 'kcp' selector")
		Expect(input.BootstrapClusterProxy.Apply(ctx, workloadClusterTemplate, "--selector", "kcp-adoption.step2")).ShouldNot(HaveOccurred())

		var controlPlane *controlplanev1.KubeadmControlPlane
		Eventually(func() *controlplanev1.KubeadmControlPlane {
			controlPlane = framework.GetKubeadmControlPlaneByCluster(ctx, framework.GetKubeadmControlPlaneByClusterInput{
				Lister:      client,
				ClusterName: clusterName,
				Namespace:   namespace.Name,
			})
			return controlPlane
		}, "5s", "100ms").ShouldNot(BeNil())

		framework.WaitForControlPlaneToBeUpToDate(ctx, framework.WaitForControlPlaneToBeUpToDateInput{
			Getter:       client,
			ControlPlane: controlPlane,
		})

		By("Taking stable ownership of the Machines")
		must := func(r *labels.Requirement, err error) labels.Requirement {
			if err != nil {
				panic(err)
			}
			return *r
		}
		machines := clusterv1.MachineList{}
		Expect(client.List(ctx, &machines,
			ctrlclient.InNamespace(namespace.Name),
			ctrlclient.MatchingLabelsSelector{
				Selector: labels.NewSelector().
					Add(must(labels.NewRequirement(clusterv1.MachineControlPlaneLabelName, selection.Exists, []string{}))).
					Add(must(labels.NewRequirement(clusterv1.ClusterLabelName, selection.Equals, []string{clusterName}))),
			},
		)).To(Succeed())

		for _, m := range machines.Items {
			m := m
			Expect(&m).To(HaveControllerRef(framework.ObjectToKind(controlPlane), controlPlane))
			// TODO there is a missing unit test here
			Expect(m.CreationTimestamp.Time).To(BeTemporally("<", controlPlane.CreationTimestamp.Time),
				"The KCP has replaced the control plane machines after adopting them. "+
					"This may have occurred as a result of changes to the KubeadmConfig bootstrap type or reconciler. "+
					"In that case it's possible new defaulting or reconciliation logic made the KCP unable to recognize "+
					"a KubeadmConfig that it should have. "+
					"See ./bootstrap/kubeadm/api/equality/semantic.go and ensure that any new defaults are un-set so the KCP "+
					"can accurately 'guess' whether its template might have created the object.",
			)
		}
		Expect(machines.Items).To(HaveLen(int(*replicas)))

		bootstrap := bootstrapv1.KubeadmConfigList{}
		Expect(client.List(ctx, &bootstrap,
			ctrlclient.InNamespace(namespace.Name),
			ctrlclient.MatchingLabels{
				clusterv1.ClusterLabelName: clusterName,
			})).To(Succeed())

		By("Taking ownership of the cluster's PKI material")
		secrets := corev1.SecretList{}
		Expect(client.List(ctx, &secrets, ctrlclient.InNamespace(namespace.Name), ctrlclient.MatchingLabels{
			clusterv1.ClusterLabelName: cluster.Name,
		})).To(Succeed())

		bootstrapSecrets := map[string]bootstrapv1.KubeadmConfig{}
		for _, b := range bootstrap.Items {
			if b.Status.DataSecretName == nil {
				continue
			}
			bootstrapSecrets[*b.Status.DataSecretName] = b
		}

		for _, s := range secrets.Items {
			s := s
			// We don't check the data, and removing it from the object makes assertions much easier to read
			s.Data = nil

			// The bootstrap secret should still be owned by the bootstrap config so its cleaned up properly,
			// but the cluster PKI materials should have their ownership transferred.
			bootstrap, found := bootstrapSecrets[s.Name]
			switch {
			case strings.HasSuffix(s.Name, "-kubeconfig"):
				// Do nothing
			case found:
				Expect(&s).To(HaveControllerRef(framework.ObjectToKind(&bootstrap), &bootstrap))
			default:
				Expect(&s).To(HaveControllerRef(framework.ObjectToKind(controlPlane), controlPlane))
			}
		}
		Expect(secrets.Items).To(HaveLen(4 /* pki */ + 1 /* kubeconfig */ + int(*replicas)))

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
