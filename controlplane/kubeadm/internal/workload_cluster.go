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

package internal

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"sigs.k8s.io/cluster-api/util/conditions"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeProxyKey              = "kube-proxy"
	kubeadmConfigKey          = "kubeadm-config"
	labelNodeRoleControlPlane = "node-role.kubernetes.io/master"
)

var (
	ErrControlPlaneMinNodes = errors.New("cluster has fewer than 2 control plane nodes; removing an etcd member is not supported")
)

// Common control-plane pod name prefixes
const (
	KubeAPIServerPodNamePrefix         = "kube-apiserver"
	KubeControllerManagerPodNamePrefix = "kube-controller-manager"
	KubeSchedulerHealthyPodNamePrefix  = "kube-scheduler"
	EtcdPodNamePrefix                  = "etcd"
)

type ConditionReason string

// GetSeverity returns the default severity for the condition reasons for the conditions owned by KCP.
func (c ConditionReason) GetSeverity() clusterv1.ConditionSeverity {
	switch {
	case c == ConditionReason(clusterv1.PodProvisioningReason):
		return clusterv1.ConditionSeverityInfo
	case c == ConditionReason(clusterv1.PodMissingReason):
		return clusterv1.ConditionSeverityWarning
	case c == ConditionReason(clusterv1.PodFailedReason):
		return clusterv1.ConditionSeverityError
	case c == ConditionReason(clusterv1.EtcdMemberUnhealthyReason):
		return clusterv1.ConditionSeverityError
	case c == ConditionReason(controlplanev1.EtcdClusterUnhealthyReason):
		return clusterv1.ConditionSeverityWarning
	default:
		return clusterv1.ConditionSeverityInfo
	}
}

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster
type WorkloadCluster interface {
	// Basic health and status checks.
	ClusterStatus(ctx context.Context) (ClusterStatus, error)
	ControlPlaneIsHealthy(ctx context.Context, controlPlane *ControlPlane) (HealthCheckResult, error)
	EtcdIsHealthy(ctx context.Context, controlPlane *ControlPlane) (HealthCheckResult, error)

	// Upgrade related tasks.
	ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error
	ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error
	UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error
	UpdateImageRepositoryInKubeadmConfigMap(ctx context.Context, imageRepository string) error
	UpdateEtcdVersionInKubeadmConfigMap(ctx context.Context, imageRepository, imageTag string) error
	UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error
	UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error
	UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error
	RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine) error
	RemoveNodeFromKubeadmConfigMap(ctx context.Context, nodeName string) error
	ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error
	AllowBootstrapTokensToGetNodes(ctx context.Context) error

	// State recovery tasks.
	ReconcileEtcdMembers(ctx context.Context) error
}

// Workload defines operations on workload clusters.
type Workload struct {
	Client              ctrlclient.Client
	CoreDNSMigrator     coreDNSMigrator
	etcdClientGenerator etcdClientFor
}

func (w *Workload) getControlPlaneNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	labels := map[string]string{
		labelNodeRoleControlPlane: "",
	}

	if err := w.Client.List(ctx, nodes, ctrlclient.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (w *Workload) getConfigMap(ctx context.Context, configMap ctrlclient.ObjectKey) (*corev1.ConfigMap, error) {
	original := &corev1.ConfigMap{}
	if err := w.Client.Get(ctx, configMap, original); err != nil {
		return nil, errors.Wrapf(err, "error getting %s/%s configmap from target cluster", configMap.Namespace, configMap.Name)
	}
	return original.DeepCopy(), nil
}

// HealthCheckResult maps nodes that are checked to any errors the node has related to the check.
type HealthCheckResult map[string]error

// controlPlaneIsHealthy does a best effort check of the control plane components the kubeadm control plane cares about.
// The return map is a map of node names as keys to error that that node encountered.
// All nodes will exist in the map with nil errors if there were no errors for that node.
func (w *Workload) ControlPlaneIsHealthy(ctx context.Context, controlPlane *ControlPlane) (HealthCheckResult, error) {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, err
	}

	response := make(map[string]error)
	for _, node := range controlPlaneNodes.Items {
		name := node.Name
		response[name] = nil

		var owningMachine *clusterv1.Machine = nil
		// If there is no owning Machine for the node, setting conditions are no-op.
		for _, machine := range controlPlane.Machines {
			if machine.Status.NodeRef == nil {
				continue
			}
			if machine.Status.NodeRef.Name == node.Name {
				owningMachine = machine
			}
		}

		// If there is any problem with the checks below, set response to any one of the problems.
		var anyError error = nil
		if err := checkNodeNoExecuteCondition(node); err != nil {
			anyError = err
		}

		// Check kube-api-server health
		if _, err = w.checkPodStatusAndUpdateCondition(controlPlane, KubeAPIServerPodNamePrefix, node, owningMachine, clusterv1.MachineAPIServerPodHealthyCondition); err != nil {
			anyError = err
		}

		// Check kube-controller-manager health
		if _, err = w.checkPodStatusAndUpdateCondition(controlPlane, KubeControllerManagerPodNamePrefix, node, owningMachine, clusterv1.MachineControllerManagerHealthyCondition); err != nil {
			anyError = err
		}

		// Check kube-scheduler health
		if _, err = w.checkPodStatusAndUpdateCondition(controlPlane, KubeSchedulerHealthyPodNamePrefix, node, owningMachine, clusterv1.MachineSchedulerPodHealthyCondition); err != nil {
			anyError = err
		}

		if anyError != nil {
			response[name] = err
			continue
		}
	}

	return response, nil
}

// checkPodStatusAndUpdateConditions returns true, only if the Pod is in Running/Ready condition.
// There is a slight difference between returning false and error:
// checkPodStatusAndUpdateConditions returns false, if Pod is not in Ready Condition or there is a client error.
// checkPodStatusAndUpdateConditions returns error only when it is obvious that the Pod is not in Running/Ready state, so it does not return error on transient client errors.
// For instance, returning (false, nil) means Pod is not ready but is not in error state as well,
// returning (false, err) means Pod is not ready and it is in error state as well.
func (w *Workload) checkPodStatusAndUpdateCondition(controlPlane *ControlPlane, staticPodPrefix string, node corev1.Node, owningMachine *clusterv1.Machine, conditionType clusterv1.ConditionType) (bool, error) {
	staticPodKey := ctrlclient.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      staticPodName(staticPodPrefix, node.Name),
	}

	staticPod := corev1.Pod{}
	if err := w.Client.Get(context.TODO(), staticPodKey, &staticPod); err != nil {
		// If there is an error getting the Pod, do not set any conditions.
		if apierrors.IsNotFound(err) {
			if owningMachine != nil {
				if _, ok := controlPlane.MachineConditions[owningMachine.Name]; ok {
					controlPlane.MachineConditions[owningMachine.Name][conditionType] = conditions.FalseCondition(conditionType, clusterv1.PodMissingReason, ConditionReason(clusterv1.PodMissingReason).GetSeverity(), "")
				}
			}
			return false, errors.Errorf("static pod %s is missing", staticPodPrefix)
		}
		// Do not return error here, because this may be a transient error.
		return false, nil
	}

	if err := checkStaticPodReadyCondition(staticPod); err != nil {
		if checkStaticPodFailedPhase(staticPod) {
			if owningMachine != nil {
				controlPlane.MachineConditions[owningMachine.Name][conditionType] = conditions.FalseCondition(conditionType, clusterv1.PodFailedReason, ConditionReason(clusterv1.PodFailedReason).GetSeverity(), "")
			}
			return false, errors.Errorf("static pod %s is failed", staticPodPrefix)
		}

		// Check if the Pod is in Pending state
		if checkStaticPodProvisioning(staticPod) {
			if owningMachine != nil {
				controlPlane.MachineConditions[owningMachine.Name][conditionType] = conditions.FalseCondition(conditionType, clusterv1.PodProvisioningReason, ConditionReason(clusterv1.PodProvisioningReason).GetSeverity(), "")
			}
			// This is not an error case.
			return false, nil
		}

		// If Pod is not provisioning state, check if still in Pending Phase.
		podProvisioningState := checkStaticPodAfterProvisioningState(staticPod)
		// Non-nil podProvisioningState means there is at least one container in waiting state.
		if podProvisioningState != "" {
			if owningMachine != nil {
				controlPlane.MachineConditions[owningMachine.Name][conditionType] = conditions.FalseCondition(conditionType, clusterv1.PodFailedReason, ConditionReason(clusterv1.PodFailedReason).GetSeverity(), "Pod is provisioned but not ready.")
			}
			return false, errors.Errorf("static pod %s is provisioned but still is not ready", staticPodPrefix)
		}

		return false, errors.Errorf("static pod %s is not ready", staticPodPrefix)
	}

	if owningMachine != nil {
		controlPlane.MachineConditions[owningMachine.Name][conditionType] = conditions.TrueCondition(conditionType)
	}
	return true, nil
}

// UpdateKubernetesVersionInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) UpdateImageRepositoryInKubeadmConfigMap(ctx context.Context, imageRepository string) error {
	configMapKey := ctrlclient.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}
	kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
	if err != nil {
		return err
	}
	config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
	if err := config.UpdateImageRepository(imageRepository); err != nil {
		return err
	}
	if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
		return errors.Wrap(err, "error updating kubeadm ConfigMap")
	}
	return nil
}

// UpdateKubernetesVersionInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error {
	configMapKey := ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}
	kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
	if err != nil {
		return err
	}
	config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
	if err := config.UpdateKubernetesVersion(fmt.Sprintf("v%s", version)); err != nil {
		return err
	}
	if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
		return errors.Wrap(err, "error updating kubeadm ConfigMap")
	}
	return nil
}

// UpdateKubeletConfigMap will create a new kubelet-config-1.x config map for a new version of the kubelet.
// This is a necessary process for upgrades.
func (w *Workload) UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error {
	// Check if the desired configmap already exists
	desiredKubeletConfigMapName := fmt.Sprintf("kubelet-config-%d.%d", version.Major, version.Minor)
	configMapKey := ctrlclient.ObjectKey{Name: desiredKubeletConfigMapName, Namespace: metav1.NamespaceSystem}
	_, err := w.getConfigMap(ctx, configMapKey)
	if err == nil {
		// Nothing to do, the configmap already exists
		return nil
	}
	if !apierrors.IsNotFound(errors.Cause(err)) {
		return errors.Wrapf(err, "error determining if kubelet configmap %s exists", desiredKubeletConfigMapName)
	}

	previousMinorVersionKubeletConfigMapName := fmt.Sprintf("kubelet-config-%d.%d", version.Major, version.Minor-1)
	configMapKey = ctrlclient.ObjectKey{Name: previousMinorVersionKubeletConfigMapName, Namespace: metav1.NamespaceSystem}
	// Returns a copy
	cm, err := w.getConfigMap(ctx, configMapKey)
	if apierrors.IsNotFound(errors.Cause(err)) {
		return errors.Errorf("unable to find kubelet configmap %s", previousMinorVersionKubeletConfigMapName)
	}
	if err != nil {
		return err
	}

	// Update the name to the new name
	cm.Name = desiredKubeletConfigMapName
	// Clear the resource version. Is this necessary since this cm is actually a DeepCopy()?
	cm.ResourceVersion = ""

	if err := w.Client.Create(ctx, cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "error creating configmap %s", desiredKubeletConfigMapName)
	}
	return nil
}

// RemoveMachineFromKubeadmConfigMap removes the entry for the machine from the kubeadm configmap.
func (w *Workload) RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		// Nothing to do, no node for Machine
		return nil
	}

	return w.RemoveNodeFromKubeadmConfigMap(ctx, machine.Status.NodeRef.Name)
}

// RemoveNodeFromKubeadmConfigMap removes the entry for the node from the kubeadm configmap.
func (w *Workload) RemoveNodeFromKubeadmConfigMap(ctx context.Context, name string) error {
	return util.Retry(func() (bool, error) {
		configMapKey := ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}
		kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
		if err != nil {
			Log.Error(err, "unable to get kubeadmConfigMap")
			return false, nil
		}
		config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
		if err := config.RemoveAPIEndpoint(name); err != nil {
			return false, err
		}
		if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
			Log.Error(err, "error updating kubeadm ConfigMap")
			return false, nil
		}
		return true, nil
	}, 5)
}

// ClusterStatus holds stats information about the cluster.
type ClusterStatus struct {
	// Nodes are a total count of nodes
	Nodes int32
	// ReadyNodes are the count of nodes that are reporting ready
	ReadyNodes int32
	// HasKubeadmConfig will be true if the kubeadm config map has been uploaded, false otherwise.
	HasKubeadmConfig bool
}

// ClusterStatus returns the status of the cluster.
func (w *Workload) ClusterStatus(ctx context.Context) (ClusterStatus, error) {
	status := ClusterStatus{}

	// count the control plane nodes
	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return status, err
	}

	for _, node := range nodes.Items {
		nodeCopy := node
		status.Nodes++
		if util.IsNodeReady(&nodeCopy) {
			status.ReadyNodes++
		}
	}

	// find the kubeadm conifg
	key := ctrlclient.ObjectKey{
		Name:      kubeadmConfigKey,
		Namespace: metav1.NamespaceSystem,
	}
	err = w.Client.Get(ctx, key, &corev1.ConfigMap{})
	// TODO: Consider if this should only return false if the error is IsNotFound.
	// TODO: Consider adding a third state of 'unknown' when there is an error retrieving the config map.
	status.HasKubeadmConfig = err == nil
	return status, nil
}

func generateClientCert(caCertEncoded, caKeyEncoded []byte) (tls.Certificate, error) {
	privKey, err := certs.NewPrivateKey()
	if err != nil {
		return tls.Certificate{}, err
	}
	caCert, err := certs.DecodeCertPEM(caCertEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}
	caKey, err := certs.DecodePrivateKeyPEM(caKeyEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}
	x509Cert, err := newClientCert(caCert, privKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certs.EncodeCertPEM(x509Cert), certs.EncodePrivateKeyPEM(privKey))
}

func newClientCert(caCert *x509.Certificate, key *rsa.PrivateKey, caKey crypto.Signer) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "cluster-api.x-k8s.io",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:   now.Add(time.Minute * -5),
		NotAfter:    now.Add(time.Hour * 24 * 365 * 10), // 10 years
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create signed client certificate: %+v", tmpl)
	}

	c, err := x509.ParseCertificate(b)
	return c, errors.WithStack(err)
}

func staticPodName(component, nodeName string) string {
	return fmt.Sprintf("%s-%s", component, nodeName)
}

func checkStaticPodReadyCondition(pod corev1.Pod) error {
	found := false
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			found = true
		}
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
			return errors.Errorf("static pod %s/%s is not ready", pod.Namespace, pod.Name)
		}
	}

	// TODO: Remove this as static pods have ready condition.
	if !found {
		return errors.Errorf("pod does not have ready condition: %v", pod.Name)
	}
	return nil
}

func checkStaticPodFailedPhase(pod corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed
}

// If Pod is in Pending phase and PodScheduled or Initialized condition is set to false, then pod is in provisioning state.
func checkStaticPodProvisioning(pod corev1.Pod) bool {
	// Pod being not in Pending phase means it has already provisioned and running or failed or in unknown phase.
	if pod.Status.Phase != corev1.PodPending {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status != corev1.ConditionTrue {
			return true
		}
		if condition.Type == corev1.PodInitialized && condition.Status != corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// If Pod is in Pending phase, there may be some failing containers in the pod.
// This function must be called when PodScheduled and Initialized condition is set to true. If a Pod is initialized but still in Pending state,
// returns non-nil string with the reason of the container in waiting state.
func checkStaticPodAfterProvisioningState(pod corev1.Pod) string {
	// Pod being not in Pending phase means it has already provisioned and running or failed or in unknown phase.
	if pod.Status.Phase != corev1.PodPending {
		return ""
	}

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			return containerStatus.State.Waiting.Reason
		}
	}
	return ""
}

func checkNodeNoExecuteCondition(node corev1.Node) error {
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			return errors.Errorf("node has NoExecute taint: %v", node.Name)
		}
	}
	return nil
}

// UpdateKubeProxyImageInfo updates kube-proxy image in the kube-proxy DaemonSet.
func (w *Workload) UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error {
	// Return early if we've been asked to skip kube-proxy upgrades entirely.
	if _, ok := kcp.Annotations[controlplanev1.SkipKubeProxyAnnotation]; ok {
		return nil
	}

	ds := &appsv1.DaemonSet{}

	if err := w.Client.Get(ctx, ctrlclient.ObjectKey{Name: kubeProxyKey, Namespace: metav1.NamespaceSystem}, ds); err != nil {
		if apierrors.IsNotFound(err) {
			// if kube-proxy is missing, return without errors
			return nil
		}
		return errors.Wrapf(err, "failed to determine if %s daemonset already exists", kubeProxyKey)
	}

	container := findKubeProxyContainer(ds)
	if container == nil {
		return nil
	}

	newImageName, err := containerutil.ModifyImageTag(container.Image, kcp.Spec.Version)
	if err != nil {
		return err
	}
	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil &&
		kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ImageRepository != "" {
		newImageName, err = containerutil.ModifyImageRepository(newImageName, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ImageRepository)
		if err != nil {
			return err
		}
	}

	if container.Image != newImageName {
		helper, err := patch.NewHelper(ds, w.Client)
		if err != nil {
			return err
		}
		patchKubeProxyImage(ds, newImageName)
		return helper.Patch(ctx, ds)
	}
	return nil
}

func findKubeProxyContainer(ds *appsv1.DaemonSet) *corev1.Container {
	containers := ds.Spec.Template.Spec.Containers
	for idx := range containers {
		if containers[idx].Name == kubeProxyKey {
			return &containers[idx]
		}
	}
	return nil
}

func patchKubeProxyImage(ds *appsv1.DaemonSet, image string) {
	containers := ds.Spec.Template.Spec.Containers
	for idx := range containers {
		if containers[idx].Name == kubeProxyKey {
			containers[idx].Image = image
		}
	}
}
