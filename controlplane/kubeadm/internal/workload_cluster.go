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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdutil "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kubeProxyDaemonSetName = "kube-proxy"
)

var (
	ErrControlPlaneMinNodes = errors.New("cluster has fewer than 2 control plane nodes; removing an etcd member is not supported")
)

type etcdClientFor interface {
	forNode(name string) (*etcd.Client, error)
}

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster
type WorkloadCluster interface {
	// Basic health and status behaviors

	ClusterStatus(ctx context.Context) (ClusterStatus, error)
	ControlPlaneIsHealthy(ctx context.Context) (HealthCheckResult, error)
	EtcdIsHealthy(ctx context.Context) (HealthCheckResult, error)

	// Behaviors necessary for upgrade
	ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error
	ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error
	UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error
	UpdateEtcdVersionInKubeadmConfigMap(ctx context.Context, imageRepository, imageTag string) error
	UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error
	UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error
	UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error
	RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine) error
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
		"node-role.kubernetes.io/master": "",
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
func (w *Workload) ControlPlaneIsHealthy(ctx context.Context) (HealthCheckResult, error) {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, err
	}

	response := make(map[string]error)
	for _, node := range controlPlaneNodes.Items {
		name := node.Name
		response[name] = nil

		if err := checkNodeNoExecuteCondition(node); err != nil {
			response[name] = err
			continue
		}

		apiServerPodKey := ctrlclient.ObjectKey{
			Namespace: metav1.NamespaceSystem,
			Name:      staticPodName("kube-apiserver", name),
		}

		apiServerPod := &corev1.Pod{}
		if err := w.Client.Get(ctx, apiServerPodKey, apiServerPod); err != nil {
			response[name] = err
			continue
		}
		response[name] = checkStaticPodReadyCondition(apiServerPod)

		controllerManagerPodKey := ctrlclient.ObjectKey{
			Namespace: metav1.NamespaceSystem,
			Name:      staticPodName("kube-controller-manager", name),
		}
		controllerManagerPod := &corev1.Pod{}
		if err := w.Client.Get(ctx, controllerManagerPodKey, controllerManagerPod); err != nil {
			response[name] = err
			continue
		}
		response[name] = checkStaticPodReadyCondition(controllerManagerPod)
	}

	return response, nil
}

// removeMemberForNode removes the etcd member for the node. Removing the etcd
// member when the cluster has one control plane node is not supported. To allow
// the removal of a failed etcd member, the etcd API requests are sent to a
// different node.
func (w *Workload) removeMemberForNode(ctx context.Context, name string) error {
	// Pick a different node to talk to etcd
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return err
	}
	if len(controlPlaneNodes.Items) < 2 {
		return ErrControlPlaneMinNodes
	}
	anotherNode := firstNodeNotMatchingName(name, controlPlaneNodes.Items)
	if anotherNode == nil {
		return errors.Errorf("failed to find a control plane node whose name is not %s", name)
	}
	etcdClient, err := w.etcdClientGenerator.forNode(anotherNode.Name)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}

	// List etcd members. This checks that the member is healthy, because the request goes through consensus.
	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}
	member := etcdutil.MemberForName(members, name)

	// The member has already been removed, return immediately
	if member == nil {
		return nil
	}

	if err := etcdClient.RemoveMember(ctx, member.ID); err != nil {
		return errors.Wrap(err, "failed to remove member from etcd")
	}

	return nil
}

// EtcdIsHealthy runs checks for every etcd member in the cluster to satisfy our definition of healthy.
// This is a best effort check and nodes can become unhealthy after the check is complete. It is not a guarantee.
// It's used a signal for if we should allow a target cluster to scale up, scale down or upgrade.
// It returns a map of nodes checked along with an error for a given node.
func (w *Workload) EtcdIsHealthy(ctx context.Context) (HealthCheckResult, error) {
	var knownClusterID uint64
	var knownMemberIDSet etcdutil.UInt64Set

	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, err
	}

	response := make(map[string]error)
	for _, node := range controlPlaneNodes.Items {
		name := node.Name
		response[name] = nil
		if node.Spec.ProviderID == "" {
			response[name] = errors.New("empty provider ID")
			continue
		}

		// Create the etcd Client for the etcd Pod scheduled on the Node
		etcdClient, err := w.etcdClientGenerator.forNode(name)
		if err != nil {
			response[name] = errors.Wrap(err, "failed to create etcd client")
			continue
		}

		// List etcd members. This checks that the member is healthy, because the request goes through consensus.
		members, err := etcdClient.Members(ctx)
		if err != nil {
			response[name] = errors.Wrap(err, "failed to list etcd members using etcd client")
			continue
		}
		member := etcdutil.MemberForName(members, name)

		// Check that the member reports no alarms.
		if len(member.Alarms) > 0 {
			response[name] = errors.Errorf("etcd member reports alarms: %v", member.Alarms)
			continue
		}

		// Check that the member belongs to the same cluster as all other members.
		clusterID := member.ClusterID
		if knownClusterID == 0 {
			knownClusterID = clusterID
		} else if knownClusterID != clusterID {
			response[name] = errors.Errorf("etcd member has cluster ID %d, but all previously seen etcd members have cluster ID %d", clusterID, knownClusterID)
			continue
		}

		// Check that the member list is stable.
		memberIDSet := etcdutil.MemberIDSet(members)
		if knownMemberIDSet.Len() == 0 {
			knownMemberIDSet = memberIDSet
		} else {
			unknownMembers := memberIDSet.Difference(knownMemberIDSet)
			if unknownMembers.Len() > 0 {
				response[name] = errors.Errorf("etcd member reports members IDs %v, but all previously seen etcd members reported member IDs %v", memberIDSet.UnsortedList(), knownMemberIDSet.UnsortedList())
			}
			continue
		}
	}

	// Check that there is exactly one etcd member for every control plane machine.
	// There should be no etcd members added "out of band.""
	if len(controlPlaneNodes.Items) != len(knownMemberIDSet) {
		return response, errors.Errorf("there are %d control plane nodes, but %d etcd members", len(controlPlaneNodes.Items), len(knownMemberIDSet))
	}

	return response, nil
}

// UpdateEtcdVersionInKubeadmConfigMap sets the imageRepository or the imageTag or both in the kubeadm config map.
func (w *Workload) UpdateEtcdVersionInKubeadmConfigMap(ctx context.Context, imageRepository, imageTag string) error {
	configMapKey := ctrlclient.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}
	kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
	if err != nil {
		return err
	}
	config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
	changed, err := config.UpdateEtcdMeta(imageRepository, imageTag)
	if err != nil || !changed {
		return err
	}
	if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
		return errors.Wrap(err, "error updating kubeadm ConfigMap")
	}
	return nil
}

// UpdateKubernetesVersionInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error {
	configMapKey := ctrlclient.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}
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

// RemoveEtcdMemberForMachine removes the etcd member from the target cluster's etcd cluster.
func (w *Workload) RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		// Nothing to do, no node for Machine
		return nil
	}

	return w.removeMemberForNode(ctx, machine.Status.NodeRef.Name)
}

// RemoveMachineFromKubeadmConfigMap removes the entry for the machine from the kubeadm configmap.
func (w *Workload) RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		// Nothing to do, no node for Machine
		return nil
	}

	configMapKey := ctrlclient.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}
	kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
	if err != nil {
		return err
	}
	config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
	if err := config.RemoveAPIEndpoint(machine.Status.NodeRef.Name); err != nil {
		return err
	}
	if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
		return errors.Wrap(err, "error updating kubeadm ConfigMap")
	}
	return nil
}

// ReconcileKubeletRBACBinding will create a RoleBinding for the new kubelet version during upgrades.
// If the role binding already exists this function is a no-op.
func (w *Workload) ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error {
	roleName := fmt.Sprintf("kubeadm:kubelet-config-%d.%d", version.Major, version.Minor)
	roleBinding := &rbacv1.RoleBinding{}
	err := w.Client.Get(ctx, ctrlclient.ObjectKey{Name: roleName, Namespace: metav1.NamespaceSystem}, roleBinding)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to determine if kubelet config rbac role binding %q already exists", roleName)
	} else if err == nil {
		// The required role binding already exists, nothing left to do
		return nil
	}

	newRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: metav1.NamespaceSystem,
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "system:nodes",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "system:bootstrappers:kubeadm:default-node-token",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
	}
	if err := w.Client.Create(ctx, newRoleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create kubelet rbac role binding %q", roleName)
	}

	return nil
}

// ReconcileKubeletRBACRole will create a Role for the new kubelet version during upgrades.
// If the role already exists this function is a no-op.
func (w *Workload) ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error {
	majorMinor := fmt.Sprintf("%d.%d", version.Major, version.Minor)
	roleName := fmt.Sprintf("kubeadm:kubelet-config-%s", majorMinor)
	role := &rbacv1.Role{}
	if err := w.Client.Get(ctx, ctrlclient.ObjectKey{Name: roleName, Namespace: metav1.NamespaceSystem}, role); err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to determine if kubelet config rbac role %q already exists", roleName)
	} else if err == nil {
		// The required role already exists, nothing left to do
		return nil
	}

	newRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: metav1.NamespaceSystem,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"get"},
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{fmt.Sprintf("kubelet-config-%s", majorMinor)},
			},
		},
	}
	if err := w.Client.Create(ctx, newRole); err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrapf(err, "failed to create kubelet rbac role %q", roleName)
	}

	return nil
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
	kubeadmConfigKey := ctrlclient.ObjectKey{
		Name:      "kubeadm-config",
		Namespace: metav1.NamespaceSystem,
	}
	err = w.Client.Get(ctx, kubeadmConfigKey, &corev1.ConfigMap{})
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

func newClientCert(caCert *x509.Certificate, key *rsa.PrivateKey, caKey *rsa.PrivateKey) (*x509.Certificate, error) {
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

func checkStaticPodReadyCondition(pod *corev1.Pod) error {
	found := false
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			found = true
		}
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
			return errors.Errorf("static pod %s/%s is not ready", pod.Namespace, pod.Name)
		}
	}
	if !found {
		return errors.Errorf("pod does not have ready condition: %v", pod.Name)
	}
	return nil
}

func checkNodeNoExecuteCondition(node corev1.Node) error {
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			return errors.Errorf("node has NoExecute taint: %v", node.Name)
		}
	}
	return nil
}

func firstNodeNotMatchingName(name string, nodes []corev1.Node) *corev1.Node {
	for _, n := range nodes {
		if n.Name != name {
			return &n
		}
	}
	return nil
}

// UpdateKubeProxyImageInfo updates kube-proxy image in the kube-proxy DaemonSet.
func (w *Workload) UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error {
	ds := &appsv1.DaemonSet{}

	if err := w.Client.Get(ctx, ctrlclient.ObjectKey{Name: kubeProxyDaemonSetName, Namespace: metav1.NamespaceSystem}, ds); err != nil {
		if apierrors.IsNotFound(err) {
			// if kube-proxy is missing, return without errors
			return nil
		}
		return errors.Wrapf(err, "failed to determine if %s daemonset already exists", kubeProxyDaemonSetName)
	}

	if len(ds.Spec.Template.Spec.Containers) == 0 {
		return nil
	}
	newImageName, err := util.ModifyImageTag(ds.Spec.Template.Spec.Containers[0].Image, kcp.Spec.Version)
	if err != nil {
		return err
	}

	if ds.Spec.Template.Spec.Containers[0].Image != newImageName {
		patch := client.MergeFrom(ds.DeepCopy())
		ds.Spec.Template.Spec.Containers[0].Image = newImageName
		if err := w.Client.Patch(ctx, ds, patch); err != nil {
			return errors.Wrap(err, "error patching kube-proxy DaemonSet")
		}
	}
	return nil
}
