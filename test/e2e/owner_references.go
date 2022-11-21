package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// OwnerReferencesInputSpec is the input for OwnerReferencesSpec.
type OwnerReferencesInputSpec struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// Flavor, if specified is the template flavor used to create the cluster for testing.
	// If not specified, and the e2econfig variable IPFamily is IPV6, then "ipv6" is used,
	// otherwise the default flavor is used.
	Flavor *string
}

// OwnerReferencesSpec implements a spec that mimics the operation described in the Cluster API owner reference test.
func OwnerReferencesSpec(ctx context.Context, inputGetter func() OwnerReferencesInputSpec) {
	var (
		specName         = "owner-references"
		input            OwnerReferencesInputSpec
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
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster", func() {
		By("Creating a workload cluster")

		flavor := clusterctl.DefaultFlavor
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   flavor,
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

		toUML(namespace.Name, filepath.Join(input.ArtifactFolder, fmt.Sprintf("%s-before.plantuml", namespace.Name)))
		// Check that the references are as expected on cluster creation.
		framework.AssertOwnerReferences(namespace.Name,
			framework.CoreTypeOwnerReferenceAssertion,
			framework.ExpOwnerReferenceAssertions,
			framework.DockerInfraOwnerReferenceAssertions,
			framework.KubeadmBootstrapOwnerReferenceAssertions,
			framework.KubeadmControlPlaneOwnerReferenceAssertions,
			framework.SecretOwnerReferenceAssertions,
			framework.ConfigMapReferenceAssertions)

		Expect(framework.RemoveOwnerReferences(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name)).To(Succeed())

		// Wait after removing references and unpausing to allow reconcilers to replace the ownerReferences.
		time.Sleep(20 * time.Second)

		toUML(namespace.Name, filepath.Join(input.ArtifactFolder, fmt.Sprintf("%s-after.plantuml", namespace.Name)))

		// Check if the ownerReferences are as expected once the cluster reconciles again.
		framework.AssertOwnerReferences(namespace.Name,
			framework.CoreTypeOwnerReferenceAssertion,
			framework.ExpOwnerReferenceAssertions,
			framework.DockerInfraOwnerReferenceAssertions,
			framework.KubeadmBootstrapOwnerReferenceAssertions,
			framework.KubeadmControlPlaneOwnerReferenceAssertions,
			framework.SecretOwnerReferenceAssertions,
			framework.ConfigMapReferenceAssertions)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

type object struct {
	apiVersion string
	kind       string
	name       string
	owns       []object
}

func toUML(namespace, filename string) {
	graph, err := clusterctlcluster.GetOwnerGraph(namespace)
	if err != nil {
		panic(err)
	}
	umlfile := "@startuml\n"
	i := 0
	nameToID := map[string]int{}
	for _, v := range graph {
		i++
		objectName := fmt.Sprintf("\"%s/%s\"", v.Object.Kind, v.Object.Name)
		nameToID[objectName] = i
		umlfile = fmt.Sprintf("%s%s\n", umlfile,
			fmt.Sprintf("object %s as %d", objectName, i))
		umlfile = fmt.Sprintf("%s%s\n", umlfile,
			fmt.Sprintf("%d : apiVersion : %s\n%d : kind : %s\n%d : name : %s\n%d : uid : %s\n",
				i, v.Object.APIVersion,
				i, v.Object.Kind,
				i, v.Object.Name,
				i, v.Object.UID,
			))
	}
	for _, v := range graph {
		objectName := fmt.Sprintf("\"%s/%s\"", v.Object.Kind, v.Object.Name)
		for _, owner := range v.Owners {
			ownerName := fmt.Sprintf("\"%s/%s\"", owner.Kind, owner.Name)
			umlfile = fmt.Sprintf("%s%d --> %d\n", umlfile, nameToID[ownerName], nameToID[objectName])
		}
	}
	umlfile = fmt.Sprintf("%s@enduml", umlfile)

	err = os.WriteFile(fmt.Sprintf(filename), []byte(umlfile), 0600)

}
