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
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/version"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	manifests "sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	utilresource "sigs.k8s.io/cluster-api/util/resource"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

const (
	embeddedCertManagerManifestPath              = "cmd/clusterctl/config/assets/cert-manager.yaml"
	embeddedCertManagerTestResourcesManifestPath = "cmd/clusterctl/config/assets/cert-manager-test-resources.yaml"

	waitCertManagerInterval       = 1 * time.Second
	waitCertManagerDefaultTimeout = 10 * time.Minute

	certManagerImageComponent = "cert-manager"
	timeoutConfigKey          = "cert-manager-timeout"

	certmanagerVersionAnnotation = "certmanager.clusterctl.cluster.x-k8s.io/version"
	certmanagerHashAnnotation    = "certmanager.clusterctl.cluster.x-k8s.io/hash"

	certmanagerVersionLabel = "helm.sh/chart"
)

// CertManagerUpgradePlan defines the upgrade plan if cert-manager needs to be
// upgraded to a different version.
type CertManagerUpgradePlan struct {
	From, To      string
	ShouldUpgrade bool
}

// CertManagerClient has methods to work with cert-manager components in the cluster.
type CertManagerClient interface {
	// EnsureInstalled makes sure cert-manager is running and its API is available.
	// This is required to install a new provider.
	EnsureInstalled() error

	// EnsureLatestVersion checks the cert-manager version currently installed, and if it is
	// older than the version currently embedded in clusterctl, upgrades it.
	EnsureLatestVersion() error

	// PlanUpgrade retruns a CertManagerUpgradePlan with information regarding
	// a cert-manager upgrade if necessary.
	PlanUpgrade() (CertManagerUpgradePlan, error)

	// Images return the list of images required for installing the cert-manager.
	Images() ([]string, error)
}

// certManagerClient implements CertManagerClient .
type certManagerClient struct {
	configClient                       config.Client
	proxy                              Proxy
	pollImmediateWaiter                PollImmediateWaiter
	embeddedCertManagerManifestVersion string
	embeddedCertManagerManifestHash    string
}

// Ensure certManagerClient implements the CertManagerClient interface.
var _ CertManagerClient = &certManagerClient{}

func (cm *certManagerClient) setManifestHash() error {
	yamlData, err := manifests.Asset(embeddedCertManagerManifestPath)
	if err != nil {
		return err
	}
	cm.embeddedCertManagerManifestHash = fmt.Sprintf("%x", sha256.Sum256(yamlData))
	return nil
}

func (cm *certManagerClient) setManifestVersion() error {
	// Gets the cert-manager objects from the embedded assets.
	objs, err := cm.getManifestObjs()
	if err != nil {
		return err
	}
	found := false

	for i := range objs {
		o := objs[i]
		if o.GetKind() == "CustomResourceDefinition" {
			labels := o.GetLabels()
			version, ok := labels[certmanagerVersionLabel]
			if ok {
				s := strings.Split(version, "-")
				cm.embeddedCertManagerManifestVersion = s[2]
				found = true
				break
			}
		}
	}
	if !found {
		return errors.Errorf("Failed to detect cert-manager version by searching for label %s in all CRDs", certmanagerVersionLabel)
	}
	return nil
}

// newCertManagerClient returns a certManagerClient.
func newCertManagerClient(configClient config.Client, proxy Proxy, pollImmediateWaiter PollImmediateWaiter) (*certManagerClient, error) {
	cm := &certManagerClient{
		configClient:        configClient,
		proxy:               proxy,
		pollImmediateWaiter: pollImmediateWaiter,
	}
	err := cm.setManifestVersion()
	if err != nil {
		return nil, err
	}

	err = cm.setManifestHash()
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// Images return the list of images required for installing the cert-manager.
func (cm *certManagerClient) Images() ([]string, error) {
	// Gets the cert-manager objects from the embedded assets.
	objs, err := cm.getManifestObjs()
	if err != nil {
		return []string{}, nil
	}

	images, err := util.InspectImages(objs)
	if err != nil {
		return nil, err
	}
	return images, nil
}

// EnsureInstalled makes sure cert-manager is running and its API is available.
// This is required to install a new provider.
// Nb. In order to provide a simpler out-of-the box experience, the cert-manager manifest
// is embedded in the clusterctl binary.
func (cm *certManagerClient) EnsureInstalled() error {
	log := logf.Log

	// Skip re-installing cert-manager if the API is already available
	if err := cm.waitForAPIReady(ctx, false); err == nil {
		log.Info("Skipping installing cert-manager as it is already installed")
		return nil
	}

	log.Info("Installing cert-manager", "Version", cm.embeddedCertManagerManifestVersion)
	return cm.install()
}

func (cm *certManagerClient) install() error {
	// Gets the cert-manager objects from the embedded assets.
	objs, err := cm.getManifestObjs()
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
	if err := cm.waitForAPIReady(ctx, true); err != nil {
		return err
	}

	return nil
}

// PlanUpgrade retruns a CertManagerUpgradePlan with information regarding
// a cert-manager upgrade if necessary.
func (cm *certManagerClient) PlanUpgrade() (CertManagerUpgradePlan, error) {
	log := logf.Log
	log.Info("Checking cert-manager version...")

	objs, err := cm.proxy.ListResources(map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"}, "cert-manager")
	if err != nil {
		return CertManagerUpgradePlan{}, errors.Wrap(err, "failed get cert manager components")
	}

	currentVersion, shouldUpgrade, err := cm.shouldUpgrade(objs)
	if err != nil {
		return CertManagerUpgradePlan{}, err
	}

	return CertManagerUpgradePlan{
		From:          currentVersion,
		To:            cm.embeddedCertManagerManifestVersion,
		ShouldUpgrade: shouldUpgrade,
	}, nil
}

// EnsureLatestVersion checks the cert-manager version currently installed, and if it is
// older than the version currently embedded in clusterctl, upgrades it.
func (cm *certManagerClient) EnsureLatestVersion() error {
	log := logf.Log
	log.Info("Checking cert-manager version...")

	objs, err := cm.proxy.ListResources(map[string]string{clusterctlv1.ClusterctlCoreLabelName: "cert-manager"}, "cert-manager")
	if err != nil {
		return errors.Wrap(err, "failed get cert manager components")
	}

	currentVersion, shouldUpgrade, err := cm.shouldUpgrade(objs)
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

	// install the cert-manager version embedded in clusterctl
	log.Info("Installing cert-manager", "Version", cm.embeddedCertManagerManifestVersion)
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

func (cm *certManagerClient) shouldUpgrade(objs []unstructured.Unstructured) (string, bool, error) {
	needUpgrade := false
	currentVersion := ""
	for i := range objs {
		obj := objs[i]

		// Endpoints are generated by Kubernetes without the version annotation, so we are skipping them
		if obj.GetKind() == "Endpoints" {
			continue
		}

		// if no version then upgrade (v0.11.0)
		objVersion, ok := obj.GetAnnotations()[certmanagerVersionAnnotation]
		if !ok {
			// if there is no version annotation, this means the obj is cert-manager v0.11.0 (installed with older version of clusterctl)
			currentVersion = "v0.11.0"
			needUpgrade = true
			break
		}

		objSemVersion, err := version.ParseSemantic(objVersion)
		if err != nil {
			return "", false, errors.Wrapf(err, "failed to parse version for cert-manager component %s/%s", obj.GetKind(), obj.GetName())
		}

		c, err := objSemVersion.Compare(cm.embeddedCertManagerManifestVersion)
		if err != nil {
			return "", false, errors.Wrapf(err, "failed to compare version for cert-manager component %s/%s", obj.GetKind(), obj.GetName())
		}

		switch {
		case c < 0:
			// if version < current, then upgrade
			currentVersion = objVersion
			needUpgrade = true
		case c == 0:
			// if version == current, check the manifest hash; if it does not exists or if it is different, then upgrade
			objHash, ok := obj.GetAnnotations()[certmanagerHashAnnotation]
			if !ok || objHash != cm.embeddedCertManagerManifestHash {
				currentVersion = fmt.Sprintf("%s (%s)", objVersion, objHash)
				needUpgrade = true
				break
			}
			// otherwise we are already at the latest version
			currentVersion = objVersion
		case c > 0:
			// the installed version is higher than the one embedded in clusterctl, so we are ok
			currentVersion = objVersion
		}

		if needUpgrade {
			break
		}
	}
	return currentVersion, needUpgrade, nil
}

func (cm *certManagerClient) getWaitTimeout() time.Duration {
	log := logf.Log

	timeout, err := cm.configClient.Variables().Get(timeoutConfigKey)
	if err != nil {
		return waitCertManagerDefaultTimeout
	}
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		log.Info("Invalid value set for ", timeoutConfigKey, timeout)
		return waitCertManagerDefaultTimeout
	}
	return timeoutDuration
}

// getManifestObjs gets the cert-manager manifest, convert to unstructured objects, and fix images
func (cm *certManagerClient) getManifestObjs() ([]unstructured.Unstructured, error) {
	yaml, err := manifests.Asset(embeddedCertManagerManifestPath)
	if err != nil {
		return nil, err
	}

	objs, err := utilyaml.ToUnstructured(yaml)

	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml for cert-manager manifest")
	}

	objs, err = util.FixImages(objs, func(image string) (string, error) {
		return cm.configClient.ImageMeta().AlterImage(certManagerImageComponent, image)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to apply image override to the cert-manager manifest")
	}

	return objs, nil
}

// getTestResourcesManifestObjs gets the cert-manager test manifests, converted to unstructured objects.
// These are used to ensure the cert-manager API components are all ready and the API is available for use.
func getTestResourcesManifestObjs() ([]unstructured.Unstructured, error) {
	yaml, err := manifests.Asset(embeddedCertManagerTestResourcesManifestPath)
	if err != nil {
		return nil, err
	}

	objs, err := utilyaml.ToUnstructured(yaml)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml for cert-manager test resources manifest")
	}

	return objs, nil
}

func (cm *certManagerClient) createObj(obj unstructured.Unstructured) error {
	log := logf.Log

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[clusterctlv1.ClusterctlCoreLabelName] = "cert-manager"
	obj.SetLabels(labels)

	// persist version marker information as annotations to avoid character and length
	// restrictions on label values.
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	// persist the version number of stored resources to make a
	// future enhancement to add upgrade support possible.
	annotations[certmanagerVersionAnnotation] = cm.embeddedCertManagerManifestVersion
	annotations[certmanagerHashAnnotation] = cm.embeddedCertManagerManifestHash
	obj.SetAnnotations(annotations)

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

	if err := cl.Delete(ctx, &obj); err != nil {
		return err
	}

	return nil
}

// waitForAPIReady will attempt to create the cert-manager 'test assets' (i.e. a basic
// Issuer and Certificate).
// This ensures that the Kubernetes apiserver is ready to serve resources within the
// cert-manager API group.
// If retry is true, the createObj call will be retried if it fails. Otherwise, the
// 'create' operations will only be attempted once.
func (cm *certManagerClient) waitForAPIReady(ctx context.Context, retry bool) error {
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
