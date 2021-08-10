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

	"github.com/blang/semver"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	kubeadmtypes "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/certs"
	containerutil "sigs.k8s.io/cluster-api/util/container"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	kubeProxyKey              = "kube-proxy"
	kubeadmConfigKey          = "kubeadm-config"
	kubeletConfigKey          = "kubelet"
	cgroupDriverKey           = "cgroupDriver"
	labelNodeRoleControlPlane = "node-role.kubernetes.io/master"
	clusterStatusKey          = "ClusterStatus"
	clusterConfigurationKey   = "ClusterConfiguration"
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

	// Upgrade related tasks.
	ReconcileKubeletRBACBinding(ctx context.Context, version semver.Version) error
	ReconcileKubeletRBACRole(ctx context.Context, version semver.Version) error
	UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error
	UpdateImageRepositoryInKubeadmConfigMap(ctx context.Context, imageRepository string, version semver.Version) error
	UpdateEtcdVersionInKubeadmConfigMap(ctx context.Context, imageRepository, imageTag string, version semver.Version) error
	UpdateEtcdExtraArgsInKubeadmConfigMap(ctx context.Context, extraArgs map[string]string, version semver.Version) error
	UpdateAPIServerInKubeadmConfigMap(ctx context.Context, apiServer bootstrapv1.APIServer, version semver.Version) error
	UpdateControllerManagerInKubeadmConfigMap(ctx context.Context, controllerManager bootstrapv1.ControlPlaneComponent, version semver.Version) error
	UpdateSchedulerInKubeadmConfigMap(ctx context.Context, scheduler bootstrapv1.ControlPlaneComponent, version semver.Version) error
	UpdateKubeletConfigMap(ctx context.Context, version semver.Version) error
	UpdateKubeProxyImageInfo(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane) error
	UpdateCoreDNS(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, version semver.Version) error
	RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error
	RemoveMachineFromKubeadmConfigMap(ctx context.Context, machine *clusterv1.Machine, version semver.Version) error
	RemoveNodeFromKubeadmConfigMap(ctx context.Context, nodeName string, version semver.Version) error
	ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error
	AllowBootstrapTokensToGetNodes(ctx context.Context) error

	// State recovery tasks.
	ReconcileEtcdMembers(ctx context.Context, nodeNames []string, version semver.Version) ([]string, error)
}

// Workload defines operations on workload clusters.
type Workload struct {
	Client              ctrlclient.Client
	CoreDNSMigrator     coreDNSMigrator
	etcdClientGenerator etcdClientFor
}

var _ WorkloadCluster = &Workload{}

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

// UpdateImageRepositoryInKubeadmConfigMap updates the image repository in the kubeadm config map.
func (w *Workload) UpdateImageRepositoryInKubeadmConfigMap(ctx context.Context, imageRepository string, version semver.Version) error {
	return w.updateClusterConfiguration(ctx, func(c *bootstrapv1.ClusterConfiguration) {
		if imageRepository == "" {
			return
		}
		c.ImageRepository = imageRepository
	}, version)
}

// UpdateKubernetesVersionInKubeadmConfigMap updates the kubernetes version in the kubeadm config map.
func (w *Workload) UpdateKubernetesVersionInKubeadmConfigMap(ctx context.Context, version semver.Version) error {
	return w.updateClusterConfiguration(ctx, func(c *bootstrapv1.ClusterConfiguration) {
		c.KubernetesVersion = fmt.Sprintf("v%s", version.String())
	}, version)
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
func (w *Workload) UpdateAPIServerInKubeadmConfigMap(ctx context.Context, apiServer bootstrapv1.APIServer, version semver.Version) error {
	return w.updateClusterConfiguration(ctx, func(c *bootstrapv1.ClusterConfiguration) {
		c.APIServer = apiServer
	}, version)
}

// UpdateControllerManagerInKubeadmConfigMap updates controller manager configuration in kubeadm config map.
func (w *Workload) UpdateControllerManagerInKubeadmConfigMap(ctx context.Context, controllerManager bootstrapv1.ControlPlaneComponent, version semver.Version) error {
	return w.updateClusterConfiguration(ctx, func(c *bootstrapv1.ClusterConfiguration) {
		c.ControllerManager = controllerManager
	}, version)
}

// UpdateSchedulerInKubeadmConfigMap updates scheduler configuration in kubeadm config map.
func (w *Workload) UpdateSchedulerInKubeadmConfigMap(ctx context.Context, scheduler bootstrapv1.ControlPlaneComponent, version semver.Version) error {
	return w.updateClusterConfiguration(ctx, func(c *bootstrapv1.ClusterConfiguration) {
		c.Scheduler = scheduler
	}, version)
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
func (w *Workload) RemoveNodeFromKubeadmConfigMap(ctx context.Context, name string, version semver.Version) error {
	if version.GTE(minKubernetesVersionWithoutClusterStatus) {
		return nil
	}

	return w.updateClusterStatus(ctx, func(s *bootstrapv1.ClusterStatus) {
		delete(s.APIEndpoints, name)
	}, version)
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

// updateClusterConfiguration gets the ClusterConfiguration kubeadm-config ConfigMap, converts it to the
// Cluster API representation, and then applies a mutation func; if changes are detected, the
// data are converted back into the Kubeadm API version in use for the target Kubernetes version and the
// kubeadm-config ConfigMap updated.
func (w *Workload) updateClusterConfiguration(ctx context.Context, mutator func(*bootstrapv1.ClusterConfiguration), version semver.Version) error {
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
		mutator(updatedObj)

		if !reflect.DeepEqual(currentObj, updatedObj) {
			updatedData, err := kubeadmtypes.MarshalClusterConfigurationForVersion(updatedObj, version)
			if err != nil {
				return errors.Wrapf(err, "unable to encode %q kubeadm-config ConfigMap's to YAML", clusterConfigurationKey)
			}
			configMap.Data[clusterConfigurationKey] = updatedData
			if err := w.Client.Update(ctx, configMap); err != nil {
				return errors.Wrap(err, "failed to upgrade the kubeadmConfigMap")
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

// yamlToUnstructured looks inside a config map for a specific key and extracts the embedded YAML into an
// *unstructured.Unstructured.
func yamlToUnstructured(rawYAML []byte) (*unstructured.Unstructured, error) {
	unst := &unstructured.Unstructured{}
	err := yaml.Unmarshal(rawYAML, unst)
	return unst, err
}
