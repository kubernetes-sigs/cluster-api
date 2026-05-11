/*
Copyright 2026 The Kubernetes Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

const (
	orphanLearnerNodeBlockPolicyName = "orphan-learner-block-node-create"
)

// KCPOrphanLearnerSpecInput is the input for KCPOrphanLearnerSpec.
type KCPOrphanLearnerSpecInput struct {
	// This spec requires the following intervals to be defined:
	// - wait-cluster, used when waiting for the cluster infrastructure to be provisioned.
	// - orphan-learner/wait-machines, used when waiting for control-plane Machines to appear or be remediated.
	// - orphan-learner/wait-etcd-learner, used when waiting for the second etcd member to appear as a learner.
	// - orphan-learner/wait-machine-deleted, used when waiting for the stuck Machine to be deleted by KCP.
	// - orphan-learner/check-orphan-stable, used when verifying the post-deletion etcd MemberList.
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// InfrastructureProviders specifies the infrastructure to use for clusterctl operations.
	// If not specified, the default provider is used.
	InfrastructureProvider *string

	// Flavor refers to the cluster template flavor. Defaults to "orphan-learner".
	Flavor *string

	// PostNamespaceCreated is an optional hook invoked after the spec namespace is created.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// KCPOrphanLearnerSpec reproduces the stuck-learner failure mode where KCP remediates a
// control-plane Machine whose Node never registered, leaving the corresponding etcd learner
// member orphaned in the cluster (see kubernetes-sigs/cluster-api#TBD). The reproduction
// installs a ValidatingAdmissionPolicy on the workload cluster that denies Node CREATE,
// then scales KCP up so the second Machine joins etcd as a learner but never registers a
// Node. After MHC-driven remediation deletes the Machine, KCP currently leaks the etcd
// member; the final assertion fails until the upstream fix lands.
//
// This spec is tagged [FAILING_TEST] for that reason — it is a regression gate, expected
// to fail on current main and to flip green when the leak is closed.
func KCPOrphanLearnerSpec(ctx context.Context, inputGetter func() KCPOrphanLearnerSpecInput) {
	var (
		specName         = "orphan-learner"
		input            KCPOrphanLearnerSpecInput
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

		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
	})

	It("[FAILING_TEST] Leaks an etcd learner when remediating a control-plane Machine whose Node never registered", func() {
		By("Creating a workload cluster with a single control-plane replica")

		clusterResources = createOrphanLearnerWorkloadCluster(ctx, createOrphanLearnerWorkloadClusterInput{
			E2EConfig:              input.E2EConfig,
			ClusterctlConfigPath:   input.ClusterctlConfigPath,
			Proxy:                  input.BootstrapClusterProxy,
			ArtifactFolder:         input.ArtifactFolder,
			SpecName:               specName,
			Flavor:                 ptr.Deref(input.Flavor, "orphan-learner"),
			InfrastructureProvider: input.InfrastructureProvider,
			Namespace:              namespace.Name,
		})

		By("Waiting for the first control-plane Machine to reach Running with a NodeRef")
		firstMachine := waitForOrphanLearnerFirstCP(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, input.E2EConfig.GetIntervals(specName, "wait-machines"))
		Expect(firstMachine.Status.NodeRef.IsDefined()).To(BeTrue(), "first control-plane Machine should have a NodeRef")
		firstCPNodeName := firstMachine.Status.NodeRef.Name
		log.Logf("First control-plane Machine %s registered Node %s", firstMachine.Name, firstCPNodeName)

		By("Installing a ValidatingAdmissionPolicy on the workload cluster that denies Node CREATE")
		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		installNodeCreateBlockPolicy(ctx, workloadProxy.GetClient())

		By("Scaling KubeadmControlPlane to 3 replicas so a second Machine attempts to join (KCP forbids even replica counts with stacked etcd; KCP serialises scale-up so only the second Machine is provisioned while the second one is stuck as a learner)")
		scaleKCP(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, 3)

		By("Waiting for the second control-plane Machine to appear")
		secondMachine := waitForOrphanLearnerSecondMachine(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, firstMachine.Name, input.E2EConfig.GetIntervals(specName, "wait-machines"))
		log.Logf("Second control-plane Machine %s observed without a NodeRef (Node creation is blocked)", secondMachine.Name)

		By("Waiting for the second etcd member to appear in MemberList with IsLearner=true")
		learnerMemberID, learnerMemberName := waitForOrphanLearnerEtcdMember(ctx, workloadProxy.GetRESTConfig(), firstCPNodeName, input.E2EConfig.GetIntervals(specName, "wait-etcd-learner"))
		log.Logf("Second control-plane Machine produced etcd learner ID=%x name=%q", learnerMemberID, learnerMemberName)

		By("Labelling the second Machine with mhc-test:fail to opt it into MHC remediation")
		labelMachineForOrphanLearnerRemediation(ctx, input.BootstrapClusterProxy.GetClient(), secondMachine)

		By(fmt.Sprintf("Waiting for the second Machine %s to be deleted by KCP remediation", secondMachine.Name))
		waitForMachineDeletion(ctx, input.BootstrapClusterProxy.GetClient(), secondMachine, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"))

		By("Asserting the orphan etcd learner is removed from MemberList after Machine deletion (fails until the upstream fix lands)")
		// Eventually rather than Consistently: the FIXED behaviour is "orphan goes away within X seconds".
		// The CURRENT behaviour is "orphan persists forever". This assertion fails until the fix lands.
		Eventually(func(g Gomega) {
			members, err := listEtcdMembers(ctx, workloadProxy.GetRESTConfig(), firstCPNodeName)
			g.Expect(err).NotTo(HaveOccurred())
			ids := make([]uint64, 0, len(members))
			for _, m := range members {
				ids = append(ids, m.ID)
			}
			g.Expect(ids).NotTo(ContainElement(learnerMemberID),
				"orphan etcd learner ID=%x name=%q persists in MemberList %v after Machine %s was deleted (this is the orphan-learner leak (#13667))",
				learnerMemberID, learnerMemberName, ids, secondMachine.Name)
		}, input.E2EConfig.GetIntervals(specName, "check-orphan-stable")...).Should(Succeed(),
			"Expected the orphan etcd learner to be removed once the Machine is deleted")
	})

	AfterEach(func() {
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

// --- helpers ---------------------------------------------------------------

type createOrphanLearnerWorkloadClusterInput struct {
	E2EConfig              *clusterctl.E2EConfig
	ClusterctlConfigPath   string
	Proxy                  framework.ClusterProxy
	ArtifactFolder         string
	SpecName               string
	Flavor                 string
	InfrastructureProvider *string
	Namespace              string
}

// createOrphanLearnerWorkloadCluster applies the orphan-learner template with a single control-plane
// replica and waits for the Cluster object to be discovered.
func createOrphanLearnerWorkloadCluster(ctx context.Context, input createOrphanLearnerWorkloadClusterInput) *clusterctl.ApplyClusterTemplateAndWaitResult {
	result := new(clusterctl.ApplyClusterTemplateAndWaitResult)

	infrastructureProvider := clusterctl.DefaultInfrastructureProvider
	if input.InfrastructureProvider != nil {
		infrastructureProvider = *input.InfrastructureProvider
	}
	clusterName := fmt.Sprintf("%s-%s", input.SpecName, util.RandomString(6))

	log.Logf("Rendering cluster template flavor=%s clusterName=%s", input.Flavor, clusterName)
	workloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
		ClusterctlConfigPath:     input.ClusterctlConfigPath,
		KubeconfigPath:           input.Proxy.GetKubeconfigPath(),
		Flavor:                   input.Flavor,
		Namespace:                input.Namespace,
		ClusterName:              clusterName,
		KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersion),
		ControlPlaneMachineCount: ptr.To[int64](1),
		WorkerMachineCount:       ptr.To[int64](0),
		InfrastructureProvider:   infrastructureProvider,
		LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.Proxy.GetName()),
	})
	Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to render the cluster template")

	Eventually(func() error {
		return input.Proxy.CreateOrUpdate(ctx, workloadClusterTemplate)
	}, 10*time.Second).Should(Succeed(), "Failed to apply the cluster template")

	log.Logf("Waiting for the cluster infrastructure to be provisioned")
	result.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
		Getter:    input.Proxy.GetClient(),
		Namespace: input.Namespace,
		Name:      clusterName,
	}, input.E2EConfig.GetIntervals(input.SpecName, "wait-cluster")...)

	return result
}

// waitForOrphanLearnerFirstCP waits for exactly one control-plane Machine to exist and to
// have a NodeRef set, then returns it.
func waitForOrphanLearnerFirstCP(ctx context.Context, c client.Client, namespace, clusterName string, intervals []interface{}) *clusterv1.Machine {
	var first *clusterv1.Machine
	Eventually(func(g Gomega) {
		machines := &clusterv1.MachineList{}
		g.Expect(c.List(ctx, machines,
			client.InNamespace(namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel:         clusterName,
				clusterv1.MachineControlPlaneLabel: "",
			},
		)).To(Succeed())
		g.Expect(machines.Items).To(HaveLen(1), "expected exactly one control-plane Machine")
		m := machines.Items[0]
		g.Expect(m.Status.NodeRef.IsDefined()).To(BeTrue(), "Machine %s does not yet have a NodeRef", m.Name)
		first = m.DeepCopy()
	}, intervals...).Should(Succeed())
	return first
}

// waitForOrphanLearnerSecondMachine waits for a second control-plane Machine (not the first)
// to be created. It does NOT wait for NodeRef — by design, the second Node never registers.
func waitForOrphanLearnerSecondMachine(ctx context.Context, c client.Client, namespace, clusterName, firstMachineName string, intervals []interface{}) *clusterv1.Machine {
	var second *clusterv1.Machine
	Eventually(func(g Gomega) {
		machines := &clusterv1.MachineList{}
		g.Expect(c.List(ctx, machines,
			client.InNamespace(namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel:         clusterName,
				clusterv1.MachineControlPlaneLabel: "",
			},
		)).To(Succeed())
		for i := range machines.Items {
			if machines.Items[i].Name != firstMachineName {
				second = machines.Items[i].DeepCopy()
				return
			}
		}
		g.Expect(second).NotTo(BeNil(), "second control-plane Machine has not yet been created")
	}, intervals...).Should(Succeed())
	return second
}

// scaleKCP patches the KubeadmControlPlane to the desired replica count.
func scaleKCP(ctx context.Context, c client.Client, namespace, clusterName string, replicas int32) {
	cluster := &clusterv1.Cluster{}
	Expect(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster)).To(Succeed())
	Expect(cluster.Spec.ControlPlaneRef.Kind).To(Equal("KubeadmControlPlane"), "this spec assumes a KCP-managed control plane")

	// Use an unstructured object to avoid coupling the test to the typed KCP API package.
	kcp := &unstructured.Unstructured{}
	kcp.SetGroupVersionKind(kcpGVK)
	kcp.SetNamespace(namespace)
	kcp.SetName(cluster.Spec.ControlPlaneRef.Name)

	Eventually(func() error {
		patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
		return c.Patch(ctx, kcp, client.RawPatch(types.MergePatchType, patch))
	}, 30*time.Second, 2*time.Second).Should(Succeed(), "failed to scale KubeadmControlPlane")
}

// kcpGVK is the GroupVersionKind for KubeadmControlPlane v1beta2.
var kcpGVK = schema.GroupVersionKind{
	Group:   "controlplane.cluster.x-k8s.io",
	Version: "v1beta2",
	Kind:    "KubeadmControlPlane",
}

// labelMachineForOrphanLearnerRemediation patches the Machine with the mhc-test:fail label
// so the MHC selects and remediates it.
func labelMachineForOrphanLearnerRemediation(ctx context.Context, c client.Client, m *clusterv1.Machine) {
	patched := m.DeepCopy()
	if patched.Labels == nil {
		patched.Labels = map[string]string{}
	}
	patched.Labels["mhc-test"] = failLabelValue
	Expect(c.Patch(ctx, patched, client.MergeFrom(m))).To(Succeed(), "failed to label Machine %s", m.Name)
}

// waitForMachineDeletion waits until the given Machine has been removed from the API server.
func waitForMachineDeletion(ctx context.Context, c client.Client, m *clusterv1.Machine, intervals []interface{}) {
	Eventually(func(g Gomega) {
		fresh := &clusterv1.Machine{}
		err := c.Get(ctx, client.ObjectKeyFromObject(m), fresh)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected Machine %s to be gone, got err=%v", m.Name, err)
	}, intervals...).Should(Succeed())
}

// installNodeCreateBlockPolicy installs a ValidatingAdmissionPolicy + Binding on the workload cluster
// that denies all Node CREATE requests. Existing Nodes are unaffected; CREATEs (e.g. from a fresh
// kubelet on a joining control-plane container) are rejected. This synthesises the orphan-learner trigger: kubeadm-join's
// MemberAdd succeeds (it goes against etcd directly), but the Node never registers via the kube-apiserver.
func installNodeCreateBlockPolicy(ctx context.Context, c client.Client) {
	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: orphanLearnerNodeBlockPolicyName},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: ptr.To(admissionregistrationv1.Fail),
			MatchConstraints: &admissionregistrationv1.MatchResources{
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1.RuleWithOperations{
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"nodes"},
						},
					},
				}},
			},
			Validations: []admissionregistrationv1.Validation{{
				Expression: "false",
				Message:    "Node creation blocked by orphan-learner e2e ValidatingAdmissionPolicy",
			}},
		},
	}
	Expect(c.Create(ctx, policy)).To(Succeed(), "failed to create ValidatingAdmissionPolicy")

	binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: orphanLearnerNodeBlockPolicyName},
		Spec: admissionregistrationv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName:        orphanLearnerNodeBlockPolicyName,
			ValidationActions: []admissionregistrationv1.ValidationAction{admissionregistrationv1.Deny},
		},
	}
	Expect(c.Create(ctx, binding)).To(Succeed(), "failed to create ValidatingAdmissionPolicyBinding")
}

// etcdMember is the subset of `etcdctl member list -w json` output we care about.
type etcdMember struct {
	ID        uint64   `json:"ID"`
	Name      string   `json:"name"`
	PeerURLs  []string `json:"peerURLs"`
	IsLearner bool     `json:"isLearner"`
}

type etcdMemberList struct {
	Members []etcdMember `json:"members"`
}

// listEtcdMembers exec-s into the etcd static pod on the named control-plane Node and runs
// `etcdctl member list -w json`. The first CP node is always a healthy voter at the points
// the test calls this helper.
func listEtcdMembers(ctx context.Context, restCfg *rest.Config, cpNodeName string) ([]etcdMember, error) {
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}
	podName := fmt.Sprintf("etcd-%s", cpNodeName)
	cmd := []string{
		"etcdctl",
		"--endpoints=https://127.0.0.1:2379",
		"--cacert=/etc/kubernetes/pki/etcd/ca.crt",
		"--cert=/etc/kubernetes/pki/etcd/server.crt",
		"--key=/etc/kubernetes/pki/etcd/server.key",
		"member", "list", "-w", "json",
	}
	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(metav1.NamespaceSystem).
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "etcd",
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(restCfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("build SPDY executor: %w", err)
	}
	var stdout, stderr bytes.Buffer
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		return nil, fmt.Errorf("etcdctl member list failed: %w (stderr=%s)", err, stderr.String())
	}
	var out etcdMemberList
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("parse etcdctl JSON output (stdout=%q): %w", stdout.String(), err)
	}
	return out.Members, nil
}

// waitForOrphanLearnerEtcdMember polls etcdctl until it observes a member with IsLearner=true
// that is not the first CP. Returns the learner's ID and name.
func waitForOrphanLearnerEtcdMember(ctx context.Context, restCfg *rest.Config, firstCPNodeName string, intervals []interface{}) (uint64, string) {
	var (
		learnerID   uint64
		learnerName string
	)
	Eventually(func(g Gomega) {
		members, err := listEtcdMembers(ctx, restCfg, firstCPNodeName)
		g.Expect(err).NotTo(HaveOccurred(), "failed to list etcd members")
		for _, m := range members {
			if m.IsLearner {
				learnerID = m.ID
				learnerName = m.Name
				return
			}
		}
		g.Expect(learnerID).NotTo(BeZero(), "no learner observed yet; members=%+v", members)
	}, intervals...).Should(Succeed())
	return learnerID, learnerName
}
