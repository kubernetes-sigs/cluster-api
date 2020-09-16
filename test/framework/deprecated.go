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

package framework

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/test/framework/management/kind"
	"sigs.k8s.io/cluster-api/test/framework/options"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// eventuallyInterval is the polling interval used by gomega's Eventually
	// Deprecated
	eventuallyInterval = 10 * time.Second
)

// Applier is an interface around applying YAML to a cluster
// Deprecated. Please use ClusterProxy
type Applier interface {
	// Apply allows us to apply YAML to the cluster, `kubectl apply`
	Apply(context.Context, []byte) error
}

// Waiter is an interface around waiting for something on a kubernetes cluster.
// Deprecated. Please use ClusterProxy
type Waiter interface {
	// Wait allows us to wait for something in the cluster, `kubectl wait`
	Wait(context.Context, ...string) error
}

// ImageLoader is an interface around loading an image onto a cluster.
// Deprecated. Please use ClusterProxy
type ImageLoader interface {
	// LoadImage will put a local image onto the cluster.
	LoadImage(context.Context, string) error
}

// ManagementCluster are all the features we need out of a kubernetes cluster to qualify as a management cluster.
// Deprecated. Please use ClusterProxy
type ManagementCluster interface {
	Applier
	Waiter
	// Teardown will completely clean up the ManagementCluster.
	// This should be implemented as a synchronous function.
	// Generally to be used in the AfterSuite function if a management cluster is shared between tests.
	// Should try to clean everything up and report any dangling artifacts that needs manual intervention.
	Teardown(context.Context)
	// GetName returns the name of the cluster.
	GetName() string
	// GetKubeconfigPath returns the path to the kubeconfig file for the cluster.
	GetKubeconfigPath() string
	// GetScheme returns the scheme defining the types hosted in the cluster.
	GetScheme() *runtime.Scheme
	// GetClient returns a client to the Management cluster.
	GetClient() (client.Client, error)
	// GetClientSet returns a clientset to the management cluster.
	GetClientSet() (*kubernetes.Clientset, error)
	// GetWorkdloadClient returns a client to the specified workload cluster.
	GetWorkloadClient(ctx context.Context, namespace, name string) (client.Client, error)
	// GetWorkerKubeconfigPath returns the path to the kubeconfig file for the specified workload cluster.
	GetWorkerKubeconfigPath(ctx context.Context, namespace, name string) (string, error)
}

// MachineDeployment contains the objects needed to create a
// CAPI MachineDeployment resource and its associated template
// resources.
// Deprecated. Please use the individual create/assert methods.
type MachineDeployment struct {
	MachineDeployment       *clusterv1.MachineDeployment
	BootstrapConfigTemplate runtime.Object
	InfraMachineTemplate    runtime.Object
}

// Node contains all the pieces necessary to make a single node
// Deprecated.
type Node struct {
	Machine         *clusterv1.Machine
	InfraMachine    runtime.Object
	BootstrapConfig runtime.Object
}

// ControlplaneClusterInput defines the necessary dependencies to run a multi-node control plane cluster.
// Deprecated.
type ControlplaneClusterInput struct {
	Management        ManagementCluster
	Cluster           *clusterv1.Cluster
	InfraCluster      runtime.Object
	Nodes             []Node
	MachineDeployment MachineDeployment
	RelatedResources  []runtime.Object
	CreateTimeout     time.Duration
	DeleteTimeout     time.Duration

	ControlPlane    *controlplanev1.KubeadmControlPlane
	MachineTemplate runtime.Object
}

// SetDefaults defaults the struct fields if necessary.
// Deprecated.
func (input *ControlplaneClusterInput) SetDefaults() {
	if input.CreateTimeout == 0 {
		input.CreateTimeout = 10 * time.Minute
	}

	if input.DeleteTimeout == 0 {
		input.DeleteTimeout = 5 * time.Minute
	}
}

// ControlPlaneCluster creates an n node control plane cluster.
// Assertions:
//  * The number of nodes in the created cluster will equal the number
//    of control plane nodes plus the number of replicas in the machine
//    deployment.
// Deprecated. Please use the supplied functions below to get the exact behavior desired.
func (input *ControlplaneClusterInput) ControlPlaneCluster() {
	ctx := context.Background()
	Expect(input.Management).ToNot(BeNil())

	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By("creating an InfrastructureCluster resource")
	Expect(mgmtClient.Create(ctx, input.InfraCluster)).To(Succeed())

	// This call happens in an eventually because of a race condition with the
	// webhook server. If the latter isn't fully online then this call will
	// fail.
	By("creating a Cluster resource linked to the InfrastructureCluster resource")
	Eventually(func() error {
		if err := mgmtClient.Create(ctx, input.Cluster); err != nil {
			log.Logf("Failed to create the cluster: %+v", err)
			return err
		}
		return nil
	}, input.CreateTimeout, eventuallyInterval).Should(BeNil())

	By("creating related resources")
	for i := range input.RelatedResources {
		obj := input.RelatedResources[i]
		By(fmt.Sprintf("creating a/an %s resource", obj.GetObjectKind().GroupVersionKind()))
		Eventually(func() error {
			return mgmtClient.Create(ctx, obj)
		}, input.CreateTimeout, eventuallyInterval).Should(BeNil())
	}

	By("creating the machine template")
	Expect(mgmtClient.Create(ctx, input.MachineTemplate)).To(Succeed())

	By("creating a KubeadmControlPlane")
	Eventually(func() error {
		err := mgmtClient.Create(ctx, input.ControlPlane)
		if err != nil {
			log.Logf("Failed to create the KubeadmControlPlane: %+v", err)
		}
		return err
	}, input.CreateTimeout, 10*time.Second).Should(BeNil())

	By("waiting for cluster to enter the provisioned phase")
	Eventually(func() (string, error) {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		if err := mgmtClient.Get(ctx, key, cluster); err != nil {
			return "", err
		}
		return cluster.Status.Phase, nil
	}, input.CreateTimeout, eventuallyInterval).Should(Equal(string(clusterv1.ClusterPhaseProvisioned)))

	// Create the machine deployment if the replica count >0.
	if machineDeployment := input.MachineDeployment.MachineDeployment; machineDeployment != nil {
		if replicas := machineDeployment.Spec.Replicas; replicas != nil && *replicas > 0 {
			By("creating a core MachineDeployment resource")
			Expect(mgmtClient.Create(ctx, machineDeployment)).To(Succeed())

			By("creating a BootstrapConfigTemplate resource")
			Expect(mgmtClient.Create(ctx, input.MachineDeployment.BootstrapConfigTemplate)).To(Succeed())

			By("creating an InfrastructureMachineTemplate resource")
			Expect(mgmtClient.Create(ctx, input.MachineDeployment.InfraMachineTemplate)).To(Succeed())
		}

		By("Waiting for the workload nodes to exist")
		Eventually(func() ([]corev1.Node, error) {
			workloadClient, err := input.Management.GetWorkloadClient(ctx, input.Cluster.Namespace, input.Cluster.Name)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get workload client")
			}
			nodeList := corev1.NodeList{}
			if err := workloadClient.List(ctx, &nodeList); err != nil {
				return nil, err
			}
			return nodeList.Items, nil
		}, input.CreateTimeout, 10*time.Second).Should(HaveLen(int(*machineDeployment.Spec.Replicas)))
	}

	By("waiting for all machines to be running")
	inClustersNamespaceListOption := client.InNamespace(input.Cluster.Namespace)
	matchClusterListOption := client.MatchingLabels{clusterv1.ClusterLabelName: input.Cluster.Name}
	Eventually(func() (bool, error) {
		// Get a list of all the Machine resources that belong to the Cluster.
		machineList := &clusterv1.MachineList{}
		if err := mgmtClient.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			return false, err
		}
		for _, machine := range machineList.Items {
			if machine.Status.Phase != string(clusterv1.MachinePhaseRunning) {
				return false, errors.Errorf("machine %s is not running, it's %s", machine.Name, machine.Status.Phase)
			}
		}
		return true, nil
	}, input.CreateTimeout, eventuallyInterval).Should(BeTrue())
	// wait for the control plane to be ready
	By("waiting for the control plane to be ready")
	Eventually(func() bool {
		controlplane := &controlplanev1.KubeadmControlPlane{}
		key := client.ObjectKey{
			Namespace: input.ControlPlane.GetNamespace(),
			Name:      input.ControlPlane.GetName(),
		}
		if err := mgmtClient.Get(ctx, key, controlplane); err != nil {
			log.Logf("Failed to get the control plane: %+v", err)
			return false
		}
		return controlplane.Status.Initialized
	}, input.CreateTimeout, 10*time.Second).Should(BeTrue())
}

// CleanUpCoreArtifacts deletes the cluster and waits for everything to be gone.
// Assertions made on objects owned by the Cluster:
//   * All Machines are removed
//   * All MachineSets are removed
//   * All MachineDeployments are removed
//   * All KubeadmConfigs are removed
//   * All Secrets are removed
// Deprecated
func (input *ControlplaneClusterInput) CleanUpCoreArtifacts() {
	input.SetDefaults()
	ctx := context.Background()
	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By(fmt.Sprintf("deleting cluster %s", input.Cluster.GetName()))
	Expect(mgmtClient.Delete(ctx, input.Cluster)).To(Succeed())

	Eventually(func() bool {
		clusters := clusterv1.ClusterList{}
		if err := mgmtClient.List(ctx, &clusters); err != nil {
			log.Logf("Failed to list the clusters: %+v", err)
			return false
		}
		return len(clusters.Items) == 0
	}, input.DeleteTimeout, eventuallyInterval).Should(BeTrue())

	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	listOpts := &client.ListOptions{LabelSelector: lbl}

	By("ensuring all CAPI artifacts have been deleted")
	ensureArtifactsDeleted(ctx, mgmtClient, listOpts)
}

// Deprecated
func ensureArtifactsDeleted(ctx context.Context, mgmtClient Lister, opt client.ListOption) {
	// assertions
	ml := &clusterv1.MachineList{}
	Expect(mgmtClient.List(ctx, ml, opt)).To(Succeed())
	Expect(ml.Items).To(HaveLen(0))

	msl := &clusterv1.MachineSetList{}
	Expect(mgmtClient.List(ctx, msl, opt)).To(Succeed())
	Expect(msl.Items).To(HaveLen(0))

	mdl := &clusterv1.MachineDeploymentList{}
	Expect(mgmtClient.List(ctx, mdl, opt)).To(Succeed())
	Expect(mdl.Items).To(HaveLen(0))

	kcpl := &controlplanev1.KubeadmControlPlaneList{}
	Expect(mgmtClient.List(ctx, kcpl, opt)).To(Succeed())
	Expect(kcpl.Items).To(HaveLen(0))

	kcl := &cabpkv1.KubeadmConfigList{}
	Expect(mgmtClient.List(ctx, kcl, opt)).To(Succeed())
	Expect(kcl.Items).To(HaveLen(0))

	sl := &corev1.SecretList{}
	Expect(mgmtClient.List(ctx, sl, opt)).To(Succeed())
	Expect(sl.Items).To(HaveLen(0))
}

// DumpResources dump cluster API related resources to YAML
// Deprecated. Please use DumpAllResources instead
func DumpResources(mgmt ManagementCluster, resourcePath string, writer io.Writer) error {
	resources := map[string]runtime.Object{
		"Cluster":             &clusterv1.ClusterList{},
		"MachineDeployment":   &clusterv1.MachineDeploymentList{},
		"MachineSet":          &clusterv1.MachineSetList{},
		"MachinePool":         &expv1.MachinePoolList{},
		"Machine":             &clusterv1.MachineList{},
		"KubeadmControlPlane": &controlplanev1.KubeadmControlPlaneList{},
		"KubeadmConfig":       &bootstrapv1.KubeadmConfigList{},
		"Node":                &corev1.NodeList{},
	}

	return dumpResources(mgmt, resources, resourcePath)
}

// DumpProviderResources dump provider specific API related resources to YAML
// Deprecated. Please use DumpAllResources instead
func DumpProviderResources(mgmt ManagementCluster, resources map[string]runtime.Object, resourcePath string, writer io.Writer) error {
	return dumpResources(mgmt, resources, resourcePath)
}

func dumpResources(mgmt ManagementCluster, resources map[string]runtime.Object, resourcePath string) error {
	c, err := mgmt.GetClient()
	if err != nil {
		return err
	}

	for kind, resourceList := range resources {
		if err := c.List(context.TODO(), resourceList); err != nil {
			return errors.Wrapf(err, "error getting resources of kind %s", kind)
		}

		objs, err := apimeta.ExtractList(resourceList)
		if err != nil {
			return errors.Wrapf(err, "error extracting list of kind %s", kind)
		}

		for _, obj := range objs {
			metaObj, _ := apimeta.Accessor(obj)
			if err != nil {
				return err
			}

			namespace := metaObj.GetNamespace()
			name := metaObj.GetName()

			resourceFilePath := path.Join(resourcePath, kind, namespace, name+".yaml")
			if err := dumpResource(resourceFilePath, obj); err != nil {
				return err
			}
		}
	}

	return nil
}

func dumpResource(resourceFilePath string, resource runtime.Object) error {
	log.Logf("Creating directory: %s\n", filepath.Dir(resourceFilePath))
	if err := os.MkdirAll(filepath.Dir(resourceFilePath), 0755); err != nil {
		return errors.Wrapf(err, "error making logDir %q", filepath.Dir(resourceFilePath))
	}

	f, err := os.OpenFile(resourceFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrapf(err, "error opening created logFile %q", resourceFilePath)
	}
	defer f.Close()

	resourceYAML, err := yaml.Marshal(resource)
	if err != nil {
		return errors.Wrapf(err, "error marshaling cluster ")
	}

	if err := ioutil.WriteFile(f.Name(), resourceYAML, 0600); err != nil {
		return errors.Wrapf(err, "error writing cluster yaml to file %q", f.Name())
	}

	return nil
}

// InitManagementClusterInput is the information required to initialize a new
// management cluster for e2e testing.
type InitManagementClusterInput struct {
	Config

	// Scheme is used to initialize the scheme for the management cluster
	// client.
	// Defaults to a new runtime.Scheme.
	Scheme *runtime.Scheme

	// ComponentGenerators is a list objects that supply additional component
	// YAML to apply to the management cluster.
	// Please note this is meant to be used at runtime to add YAML to the
	// management cluster outside of what is provided by the Components field.
	// For example, a caller could use this field to apply a Secret required by
	// some component from the Components field.
	ComponentGenerators []ComponentGenerator

	// NewManagementClusterFn may be used to provide a custom function for
	// returning a new management cluster. Otherwise kind.NewCluster is used.
	NewManagementClusterFn func() (ManagementCluster, error)
}

// Defaults assigns default values to the object.
func (c *InitManagementClusterInput) Defaults(ctx context.Context) {
	c.Config.Defaults()
	if c.Scheme == nil {
		c.Scheme = runtime.NewScheme()
	}
	if c.NewManagementClusterFn == nil {
		c.NewManagementClusterFn = func() (ManagementCluster, error) {
			return kind.NewCluster(ctx, c.ManagementClusterName, c.Scheme)
		}
	}
}

// InitManagementCluster returns a new cluster initialized as a CAPI management
// cluster.
// Deprecated. Please use bootstrap.ClusterProvider and ClusterProxy
func InitManagementCluster(ctx context.Context, input *InitManagementClusterInput) ManagementCluster {
	By("initializing the management cluster")
	Expect(input).ToNot(BeNil())

	By("initialzing the management cluster configuration defaults")
	input.Defaults(ctx)

	By("validating the management cluster configuration")
	Expect(input.Validate()).To(Succeed())

	By("loading the kubernetes and capi core schemes")
	TryAddDefaultSchemes(input.Scheme)

	By("creating the management cluster")
	managementCluster, err := input.NewManagementClusterFn()
	Expect(err).ToNot(HaveOccurred())
	Expect(managementCluster).ToNot(BeNil())

	// Load the images.
	if imageLoader, ok := managementCluster.(ImageLoader); ok {
		By("management cluster supports loading images")
		for _, image := range input.Images {
			switch image.LoadBehavior {
			case MustLoadImage:
				By(fmt.Sprintf("must load image %s into the management cluster", image.Name))
				Expect(imageLoader.LoadImage(ctx, image.Name)).To(Succeed())
			case TryLoadImage:
				By(fmt.Sprintf("try to load image %s into the management cluster", image.Name))
				imageLoader.LoadImage(ctx, image.Name) //nolint:errcheck
			}
		}
	}

	// Install the YAML from the component generators.
	for _, componentGenerator := range input.ComponentGenerators {
		InstallComponents(ctx, managementCluster, componentGenerator)
	}

	// Install all components.
	for _, component := range input.Components {
		for _, source := range component.Sources {
			name := component.Name
			if source.Name != "" {
				name = fmt.Sprintf("%s/%s", component.Name, source.Name)
			}
			source.Name = name
			InstallComponents(ctx, managementCluster, ComponentGeneratorForComponentSource(source))
		}
		for _, waiter := range component.Waiters {
			switch waiter.Type {
			case PodsWaiter:
				WaitForPodsReadyInNamespace(ctx, managementCluster, waiter.Value)
			case ServiceWaiter:
				WaitForAPIServiceAvailable(ctx, managementCluster, waiter.Value)
			}
		}
	}

	return managementCluster
}

// InstallComponents is a helper function that applies components, generally to a management cluster.
func InstallComponents(ctx context.Context, mgmt Applier, components ...ComponentGenerator) {
	Describe("Installing the provider components", func() {
		for _, component := range components {
			By(fmt.Sprintf("installing %s", component.GetName()))
			c, err := component.Manifests(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgmt.Apply(ctx, c)).To(Succeed())
		}
	})
}

// WaitForPodsReadyInNamespace will wait for all pods to be Ready in the
// specified namespace.
// For example, kubectl wait --for=condition=Ready --timeout=300s --namespace capi-system pods --all
func WaitForPodsReadyInNamespace(ctx context.Context, cluster Waiter, namespace string) {
	By(fmt.Sprintf("waiting for pods to be ready in namespace %q", namespace))
	err := cluster.Wait(ctx, "--for", "condition=Ready", "--timeout", "300s", "--namespace", namespace, "pods", "--all")
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)
}

// WaitForAPIServiceAvailable will wait for an an APIService to be available.
// For example, kubectl wait --for=condition=Available --timeout=300s apiservice v1beta1.webhook.cert-manager.io
func WaitForAPIServiceAvailable(ctx context.Context, mgmt Waiter, serviceName string) {
	By(fmt.Sprintf("waiting for api service %q to be available", serviceName))
	err := mgmt.Wait(ctx, "--for", "condition=Available", "--timeout", "300s", "apiservice", serviceName)
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)
}

// HTTPGetter wraps up the Get method exposed by the net/http.Client.
type HTTPGetter interface {
	Get(url string) (resp *http.Response, err error)
}

// ApplyYAMLURLInput is the input for ApplyYAMLURL.
type ApplyYAMLURLInput struct {
	Client        client.Client
	HTTPGetter    HTTPGetter
	NetworkingURL string
	Scheme        *runtime.Scheme
}

// ApplyYAMLURL is essentially kubectl apply -f <url>.
// If the YAML in the URL contains Kinds not registered with the scheme this will fail.
// Deprecated. Getting yaml from an URL during a test it can introduce flakes.
func ApplyYAMLURL(ctx context.Context, input ApplyYAMLURLInput) {
	By(fmt.Sprintf("Applying networking from %s", input.NetworkingURL))
	resp, err := input.HTTPGetter.Get(input.NetworkingURL)
	Expect(err).ToNot(HaveOccurred())
	yamls, err := ioutil.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())
	defer resp.Body.Close()
	yamlFiles := bytes.Split(yamls, []byte("---"))
	codecs := serializer.NewCodecFactory(input.Scheme)
	for _, f := range yamlFiles {
		f = bytes.TrimSpace(f)
		if len(f) == 0 {
			continue
		}
		decode := codecs.UniversalDeserializer().Decode
		obj, _, err := decode(f, nil, nil)
		if runtime.IsMissingKind(err) {
			continue
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(input.Client.Create(ctx, obj)).To(Succeed())
	}
}

// AssertAllClusterAPIResourcesAreGoneInput is the input for AssertAllClusterAPIResourcesAreGone.
type AssertAllClusterAPIResourcesAreGoneInput struct {
	Lister  Lister
	Cluster *clusterv1.Cluster
}

// AssertAllClusterAPIResourcesAreGone ensures that all known Cluster API resources have been remvoed.
// Deprecated. Please use GetCAPIResources instead
func AssertAllClusterAPIResourcesAreGone(ctx context.Context, input AssertAllClusterAPIResourcesAreGoneInput) {
	if options.SkipResourceCleanup {
		return
	}
	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	opt := &client.ListOptions{LabelSelector: lbl}

	By("ensuring all CAPI artifacts have been deleted")

	ml := &clusterv1.MachineList{}
	Expect(input.Lister.List(ctx, ml, opt)).To(Succeed())
	Expect(ml.Items).To(HaveLen(0))

	msl := &clusterv1.MachineSetList{}
	Expect(input.Lister.List(ctx, msl, opt)).To(Succeed())
	Expect(msl.Items).To(HaveLen(0))

	mdl := &clusterv1.MachineDeploymentList{}
	Expect(input.Lister.List(ctx, mdl, opt)).To(Succeed())
	Expect(mdl.Items).To(HaveLen(0))

	mpl := &expv1.MachinePoolList{}
	Expect(input.Lister.List(ctx, mpl, opt)).To(Succeed())
	Expect(mpl.Items).To(HaveLen(0))

	kcpl := &controlplanev1.KubeadmControlPlaneList{}
	Expect(input.Lister.List(ctx, kcpl, opt)).To(Succeed())
	Expect(kcpl.Items).To(HaveLen(0))

	kcl := &cabpkv1.KubeadmConfigList{}
	Expect(input.Lister.List(ctx, kcl, opt)).To(Succeed())
	Expect(kcl.Items).To(HaveLen(0))

	sl := &corev1.SecretList{}
	Expect(input.Lister.List(ctx, sl, opt)).To(Succeed())
	Expect(sl.Items).To(HaveLen(0))
}
