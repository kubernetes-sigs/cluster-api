/*
Copyright 2019 The Kubernetes Authors.

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

package cluster

import (
	"context"
	_ "embed"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	utilresource "sigs.k8s.io/cluster-api/util/resource"
	"sigs.k8s.io/cluster-api/util/version"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

const (
	waitCertManagerInterval = 1 * time.Second

	certManagerNamespace = "cert-manager"

	// Deprecated: Use clusterctlv1.CertManagerVersionAnnotation instead.
	// This is maintained only for supporting upgrades from cluster created with clusterctl v1alpha3.
	// TODO: Remove once upgrades from v1alpha3 are no longer supported.
	certManagerVersionAnnotation = "certmanager.clusterctl.cluster.x-k8s.io/version"
)

var (
	//go:embed assets/cert-manager-test-resources.yaml
	certManagerTestManifest []byte
)

// CertManagerUpgradePlan defines the upgrade plan if cert-manager needs to be
// upgraded to a different version.
type CertManagerUpgradePlan struct {
	ExternallyManaged bool
	From, To          string
	ShouldUpgrade     bool
}

// CertManagerClient has methods to work with cert-manager components in the cluster.
type CertManagerClient interface {
	// EnsureInstalled makes sure cert-manager is running and its API is available.
	// This is required to install a new provider.
	EnsureInstalled() error

	// EnsureLatestVersion checks the cert-manager version currently installed, and if it is
	// older than the version currently suggested by clusterctl, upgrades it.
	EnsureLatestVersion() error

	// PlanUpgrade retruns a CertManagerUpgradePlan with information regarding
	// a cert-manager upgrade if necessary.
	PlanUpgrade() (CertManagerUpgradePlan, error)

	// Images return the list of images required for installing the cert-manager.
	Images() ([]string, error)
}

// certManagerClient implements CertManagerClient .
type certManagerClient struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	proxy                   Proxy
	pollImmediateWaiter     PollImmediateWaiter
}

// Ensure certManagerClient implements the CertManagerClient interface.
var _ CertManagerClient = &certManagerClient{}

// newCertManagerClient returns a certManagerClient.
func newCertManagerClient(configClient config.Client, repositoryClientFactory RepositoryClientFactory, proxy Proxy, pollImmediateWaiter PollImmediateWaiter) *certManagerClient {
	return &certManagerClient{
		configClient:            configClient,
		repositoryClientFactory: repositoryClientFactory,
		proxy:                   proxy,
		pollImmediateWaiter:     pollImmediateWaiter,
	}
}

// Images return the list of images required for installing the cert-manager.
func (cm *certManagerClient) Images() ([]string, error) {
	// If cert manager already exists in the cluster, there is no need of additional images for cert-manager.
	exists, err := cm.certManagerNamespaceExists()
	if err != nil {
		return nil, err
	}
	if exists {
		return []string{}, nil
	}

	// Otherwise, retrieve the images from the cert-manager manifest.
	config, err := cm.configClient.CertManager().Get()
	if err != nil {
		return nil, err
	}

	objs, err := cm.getManifestObjs(config)
	if err != nil {
		return nil, err
	}

	images, err := util.InspectImages(objs)
	if err != nil {
		return nil, err
	}
	return images, nil
}

func (cm *certManagerClient) certManagerNamespaceExists() (bool, error) {
	ns := &corev1.Namespace{}
	key := client.ObjectKey{Name: certManagerNamespace}
	c, err := cm.proxy.NewClient()
	if err != nil {
		return false, err
	}

	if err := c.Get(ctx, key, ns); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// EnsureInstalled makes sure cert-manager is running and its API is available.
// This is required to install a new provider.
func (cm *certManagerClient) EnsureInstalled() error {
	log := logf.Log

	// Checking if a version of cert manager supporting cert-manager-test-resources.yaml is already installed and properly working.
	if err := cm.waitForAPIReady(ctx, false); err == nil {
		log.Info("Skipping installing cert-manager as it is already installed")
		return nil
	}

	// Otherwise install cert manager.
	// NOTE: this instance of cert-manager will have clusterctl specific annotations that will be used to
	// manage the lifecycle of all the components.
	return cm.install()
}

func (cm *certManagerClient) install() error {
	log := logf.Log

	config, err := cm.configClient.CertManager().Get()
	if err != nil {
		return err
	}
	log.Info("Installing cert-manager", "Version", config.Version())

	// Gets the cert-manager components from the repository.
	objs, err := cm.getManifestObjs(config)
	if err != nil {
		return err
	}

	// Install all cert-manager manifests
	createCertManagerBackoff := newWriteBackoff()
	objs = utilresource.SortForCreate(objs)
	for i := range objs {
		o := objs[i]
		// Create the Kubernetes object.
		// Nb. The operation is wrapped in a retry loop to make ensureCerts more resilient to unexpected conditions.
		if err := retryWithExponentialBackoff(createCertManagerBackoff, func() error {
			return cm.createObj(o)
		}); err != nil {
			return err
		}
	}

	// Wait for the cert-manager API to be ready to accept requests
	return cm.waitForAPIReady(ctx, true)
}

// PlanUpgrade retruns a CertManagerUpgradePlan with information regarding
// a cert-manager upgrade if necessary.
func (cm *certManagerClient) PlanUpgrade() (CertManagerUpgradePlan, error) {
	log := logf.Log

	objs, err := cm.proxy.ListResources(map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue}, certManagerNamespace)
	if err != nil {
		return CertManagerUpgradePlan{}, errors.Wrap(err, "failed get cert manager components")
	}

	// If there are no cert manager components with the clusterctl labels, it means that cert-manager is externally managed.
	if len(objs) == 0 {
		log.V(5).Info("Skipping cert-manager version check because externally managed")
		return CertManagerUpgradePlan{ExternallyManaged: true}, nil
	}

	log.Info("Checking cert-manager version...")
	currentVersion, targetVersion, shouldUpgrade, err := cm.shouldUpgrade(objs)
	if err != nil {
		return CertManagerUpgradePlan{}, err
	}

	return CertManagerUpgradePlan{
		From:          currentVersion,
		To:            targetVersion,
		ShouldUpgrade: shouldUpgrade,
	}, nil
}

// EnsureLatestVersion checks the cert-manager version currently installed, and if it is
// older than the version currently suggested by clusterctl, upgrades it.
func (cm *certManagerClient) EnsureLatestVersion() error {
	log := logf.Log

	objs, err := cm.proxy.ListResources(map[string]string{clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelCertManagerValue}, certManagerNamespace)
	if err != nil {
		return errors.Wrap(err, "failed get cert manager components")
	}

	// If there are no cert manager components with the clusterctl labels, it means that cert-manager is externally managed.
	if len(objs) == 0 {
		log.V(5).Info("Skipping cert-manager upgrade because externally managed")
		return nil
	}

	log.Info("Checking cert-manager version...")
	currentVersion, _, shouldUpgrade, err := cm.shouldUpgrade(objs)
	if err != nil {
		return err
	}

	if !shouldUpgrade {
		log.Info("Cert-manager is already up to date")
		return nil
	}

	// delete the cert-manager version currently installed (because it should be upgraded);
	// NOTE: CRDs, and namespace are preserved in order to avoid deletion of user objects;
	// web-hooks are preserved to avoid a user attempting to CREATE a cert-manager resource while the upgrade is in progress.
	log.Info("Deleting cert-manager", "Version", currentVersion)
	if err := cm.deleteObjs(objs); err != nil {
		return err
	}

	// Install cert-manager.
	return cm.install()
}

func (cm *certManagerClient) deleteObjs(objs []unstructured.Unstructured) error {
	deleteCertManagerBackoff := newWriteBackoff()
	for i := range objs {
		obj := objs[i]

		// CRDs, and namespace are preserved in order to avoid deletion of user objects;
		// web-hooks are preserved to avoid a user attempting to CREATE a cert-manager resource while the upgrade is in progress.
		if obj.GetKind() == "CustomResourceDefinition" ||
			obj.GetKind() == "Namespace" ||
			obj.GetKind() == "MutatingWebhookConfiguration" ||
			obj.GetKind() == "ValidatingWebhookConfiguration" {
			continue
		}

		if err := retryWithExponentialBackoff(deleteCertManagerBackoff, func() error {
			if err := cm.deleteObj(obj); err != nil {
				// tolerate NotFound errors when deleting the test resources
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (cm *certManagerClient) shouldUpgrade(objs []unstructured.Unstructured) (string, string, bool, error) {
	config, err := cm.configClient.CertManager().Get()
	if err != nil {
		return "", "", false, err
	}

	desiredVersion := config.Version()
	desiredSemVersion, err := semver.ParseTolerant(desiredVersion)
	if err != nil {
		return "", "", false, errors.Wrapf(err, "failed to parse config version [%s] for cert-manager component", desiredVersion)
	}

	needUpgrade := false
	currentVersion := ""
	for i := range objs {
		obj := objs[i]

		// Endpoints and EndpointSlices are generated by Kubernetes without the version annotation, so we are skipping them
		if obj.GetKind() == "Endpoints" || obj.GetKind() == "EndpointSlice" {
			continue
		}

		// if there is no version annotation, this means the obj is cert-manager v0.11.0 (installed with older version of clusterctl)
		objVersion, ok := obj.GetAnnotations()[clusterctlv1.CertManagerVersionAnnotation]
		if !ok {
			// try the old annotation name
			objVersion, ok = obj.GetAnnotations()[certManagerVersionAnnotation]
			if !ok {
				currentVersion = "v0.11.0"
				needUpgrade = true
				break
			}
		}

		objSemVersion, err := semver.ParseTolerant(objVersion)
		if err != nil {
			return "", "", false, errors.Wrapf(err, "failed to parse version for cert-manager component %s/%s", obj.GetKind(), obj.GetName())
		}

		c := version.Compare(objSemVersion, desiredSemVersion, version.WithBuildTags())
		switch {
		case c < 0 || c == 2:
			// if version < current or same version and different non numeric build metadata, then upgrade
			currentVersion = objVersion
			needUpgrade = true
		case c >= 0:
			// the installed version is greather or equal than the one required by clusterctl, so we are ok
			currentVersion = objVersion
		}

		if needUpgrade {
			break
		}
	}
	return currentVersion, desiredVersion, needUpgrade, nil
}

func (cm *certManagerClient) getWaitTimeout() time.Duration {
	log := logf.Log

	certManagerConfig, err := cm.configClient.CertManager().Get()
	if err != nil {
		return config.CertManagerDefaultTimeout
	}
	timeoutDuration, err := time.ParseDuration(certManagerConfig.Timeout())
	if err != nil {
		log.Info("Invalid value set for cert-manager configuration", "timeout", certManagerConfig.Timeout())
		return config.CertManagerDefaultTimeout
	}
	return timeoutDuration
}

func (cm *certManagerClient) getManifestObjs(certManagerConfig config.CertManager) ([]unstructured.Unstructured, error) {
	// Given that cert manager components yaml are stored in a repository like providers components yaml,
	// we are using the same machinery to retrieve the file by using a fake provider object using
	// the cert manager repository url.
	certManagerFakeProvider := config.NewProvider("cert-manager", certManagerConfig.URL(), "")
	certManagerRepository, err := cm.repositoryClientFactory(certManagerFakeProvider, cm.configClient)
	if err != nil {
		return nil, err
	}

	// Gets the cert-manager component yaml from the repository.
	file, err := certManagerRepository.Components().Raw(repository.ComponentsOptions{
		Version: certManagerConfig.Version(),
	})
	if err != nil {
		return nil, err
	}

	// Converts the file to ustructured objects.
	objs, err := utilyaml.ToUnstructured(file)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml for cert-manager manifest")
	}

	// Apply image overrides.
	objs, err = util.FixImages(objs, func(image string) (string, error) {
		return cm.configClient.ImageMeta().AlterImage(config.CertManagerImageComponent, image)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply image override to the cert-manager manifest")
	}

	// Add cert manager labels and annotations.
	objs = addCerManagerLabel(objs)
	objs = addCerManagerAnnotations(objs, certManagerConfig.Version())

	return objs, nil
}

func addCerManagerLabel(objs []unstructured.Unstructured) []unstructured.Unstructured {
	for _, o := range objs {
		labels := o.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[clusterctlv1.ClusterctlLabelName] = ""
		labels[clusterctlv1.ClusterctlCoreLabelName] = clusterctlv1.ClusterctlCoreLabelCertManagerValue
		o.SetLabels(labels)
	}
	return objs
}

func addCerManagerAnnotations(objs []unstructured.Unstructured, version string) []unstructured.Unstructured {
	for _, o := range objs {
		annotations := o.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[clusterctlv1.CertManagerVersionAnnotation] = version
		o.SetAnnotations(annotations)
	}
	return objs
}

// getTestResourcesManifestObjs gets the cert-manager test manifests, converted to unstructured objects.
// These are used to ensure the cert-manager API components are all ready and the API is available for use.
func getTestResourcesManifestObjs() ([]unstructured.Unstructured, error) {
	objs, err := utilyaml.ToUnstructured(certManagerTestManifest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml for cert-manager test resources manifest")
	}
	return objs, nil
}

func (cm *certManagerClient) createObj(obj unstructured.Unstructured) error {
	log := logf.Log

	c, err := cm.proxy.NewClient()
	if err != nil {
		return err
	}

	// check if the component already exists, and eventually update it; otherwise create it
	// NOTE: This is required because this func is used also for upgrading cert-manager and during upgrades
	// some objects of the previous release are preserved in order to avoid to delete user data (e.g. CRDs).
	currentR := &unstructured.Unstructured{}
	currentR.SetGroupVersionKind(obj.GroupVersionKind())

	key := client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	if err := c.Get(ctx, key, currentR); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get cert-manager object %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}

		// if it does not exists, create the component
		log.V(5).Info("Creating", logf.UnstructuredToValues(obj)...)
		if err := c.Create(ctx, &obj); err != nil {
			return errors.Wrapf(err, "failed to create cert-manager component %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
		}
		return nil
	}

	// otherwise update the component
	log.V(5).Info("Updating", logf.UnstructuredToValues(obj)...)
	obj.SetResourceVersion(currentR.GetResourceVersion())
	if err := c.Update(ctx, &obj); err != nil {
		return errors.Wrapf(err, "failed to update cert-manager component %s, %s/%s", obj.GroupVersionKind(), obj.GetNamespace(), obj.GetName())
	}
	return nil
}

func (cm *certManagerClient) deleteObj(obj unstructured.Unstructured) error {
	log := logf.Log
	log.V(5).Info("Deleting", logf.UnstructuredToValues(obj)...)

	cl, err := cm.proxy.NewClient()
	if err != nil {
		return err
	}

	return cl.Delete(ctx, &obj)
}

// waitForAPIReady will attempt to create the cert-manager 'test assets' (i.e. a basic
// Issuer and Certificate).
// This ensures that the Kubernetes apiserver is ready to serve resources within the
// cert-manager API group.
// If retry is true, the createObj call will be retried if it fails. Otherwise, the
// 'create' operations will only be attempted once.
func (cm *certManagerClient) waitForAPIReady(_ context.Context, retry bool) error {
	log := logf.Log
	// Waits for for the cert-manager to be available.
	if retry {
		log.Info("Waiting for cert-manager to be available...")
	}

	testObjs, err := getTestResourcesManifestObjs()
	if err != nil {
		return err
	}

	for i := range testObjs {
		o := testObjs[i]

		// Create the Kubernetes object.
		// This is wrapped with a retry as the cert-manager API may not be available
		// yet, so we need to keep retrying until it is.
		if err := cm.pollImmediateWaiter(waitCertManagerInterval, cm.getWaitTimeout(), func() (bool, error) {
			if err := cm.createObj(o); err != nil {
				// If retrying is disabled, return the error here.
				if !retry {
					return false, err
				}
				return false, nil
			}
			return true, nil
		}); err != nil {
			return err
		}
	}
	deleteCertManagerBackoff := newWriteBackoff()
	for i := range testObjs {
		obj := testObjs[i]
		if err := retryWithExponentialBackoff(deleteCertManagerBackoff, func() error {
			if err := cm.deleteObj(obj); err != nil {
				// tolerate NotFound errors when deleting the test resources
				if apierrors.IsNotFound(err) {
					return nil
				}
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}
