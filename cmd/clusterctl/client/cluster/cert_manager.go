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
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	manifests "sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	utilresource "sigs.k8s.io/cluster-api/util/resource"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

var (
	// This is declared as a variable mapping a version number to the hash of the
	// embedded cert-manager.yaml file.
	// The hash is used to ensure that when the cert-manager.yaml file is updated,
	// the version number marker here is _also_ updated.
	// If the cert-manager.yaml asset is modified, this line **MUST** be updated
	// accordingly else future upgrades of the cert-manager component will not
	// be possible, as there'll be no record of the version installed.
	// You can either generate the SHA256 hash of the file, or alternatively
	// run `go test` against this package. THe Test_VersionMarkerUpToDate will output
	// the expected hash if it does not match the hash here.
	embeddedCertManagerManifestVersion = map[string]string{"v0.16.0": "5770f5f01c10a902355b3522b8ce44508ebb6ec88955efde9a443afe5b3969d7"}
)

const (
	embeddedCertManagerManifestPath              = "cmd/clusterctl/config/assets/cert-manager.yaml"
	embeddedCertManagerTestResourcesManifestPath = "cmd/clusterctl/config/assets/cert-manager-test-resources.yaml"

	waitCertManagerInterval       = 1 * time.Second
	waitCertManagerDefaultTimeout = 10 * time.Minute

	certManagerImageComponent = "cert-manager"
	timeoutConfigKey          = "cert-manager-timeout"
)

// CertManagerClient has methods to work with cert-manager components in the cluster.
type CertManagerClient interface {
	// EnsureInstalled makes sure cert-manager is running and its API is available.
	// This is required to install a new provider.
	EnsureInstalled() error

	// Images return the list of images required for installing the cert-manager.
	Images() ([]string, error)
}

// certManagerClient implements CertManagerClient .
type certManagerClient struct {
	configClient        config.Client
	proxy               Proxy
	pollImmediateWaiter PollImmediateWaiter
}

// Ensure certManagerClient implements the CertManagerClient interface.
var _ CertManagerClient = &certManagerClient{}

// newCertMangerClient returns a certManagerClient.
func newCertMangerClient(configClient config.Client, proxy Proxy, pollImmediateWaiter PollImmediateWaiter) *certManagerClient {
	return &certManagerClient{
		configClient:        configClient,
		proxy:               proxy,
		pollImmediateWaiter: pollImmediateWaiter,
	}
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

	// Gets the cert-manager objects from the embedded assets.
	objs, err := cm.getManifestObjs()
	if err != nil {
		return err
	}

	log.Info("Installing cert-manager")

	// Install all cert-manager manifests
	createCertManagerBackoff := newWriteBackoff()
	objs = utilresource.SortForCreate(objs)
	for i := range objs {
		o := objs[i]
		log.V(5).Info("Creating", logf.UnstructuredToValues(o)...)

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

func (cm *certManagerClient) createObj(o unstructured.Unstructured) error {
	c, err := cm.proxy.NewClient()
	if err != nil {
		return err
	}

	labels := o.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[clusterctlv1.ClusterctlCoreLabelName] = "cert-manager"
	o.SetLabels(labels)

	// persist version marker information as annotations to avoid character and length
	// restrictions on label values.
	annotations := o.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	// persist the version number of stored resources to make a
	// future enhancement to add upgrade support possible.
	version, hash := embeddedCertManagerVersion()
	annotations["certmanager.clusterctl.cluster.x-k8s.io/version"] = version
	annotations["certmanager.clusterctl.cluster.x-k8s.io/hash"] = hash
	o.SetAnnotations(annotations)

	if err = c.Create(ctx, &o); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to create cert-manager component: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
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
	// Waits for for the cert-manager web-hook to be available.
	log.Info("Waiting for cert-manager to be available...")

	testObjs, err := getTestResourcesManifestObjs()
	if err != nil {
		return err
	}

	for i := range testObjs {
		o := testObjs[i]
		log.V(5).Info("Creating", logf.UnstructuredToValues(o)...)

		// Create the Kubernetes object.
		// This is wrapped with a retry as the cert-manager API may not be available
		// yet, so we need to keep retrying until it is.
		if err := cm.pollImmediateWaiter(waitCertManagerInterval, cm.getWaitTimeout(), func() (bool, error) {
			if err := cm.createObj(o); err != nil {
				log.V(5).Info("Failed to create cert-manager test resource", logf.UnstructuredToValues(o)...)

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
	for i := range testObjs {
		obj := testObjs[i]
		if err := cm.deleteObj(obj); err != nil {
			// tolerate NotFound errors when deleting the test resources
			if apierrors.IsNotFound(err) {
				continue
			}
			log.V(5).Info("Failed to delete cert-manager test resource", logf.UnstructuredToValues(obj)...)
			return err
		}
	}

	return nil
}

// returns the version number and hash of the cert-manager manifest embedded
// into the clusterctl binary
func embeddedCertManagerVersion() (version, hash string) {
	if len(embeddedCertManagerManifestVersion) != 1 {
		panic("embeddedCertManagerManifestVersion MUST only have one entry")
	}
	for version, hash := range embeddedCertManagerManifestVersion {
		return version, hash
	}
	// unreachable
	return "", ""
}
