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
	"reflect"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/proxy"
	"sigs.k8s.io/cluster-api/internal/util/kubeadm"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/version"
)

const (
	kubeProxyKey                   = "kube-proxy"
	kubeadmConfigKey               = "kubeadm-config"
	kubeadmAPIServerCertCommonName = "kube-apiserver"
	kubeletConfigKey               = "kubelet"
	cgroupDriverKey                = "cgroupDriver"
	labelNodeRoleOldControlPlane   = "node-role.kubernetes.io/master" // Deprecated: https://github.com/kubernetes/kubeadm/issues/2200
	labelNodeRoleControlPlane      = "node-role.kubernetes.io/control-plane"
	clusterStatusKey               = "ClusterStatus"
	clusterConfigurationKey        = "ClusterConfiguration"
)

var (
	// Starting from v1.22.0 kubeadm dropped the usage of the ClusterStatus entry from the kubeadm-config ConfigMap
	// so we're not anymore required to remove API endpoints for control plane nodes after deletion.
	//
	// NOTE: The following assumes that kubeadm version equals to Kubernetes version.
	minKubernetesVersionWithoutClusterStatus = semver.MustParse("1.22.0")

	// Starting from v1.21.0 kubeadm defaults to systemdCGroup driver, as well as images built with ImageBuilder,
	// so it is necessary to mutate the kubelet-config-xx ConfigMap.
	//
	// NOTE: The following assumes that kubeadm version equals to Kubernetes version.
	minVerKubeletSystemdDriver = semver.MustParse("1.21.0")

	// Starting from v1.24.0 kubeadm uses "kubelet-config" a ConfigMap name for KubeletConfiguration,
	// Dropping the X-Y suffix.
	//
	// NOTE: The following assumes that kubeadm version equals to Kubernetes version.
	minVerUnversionedKubeletConfig = semver.MustParse("1.24.0")

	// ErrControlPlaneMinNodes signals that a cluster doesn't meet the minimum required nodes
	// to remove an etcd member.
	ErrControlPlaneMinNodes = errors.New("cluster has fewer than 2 control plane nodes; removing an etcd member is not supported")
)

// WorkloadCluster defines all behaviors necessary to upgrade kubernetes on a workload cluster
//
// TODO: Add a detailed description to each of these method definitions.
type WorkloadCluster interface {
	// Basic health and status checks.
	ClusterStatus(ctx context.Context) (ClusterStatus, error)
	UpdateStaticPodConditions(ctx context.Context, controlPlane *ControlPlane)
	UpdateEtcdConditions(ctx context.Context, controlPlane *ControlPlane)
	EtcdMembers(ctx context.Context) ([]string, error)
	GetAPIServerCertificateExpiry(ctx context.Context, kubeadmConfig *bootstrapv1.KubeadmConfig, nodeName string) (*time.Time, error)

	// Upgrade related tasks.
	ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error
	ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error
	UpdateKubernetesVersionInKubeadmConfigMap(version semver.Version) func(*bootstrapv1.ClusterConfiguration)
	UpdateImageRepositoryInKubeadmConfigMap(imageRepository string) func(*bootstrapv1.ClusterConfiguration)
	UpdateFeatureGatesInKubeadmConfigMap(featureGates map[string]bool) func(*bootstrapv1.ClusterConfiguration)
	UpdateEtcdLocalInKubeadmConfigMap(localEtcd *bootstrapv1.LocalEtcd) func(*bootstrapv1.ClusterConfiguration)
	UpdateEtcdExternalInKubeadmConfigMap(externalEtcd *bootstrapv1.ExternalEtcd) func(*bootstrapv1.ClusterConfiguration)
	UpdateAPIServerInKubeadmConfigMap(apiServer bootstrapv1.APIServer) func(*bootstrapv1.ClusterConfiguration)
	UpdateControllerManagerInKubeadmConfigMap(controllerManager bootstrapv1.ControlPlaneComponent) func(*bootstrapv1.ClusterConfiguration)
	UpdateSchedulerInKubeadmConfigMap(scheduler bootstrapv1.ControlPlaneComponent) func(*bootstrapv1.ClusterConfiguration)
	UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error
	UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, version semver.Version) error
	UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, version semver.Version) error
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error
	RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine, version semver.Version) error
	RemoveNodeFromKubeadmConfigMap(ctx context.Context, nodeName string, version semver.Version) error
	ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error
	AllowBootstrapTokensToGetNodes(ctx context.Context) error
	AllowClusterAdminPermissions(ctx context.Context, version semver.Version) error
	UpdateClusterConfiguration(ctx context.Context, version semver.Version, mutators ...func(*bootstrapv1.ClusterConfiguration)) error

	// State recovery tasks.
	ReconcileEtcdMembers(ctx context.Context, nodeNames []string, version semver.Version) ([]string, error)
}

// Workload defines operations on workload clusters.
type Workload struct {
	Client              ctrlclient.Client
	CoreDNSMigrator     coreDNSMigrator
	etcdClientGenerator etcdClientFor
	restConfig          *rest.Config
}

var _ WorkloadCluster = &Workload{}

func (w *Workload) getControlPlaneNodes(ctx context.Context) (*corev1.NodeList, error) {
	controlPlaneNodes := &corev1.NodeList{}
	controlPlaneNodeNames := sets.Set[string]{}

	for _, label := range []string{labelNodeRoleOldControlPlane, labelNodeRoleControlPlane} {
		nodes := &corev1.NodeList{}
		if err := w.Client.List(ctx, nodes, ctrlclient.MatchingLabels(map[string]string{
			label: "",
		})); err != nil {
			return nil, err
		}

		for i := range nodes.Items {
			node := nodes.Items[i]

			// Continue if we already added that node.
			if controlPlaneNodeNames.Has(node.Name) {
				continue
			}

			controlPlaneNodeNames.Insert(node.Name)
			controlPlaneNodes.Items = append(controlPlaneNodes.Items, node)
		}
	}

	return controlPlaneNodes, nil
}

func (w *Workload) getConfigMap(ctx context.Context, configMap ctrlclient.ObjectKey) (*corev1.ConfigMap, error) {
	original := &corev1.ConfigMap{}
	if err := w.Client.Get(ctx, configMap, original); err != nil {
		return nil, errors.Wrapf(err, "error getting %s/%s configmap from target cluster", configMap.Namespace, configMap.Name)
	}
	return original.DeepCopy(), nil
}

// UpdateImageRepositoryInKubeadmConfigMap updates the image repository in the kubeadm config map.
func (w *Workload) UpdateImageRepositoryInKubeadmConfigMap(imageRepository string) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		if imageRepository == "" {
			return
		}

		c.ImageRepository = imageRepository
	}
}

// UpdateFeatureGatesInKubeadmConfigMap updates the feature gates in the kubeadm config map.
func (w *Workload) UpdateFeatureGatesInKubeadmConfigMap(featureGates map[string]bool) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		// Even if featureGates is nil, reset it to ClusterConfiguration
		// to override any previously set feature gates.
		c.FeatureGates = featureGates
	}
}

// UpdateKubernetesVersionInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) UpdateKubernetesVersionInKubeadmConfigMap(version semver.Version) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		c.KubernetesVersion = fmt.Sprintf("v%s", version.String())
	}
}

// UpdateKubeletConfigMap will create a new kubelet-config-1.x config map for a new version of the kubelet.
// This is a necessary process for upgrades.
func (w *Workload) UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error {
	// Check if the desired configmap already exists
	desiredKubeletConfigMapName := generateKubeletConfigName(version)
	configMapKey := ctrlclient.ObjectKey{Name: desiredKubeletConfigMapName, Namespace: metav1.NamespaceSystem}
	_, err := w.getConfigMap(ctx, configMapKey)
	if err == nil {
		// Nothing to do, the configmap already exists
		return nil
	}
	if !apierrors.IsNotFound(errors.Cause(err)) {
		return errors.Wrapf(err, "error determining if kubelet configmap %s exists", desiredKubeletConfigMapName)
	}

	previousMinorVersionKubeletConfigMapName := generateKubeletConfigName(semver.Version{Major: version.Major, Minor: version.Minor - 1})

	// If desired and previous ConfigMap name are the same it means we already completed the transition
	// to the unified KubeletConfigMap name in the previous upgrade; no additional operations are required.
	if desiredKubeletConfigMapName == previousMinorVersionKubeletConfigMapName {
		return nil
	}

	configMapKey = ctrlclient.ObjectKey{Name: previousMinorVersionKubeletConfigMapName, Namespace: metav1.NamespaceSystem}
	// Returns a copy
	cm, err := w.getConfigMap(ctx, configMapKey)
	if apierrors.IsNotFound(errors.Cause(err)) {
		return errors.Errorf("unable to find kubelet configmap %s", previousMinorVersionKubeletConfigMapName)
	}
	if err != nil {
		return err
	}

	// In order to avoid using two cgroup drivers on the same machine,
	// (cgroupfs and systemd cgroup drivers), starting from
	// 1.21 image builder is going to configure containerd for using the
	// systemd driver, and the Kubelet configuration must be aligned to this change.
	// NOTE: It is considered safe to update the kubelet-config-1.21 ConfigMap
	// because only new nodes using v1.21 images will pick up the change during
	// kubeadm join.
	if version.GE(minVerKubeletSystemdDriver) {
		data, ok := cm.Data[kubeletConfigKey]
		if !ok {
			return errors.Errorf("unable to find %q key in %s", kubeletConfigKey, cm.Name)
		}
		kubeletConfig, err := yamlToUnstructured([]byte(data))
		if err != nil {
			return errors.Wrapf(err, "unable to decode kubelet ConfigMap's %q content to Unstructured object", kubeletConfigKey)
		}
		cgroupDriver, _, err := unstructured.NestedString(kubeletConfig.UnstructuredContent(), cgroupDriverKey)
		if err != nil {
			return errors.Wrapf(err, "unable to extract %q from Kubelet ConfigMap's %q", cgroupDriverKey, cm.Name)
		}

		// If the value is not already explicitly set by the user, change according to kubeadm/image builder new requirements.
		if cgroupDriver == "" {
			cgroupDriver = "systemd"

			if err := unstructured.SetNestedField(kubeletConfig.UnstructuredContent(), cgroupDriver, cgroupDriverKey); err != nil {
				return errors.Wrapf(err, "unable to update %q on Kubelet ConfigMap's %q", cgroupDriverKey, cm.Name)
			}
			updated, err := yaml.Marshal(kubeletConfig)
			if err != nil {
				return errors.Wrapf(err, "unable to encode Kubelet ConfigMap's %q to YAML", cm.Name)
			}
			cm.Data[kubeletConfigKey] = string(updated)
		}
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

// UpdateAPIServerInKubeadmConfigMap updates api server configuration in kubeadm config map.
func (w *Workload) UpdateAPIServerInKubeadmConfigMap(apiServer bootstrapv1.APIServer) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		c.APIServer = apiServer
	}
}

// UpdateControllerManagerInKubeadmConfigMap updates controller manager configuration in kubeadm config map.
func (w *Workload) UpdateControllerManagerInKubeadmConfigMap(controllerManager bootstrapv1.ControlPlaneComponent) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		c.ControllerManager = controllerManager
	}
}

// UpdateSchedulerInKubeadmConfigMap updates scheduler configuration in kubeadm config map.
func (w *Workload) UpdateSchedulerInKubeadmConfigMap(scheduler bootstrapv1.ControlPlaneComponent) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		c.Scheduler = scheduler
	}
}

// RemoveMachineFromKubeadmConfigMap removes the entry for the machine from the kubeadm configmap.
func (w *Workload) RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine, version semver.Version) error {
	if machine == nil || machine.Status.NodeRef == nil {
		// Nothing to do, no node for Machine
		return nil
	}

	return w.RemoveNodeFromKubeadmConfigMap(ctx, machine.Status.NodeRef.Name, version)
}

// RemoveNodeFromKubeadmConfigMap removes the entry for the node from the kubeadm configmap.
func (w *Workload) RemoveNodeFromKubeadmConfigMap(ctx context.Context, name string, v semver.Version) error {
	if version.Compare(v, minKubernetesVersionWithoutClusterStatus, version.WithoutPreReleases()) >= 0 {
		return nil
	}

	return w.updateClusterStatus(ctx, func(s *bootstrapv1.ClusterStatus) {
		delete(s.APIEndpoints, name)
	}, v)
}

// updateClusterStatus gets the ClusterStatus kubeadm-config ConfigMap, converts it to the
// Cluster API representation, and then applies a mutation func; if changes are detected, the
// data are converted back into the Kubeadm API version in use for the target Kubernetes version and the
// kubeadm-config ConfigMap updated.
func (w *Workload) updateClusterStatus(ctx context.Context, mutator func(status *bootstrapv1.ClusterStatus), version semver.Version) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		key := ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}
		configMap, err := w.getConfigMap(ctx, key)
		if err != nil {
			return errors.Wrap(err, "failed to get kubeadmConfigMap")
		}

		currentData, ok := configMap.Data[clusterStatusKey]
		if !ok {
			return errors.Errorf("unable to find %q in the kubeadm-config ConfigMap", clusterStatusKey)
		}

		currentClusterStatus, err := kubeadmtypes.UnmarshalClusterStatus(currentData)
		if err != nil {
			return errors.Wrapf(err, "unable to decode %q in the kubeadm-config ConfigMap's from YAML", clusterStatusKey)
		}

		updatedClusterStatus := currentClusterStatus.DeepCopy()
		mutator(updatedClusterStatus)

		if !reflect.DeepEqual(currentClusterStatus, updatedClusterStatus) {
			updatedData, err := kubeadmtypes.MarshalClusterStatusForVersion(updatedClusterStatus, version)
			if err != nil {
				return errors.Wrapf(err, "unable to encode %q kubeadm-config ConfigMap's to YAML", clusterStatusKey)
			}
			configMap.Data[clusterStatusKey] = updatedData
			if err := w.Client.Update(ctx, configMap); err != nil {
				return errors.Wrap(err, "failed to upgrade the kubeadmConfigMap")
			}
		}
		return nil
	})
}

// UpdateClusterConfiguration gets the ClusterConfiguration kubeadm-config ConfigMap, converts it to the
// Cluster API representation, and then applies a mutation func; if changes are detected, the
// data are converted back into the Kubeadm API version in use for the target Kubernetes version and the
// kubeadm-config ConfigMap updated.
func (w *Workload) UpdateClusterConfiguration(ctx context.Context, version semver.Version, mutators ...func(*bootstrapv1.ClusterConfiguration)) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		key := ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}
		configMap, err := w.getConfigMap(ctx, key)
		if err != nil {
			return errors.Wrap(err, "failed to get kubeadmConfigMap")
		}

		currentData, ok := configMap.Data[clusterConfigurationKey]
		if !ok {
			return errors.Errorf("unable to find %q in the kubeadm-config ConfigMap", clusterConfigurationKey)
		}

		currentObj, err := kubeadmtypes.UnmarshalClusterConfiguration(currentData)
		if err != nil {
			return errors.Wrapf(err, "unable to decode %q in the kubeadm-config ConfigMap's from YAML", clusterConfigurationKey)
		}

		updatedObj := currentObj.DeepCopy()
		for i := range mutators {
			mutators[i](updatedObj)
		}

		if !reflect.DeepEqual(currentObj, updatedObj) {
			updatedData, err := kubeadmtypes.MarshalClusterConfigurationForVersion(updatedObj, version)
			if err != nil {
				return errors.Wrapf(err, "unable to encode %q kubeadm-config ConfigMap's to YAML", clusterConfigurationKey)
			}
			configMap.Data[clusterConfigurationKey] = updatedData
			if err := w.Client.Update(ctx, configMap); err != nil {
				return errors.Wrap(err, "failed to upgrade cluster configuration in the kubeadmConfigMap")
			}
		}
		return nil
	})
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

// GetAPIServerCertificateExpiry returns the certificate expiry of the apiserver on the given node.
func (w *Workload) GetAPIServerCertificateExpiry(ctx context.Context, kubeadmConfig *bootstrapv1.KubeadmConfig, nodeName string) (*time.Time, error) {
	// Create a proxy.
	p := proxy.Proxy{
		Kind:       "pods",
		Namespace:  metav1.NamespaceSystem,
		KubeConfig: w.restConfig,
		Port:       int(calculateAPIServerPort(kubeadmConfig)),
	}

	// Create a dialer.
	dialer, err := proxy.NewDialer(p)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get certificate expiry for kube-apiserver on Node/%s: failed to create dialer", nodeName)
	}

	// Dial to the kube-apiserver.
	rawConn, err := dialer.DialContextWithAddr(ctx, staticPodName("kube-apiserver", nodeName))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get certificate expiry for kube-apiserver on Node/%s: unable to dial to kube-apiserver", nodeName)
	}

	// Execute a TLS handshake over the connection to the kube-apiserver.
	// xref: roughly same code as in tls.DialWithDialer.
	conn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // Intentionally not verifying the server cert here.
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, errors.Wrapf(err, "unable to get certificate expiry for kube-apiserver on Node/%s: TLS handshake with the kube-apiserver failed", nodeName)
	}
	defer conn.Close()

	// Return the expiry of the peer certificate with cn=kube-apiserver (which is the one generated by kubeadm).
	var kubeAPIServerCert *x509.Certificate
	for _, cert := range conn.ConnectionState().PeerCertificates {
		if cert.Subject.CommonName == kubeadmAPIServerCertCommonName {
			kubeAPIServerCert = cert
		}
	}
	if kubeAPIServerCert == nil {
		return nil, errors.Wrapf(err, "unable to get certificate expiry for kube-apiserver on Node/%s: couldn't get peer certificate with cn=%q", nodeName, kubeadmAPIServerCertCommonName)
	}
	return &kubeAPIServerCert.NotAfter, nil
}

// calculateAPIServerPort calculates the kube-apiserver bind port based
// on a KubeadmConfig.
func calculateAPIServerPort(config *bootstrapv1.KubeadmConfig) int32 {
	if config.Spec.InitConfiguration != nil &&
		config.Spec.InitConfiguration.LocalAPIEndpoint.BindPort != 0 {
		return config.Spec.InitConfiguration.LocalAPIEndpoint.BindPort
	}

	if config.Spec.JoinConfiguration != nil &&
		config.Spec.JoinConfiguration.ControlPlane != nil &&
		config.Spec.JoinConfiguration.ControlPlane.LocalAPIEndpoint.BindPort != 0 {
		return config.Spec.JoinConfiguration.ControlPlane.LocalAPIEndpoint.BindPort
	}

	return 6443
}

func generateClientCert(caCertEncoded, caKeyEncoded []byte, clientKey *rsa.PrivateKey) (tls.Certificate, error) {
	caCert, err := certs.DecodeCertPEM(caCertEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}
	caKey, err := certs.DecodePrivateKeyPEM(caKeyEncoded)
	if err != nil {
		return tls.Certificate{}, err
	}
	x509Cert, err := newClientCert(caCert, clientKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certs.EncodeCertPEM(x509Cert), certs.EncodePrivateKeyPEM(clientKey))
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

// UpdateKubeProxyImageInfo updates kube-proxy image in the kube-proxy DaemonSet.
func (w *Workload) UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, version semver.Version) error {
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

	// Modify the image repository if a value was explicitly set or an upgrade is required.
	imageRepository := ImageRepositoryFromClusterConfig(kcp.Spec.KubeadmConfigSpec.ClusterConfiguration, version)
	if imageRepository != "" {
		newImageName, err = containerutil.ModifyImageRepository(newImageName, imageRepository)
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

// yamlToUnstructured looks inside a config map for a specific key and extracts the embedded YAML into an
// *unstructured.Unstructured.
func yamlToUnstructured(rawYAML []byte) (*unstructured.Unstructured, error) {
	unst := &unstructured.Unstructured{}
	err := yaml.Unmarshal(rawYAML, unst)
	return unst, err
}

// ImageRepositoryFromClusterConfig returns the image repository to use. It returns:
//   - clusterConfig.ImageRepository if set.
//   - else either k8s.gcr.io or registry.k8s.io depending on the default registry of the kubeadm
//     binary of the given kubernetes version. This is only done for Kubernetes versions >= v1.22.0
//     and < v1.26.0 because in this version range the default registry was changed.
//
// Note: Please see the following issue for more context: https://github.com/kubernetes-sigs/cluster-api/issues/7833
// tl;dr is that the imageRepository must be in sync with the default registry of kubeadm.
// Otherwise kubeadm preflight checks will fail because kubeadm is trying to pull the CoreDNS image
// from the wrong repository (<registry>/coredns instead of <registry>/coredns/coredns).
func ImageRepositoryFromClusterConfig(clusterConfig *bootstrapv1.ClusterConfiguration, kubernetesVersion semver.Version) string {
	// If ImageRepository is explicitly specified, return early.
	if clusterConfig != nil &&
		clusterConfig.ImageRepository != "" {
		return clusterConfig.ImageRepository
	}

	// If v1.22.0 <= version < v1.26.0 return the default registry of the
	// corresponding kubeadm binary.
	if kubernetesVersion.GTE(kubeadm.MinKubernetesVersionImageRegistryMigration) &&
		kubernetesVersion.LT(kubeadm.NextKubernetesVersionImageRegistryMigration) {
		return kubeadm.GetDefaultRegistry(kubernetesVersion)
	}

	// Use defaulting or current values otherwise.
	return ""
}
