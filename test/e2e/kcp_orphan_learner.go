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
	"strings"
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

		By("Resolving a direct REST config to the first control-plane apiserver (bypasses the workload-cluster HAProxy LB so the orphan-stable assertion does not flake when the LB churns during remediation)")
		directCfg := resolveDirectAPIServerConfig(ctx, workloadProxy.GetRESTConfig(), firstCPNodeName)

		By("Scaling KubeadmControlPlane to 3 replicas so a second Machine attempts to join (KCP forbids even replica counts with stacked etcd; KCP serialises scale-up so only the second Machine is provisioned while the second one is stuck as a learner)")
		scaleKCP(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, 3)

		By("Waiting for the second control-plane Machine to appear")
		secondMachine := waitForOrphanLearnerSecondMachine(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, firstMachine.Name, input.E2EConfig.GetIntervals(specName, "wait-machines"))
		log.Logf("Second control-plane Machine %s observed without a NodeRef (Node creation is blocked)", secondMachine.Name)

		By("Waiting for the second etcd member to appear in MemberList with IsLearner=true")
		learnerMemberID, learnerMemberName := waitForOrphanLearnerEtcdMember(ctx, directCfg, firstCPNodeName, input.E2EConfig.GetIntervals(specName, "wait-etcd-learner"))
		log.Logf("Second control-plane Machine produced etcd learner ID=%x name=%q", learnerMemberID, learnerMemberName)
		logEtcdMembers(ctx, directCfg, firstCPNodeName, "after learner observed")
		logKCPEtcdCondition(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, "after learner observed")

		By("Labelling the second Machine with mhc-test:fail to opt it into MHC remediation")
		labelMachineForOrphanLearnerRemediation(ctx, input.BootstrapClusterProxy.GetClient(), secondMachine)
		logEtcdMembers(ctx, directCfg, firstCPNodeName, "after labelling for remediation")

		By(fmt.Sprintf("Waiting for the second Machine %s to be deleted by KCP remediation", secondMachine.Name))
		waitForMachineDeletion(ctx, input.BootstrapClusterProxy.GetClient(), secondMachine, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"))

		By("Pausing the Cluster so KCP does not create a replacement Machine that would add another learner during the orphan-stable assertion")
		pauseClusterForOrphanLearner(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name)
		logEtcdMembers(ctx, directCfg, firstCPNodeName, "immediately after Machine deletion + pause")
		logKCPEtcdCondition(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name, "immediately after Machine deletion + pause")

		By("Asserting KCP's EtcdClusterHealthy condition returns to True after Machine deletion (fails until the upstream fix lands)")
		// We poll KCP's own condition rather than running etcdctl ourselves: the orphan member
		// wedges the workload-cluster etcd / apiserver exec channel intermittently, but the
		// management-cluster API stays reliable. With the bug present, KCP reports either:
		//   * EtcdClusterHealthy=False with message "Etcd member <X> does not have a corresponding Machine"
		//   * EtcdClusterHealthy=Unknown with message "Failed to connect to etcd: ... context deadline exceeded"
		// Both are user-visible symptoms of orphan-learner. The fixed behaviour is EtcdClusterHealthy=True
		// within the orphan-stable interval after Machine deletion.
		// We also log the etcd MemberList best-effort each iteration so the trace carries direct
		// evidence when the workload apiserver happens to be reachable.
		Eventually(func(g Gomega) {
			logEtcdMembers(ctx, directCfg, firstCPNodeName, "during orphan-stable check")
			status, reason, message, err := kcpEtcdClusterHealthy(ctx, input.BootstrapClusterProxy.GetClient(), namespace.Name, clusterResources.Cluster.Name)
			g.Expect(err).NotTo(HaveOccurred(), "failed to read KCP EtcdClusterHealthy condition (will retry)")
			log.Logf("KCP EtcdClusterHealthy during orphan-stable check: status=%s reason=%s message=%q", status, reason, message)
			g.Expect(status).To(Equal("True"),
				"KCP EtcdClusterHealthy is %s (reason=%s) after Machine %s was deleted: %s — this is the orphan-learner leak (#13667) (orphan etcd member ID=%x name=%q for the deleted Machine persists in the workload-cluster etcd)",
				status, reason, secondMachine.Name, message, learnerMemberID, learnerMemberName)
		}, input.E2EConfig.GetIntervals(specName, "check-orphan-stable")...).Should(Succeed(),
			"Expected KCP EtcdClusterHealthy to return to True once Machine %s was deleted (orphan etcd member ID=%x leaked).", secondMachine.Name, learnerMemberID)
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

// resolveDirectAPIServerConfig builds a rest.Config that talks straight to the named control-plane
// node's kube-apiserver, bypassing the workload-cluster HAProxy LB. The LB churns during scale and
// remediation (briefly routing to dying or starting CPs), which causes the etcdctl exec channel to
// EOF intermittently. The first CP node is healthy throughout this spec, so addressing it directly
// makes etcd queries deterministic.
//
// We read the Node's InternalIP through the LB while it is still healthy (early in the spec), then
// rewrite the rest.Config Host. TLS is set to InsecureSkipVerify because the apiserver cert SAN may
// not include the per-Node IP literally; identity is not part of what this spec is testing.
func resolveDirectAPIServerConfig(ctx context.Context, lbCfg *rest.Config, cpNodeName string) *rest.Config {
	clientset, err := kubernetes.NewForConfig(lbCfg)
	Expect(err).NotTo(HaveOccurred(), "build clientset for direct apiserver resolution")

	var ip string
	Eventually(func(g Gomega) {
		node, getErr := clientset.CoreV1().Nodes().Get(ctx, cpNodeName, metav1.GetOptions{})
		g.Expect(getErr).NotTo(HaveOccurred())
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP && addr.Address != "" {
				ip = addr.Address
				return
			}
		}
		g.Expect(ip).NotTo(BeEmpty(), "Node %s has no InternalIP yet", cpNodeName)
	}, 60*time.Second, 5*time.Second).Should(Succeed(), "failed to resolve InternalIP for Node %s", cpNodeName)

	direct := rest.CopyConfig(lbCfg)
	direct.Host = fmt.Sprintf("https://%s:6443", ip)
	// Disable server CA verification because the apiserver cert SAN may not include the per-Node IP,
	// but preserve client cert/key/token from lbCfg so authentication still works. rest.Config's
	// Insecure flag is mutually exclusive with CAData/CAFile so clear those too.
	direct.TLSClientConfig.Insecure = true
	direct.TLSClientConfig.CAData = nil
	direct.TLSClientConfig.CAFile = ""
	direct.TLSClientConfig.ServerName = ""
	log.Logf("Direct apiserver REST config pinned to Node %s at %s", cpNodeName, direct.Host)
	return direct
}

// pauseClusterForOrphanLearner sets the cluster.x-k8s.io/paused annotation on the Cluster so
// neither KCP nor MHC reconciles further while the orphan-stable assertion runs. Without this,
// KCP would observe replicas=3 with one healthy Machine and immediately create a replacement,
// which the workload-cluster HAProxy LB would briefly route to and cause the apiserver exec
// channel (used by etcdctl) to flake.
func pauseClusterForOrphanLearner(ctx context.Context, c client.Client, namespace, clusterName string) {
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{%q:"true"}}}`, clusterv1.PausedAnnotation))
	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: clusterName}}
	Eventually(func() error {
		return c.Patch(ctx, cluster, client.RawPatch(types.MergePatchType, patch))
	}, 30*time.Second, 2*time.Second).Should(Succeed(), "failed to pause Cluster %s/%s", namespace, clusterName)
	log.Logf("Paused Cluster %s/%s (set %s=true)", namespace, clusterName, clusterv1.PausedAnnotation)
}

// logEtcdMembers fetches the etcd MemberList via etcdctl on the first CP node and logs the
// result with a caller-supplied label. Errors are logged but not asserted — this is purely a
// diagnostic so the trace contains evidence even when later assertions fail on transient
// exec/connectivity issues.
func logEtcdMembers(ctx context.Context, restCfg *rest.Config, cpNodeName, when string) {
	members, err := listEtcdMembers(ctx, restCfg, cpNodeName)
	if err != nil {
		log.Logf("etcd MemberList %s: failed to list (%v)", when, err)
		return
	}
	log.Logf("etcd MemberList %s: %s", when, formatEtcdMembers(members))
}

// kcpEtcdClusterHealthy reads the KubeadmControlPlane referenced by the Cluster and returns the
// status, reason and message of its EtcdClusterHealthy condition. Returns ("", "", "", err) if any
// API read fails; returns ("", "", "", nil) if the condition is not yet present.
func kcpEtcdClusterHealthy(ctx context.Context, c client.Client, namespace, clusterName string) (status, reason, message string, err error) {
	cluster := &clusterv1.Cluster{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clusterName}, cluster); err != nil {
		return "", "", "", fmt.Errorf("get Cluster %s/%s: %w", namespace, clusterName, err)
	}
	if cluster.Spec.ControlPlaneRef.Name == "" {
		return "", "", "", fmt.Errorf("Cluster %s/%s has no controlPlaneRef", namespace, clusterName)
	}
	kcp := &unstructured.Unstructured{}
	kcp.SetGroupVersionKind(kcpGVK)
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cluster.Spec.ControlPlaneRef.Name}, kcp); err != nil {
		return "", "", "", fmt.Errorf("get KCP %s/%s: %w", namespace, cluster.Spec.ControlPlaneRef.Name, err)
	}
	conditions, found, _ := unstructured.NestedSlice(kcp.Object, "status", "conditions")
	if !found {
		return "", "", "", nil
	}
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] != "EtcdClusterHealthy" {
			continue
		}
		status, _ = cond["status"].(string)
		reason, _ = cond["reason"].(string)
		message, _ = cond["message"].(string)
		return status, reason, message, nil
	}
	return "", "", "", nil
}

// logKCPEtcdCondition emits KCP's EtcdClusterHealthy condition for diagnostic checkpoints. Never
// fails the spec.
func logKCPEtcdCondition(ctx context.Context, c client.Client, namespace, clusterName, when string) {
	status, reason, message, err := kcpEtcdClusterHealthy(ctx, c, namespace, clusterName)
	if err != nil {
		log.Logf("KCP EtcdClusterHealthy %s: %v", when, err)
		return
	}
	if status == "" {
		log.Logf("KCP EtcdClusterHealthy %s: condition not present on KCP yet", when)
		return
	}
	log.Logf("KCP EtcdClusterHealthy %s: status=%s reason=%s message=%q", when, status, reason, message)
}

// formatEtcdMembers renders a member list as "ID=<hex> name=<name> isLearner=<bool> peerURLs=<...>; ..."
// for readable test logs.
func formatEtcdMembers(members []etcdMember) string {
	if len(members) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(members))
	for _, m := range members {
		parts = append(parts, fmt.Sprintf("{ID=%x name=%q isLearner=%v peerURLs=%v}", m.ID, m.Name, m.IsLearner, m.PeerURLs))
	}
	return "[" + strings.Join(parts, ", ") + "]"
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

// waitForOrphanLearnerEtcdMember polls etcdctl until it observes a second etcd member (i.e. any
// member whose name is not the first CP's Node name). It accepts both learner and voter states
// because KCP may have already promoted the new member by the time the first poll runs -- the
// orphan-after-remediation behaviour we're testing is the same either way. The promotion of a
// member whose corresponding Machine has no Node is itself a KCP bug worth flagging, but it is
// not what this spec asserts. Returns the second member's ID and name (or empty name if still
// a learner).
func waitForOrphanLearnerEtcdMember(ctx context.Context, restCfg *rest.Config, firstCPNodeName string, intervals []interface{}) (uint64, string) {
	var (
		secondID   uint64
		secondName string
	)
	Eventually(func(g Gomega) {
		members, err := listEtcdMembers(ctx, restCfg, firstCPNodeName)
		g.Expect(err).NotTo(HaveOccurred(), "failed to list etcd members")
		for _, m := range members {
			// Skip the first CP voter we already know about.
			if m.Name == firstCPNodeName {
				continue
			}
			secondID = m.ID
			secondName = m.Name
			log.Logf("Observed second etcd member: ID=%x name=%q isLearner=%v peerURLs=%v",
				m.ID, m.Name, m.IsLearner, m.PeerURLs)
			return
		}
		g.Expect(secondID).NotTo(BeZero(), "no second etcd member observed yet; members=%s", formatEtcdMembers(members))
	}, intervals...).Should(Succeed())
	return secondID, secondName
}
