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
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	manifests "sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	embeddedCertManagerManifestPath = "cmd/clusterctl/config/manifest/cert-manager.yaml"

	waitCertManagerInterval = 1 * time.Second
	waitCertManagerTimeout  = 10 * time.Minute

	certManagerImageComponent = "cert-manager"
)

// CertManagerClient has methods to work with cert-manager components in the cluster.
type CertManagerClient interface {
	// EnsureWebhook makes sure the cert-manager Webhook is Available in a cluster:
	// this is a requirement to install a new provider
	EnsureWebhook() error

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
	// Checks if the cert-manager web-hook already exists, if yes, no additional images are required for the web-hook.
	// Nb. we are ignoring the error so this operation can support listing images even if there is no an existing management cluster;
	// in case there is no an existing management cluster, we assume there is no web-hook installed in the cluster.
	hasWebhook, _ := cm.hasWebhook()
	if hasWebhook {
		return []string{}, nil
	}

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

// EnsureWebhook makes sure the cert-manager Web-hook is Available in a cluster:
// this is a requirement to install a new provider
// Nb. In order to provide a simpler out-of-the box experience, the cert-manager manifest
// is embedded in the clusterctl binary.
func (cm *certManagerClient) EnsureWebhook() error {
	log := logf.Log

	// Checks if the cert-manager web-hook already exists, if yes, exit immediately
	hasWebhook, err := cm.hasWebhook()
	if err != nil {
		return err
	}
	if hasWebhook {
		return nil
	}

	// Otherwise install cert-manager
	log.Info("Installing cert-manager")

	// Gets the cert-manager objects from the embedded assets.
	objs, err := cm.getManifestObjs()
	if err != nil {
		return nil
	}

	// installs the web-hook
	createCertManagerBackoff := newBackoff()
	objs = sortResourcesForCreate(objs)
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

	// Waits for for the cert-manager web-hook to be available.
	log.Info("Waiting for cert-manager to be available...")
	if err := cm.pollImmediateWaiter(waitCertManagerInterval, waitCertManagerTimeout, func() (bool, error) {
		webhook, err := cm.getWebhook()
		if err != nil {
			//Nb. we are ignoring the error so the pollImmediateWaiter will execute another retry
			return false, nil
		}
		if webhook == nil {
			return false, nil
		}

		isWebhookAvailable, err := cm.isWebhookAvailable(webhook)
		if err != nil {
			return false, err
		}

		return isWebhookAvailable, nil
	}); err != nil {
		return err
	}

	return nil
}

// getManifestObjs gets the cert-manager manifest, convert to unstructured objects, and fix images
func (cm *certManagerClient) getManifestObjs() ([]unstructured.Unstructured, error) {
	yaml, err := manifests.Asset(embeddedCertManagerManifestPath)
	if err != nil {
		return nil, err
	}

	objs, err := util.ToUnstructured(yaml)
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

	if err = c.Create(ctx, &o); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to create cert-manager component: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
	}
	return nil
}

// getWebhook returns the cert-manager Webhook or nil if it does not exists.
func (cm *certManagerClient) getWebhook() (*unstructured.Unstructured, error) {
	c, err := cm.proxy.NewClient()
	if err != nil {
		return nil, err
	}

	webhook := &unstructured.Unstructured{}
	webhook.SetAPIVersion("apiregistration.k8s.io/v1beta1")
	webhook.SetKind("APIService")
	webhook.SetName("v1beta1.webhook.cert-manager.io")

	key, err := client.ObjectKeyFromObject(webhook)
	if err != nil {
		return nil, err
	}

	err = c.Get(ctx, key, webhook)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, nil
	}

	return webhook, nil
}

// hasWebhook returns true if there is already a web-hook in the cluster
func (cm *certManagerClient) hasWebhook() (bool, error) {
	// Checks if the cert-manager web-hook already exists, if yes, no additional images are required
	webhook, err := cm.getWebhook()
	if err != nil {
		return false, errors.Wrap(err, "failed to check if the cert-manager web-hook exists")
	}
	if webhook != nil {
		return true, nil
	}
	return false, nil
}

// isWebhookAvailable returns true if the cert-manager web-hook has the condition type:Available with status:True.
// This is required to check the web-hook is working and ready to accept requests.
func (cm *certManagerClient) isWebhookAvailable(webhook *unstructured.Unstructured) (bool, error) {
	conditions, found, err := unstructured.NestedSlice(webhook.Object, "status", "conditions")
	if err != nil {
		return false, errors.Wrap(err, "invalid cert-manager web-hook: failed to get conditions")
	}

	// if status.conditions does not exists, we assume the web-hook is still starting
	if !found {
		return false, nil
	}

	// look for the condition with type:Available and status:True or return false
	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			return false, errors.Wrap(err, "invalid cert-manager web-hook: failed to parse conditions")
		}

		conditionType, ok := conditionMap["type"]
		if !ok {
			return false, errors.Wrap(err, "invalid cert-manager web-hook: there are conditions without the type field")
		}

		if conditionType != "Available" {
			continue
		}

		conditionStatus, ok := conditionMap["status"]
		if !ok {
			return false, errors.Wrapf(err, "invalid cert-manager web-hook: there %q condition does not have the status field", "Available")
		}

		if conditionStatus == "True" {
			return true, nil
		}
		return false, nil
	}

	return false, nil
}
