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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	embeddedCertManagerManifestPath = "cmd/clusterctl/config/manifest/cert-manager.yaml"

	waitCertManagerInterval = 1 * time.Second
	waitCertManagerTimeout  = 10 * time.Minute
)

// CertMangerClient has methods to work with cert-manager components in the cluster.
type CertMangerClient interface {
	// EnsureWebHook makes sure the cert-manager WebHook is Available in a cluster:
	// this is a requirement to install a new provider
	EnsureWebHook() error
}

// certMangerClient implements CertMangerClient .
type certMangerClient struct {
	proxy Proxy
}

// Ensure certMangerClient implements the CertMangerClient interface.
var _ CertMangerClient = &certMangerClient{}

// newCertMangerClient returns a certMangerClient.
func newCertMangerClient(proxy Proxy) *certMangerClient {
	return &certMangerClient{
		proxy: proxy,
	}
}

// EnsureWebHook makes sure the cert-manager WebHook is Available in a cluster:
// this is a requirement to install a new provider
// Nb. In order to provide a simpler out-of-the box experience, the cert-manager manifest
// is embedded in the clusterctl binary.
func (cm *certMangerClient) EnsureWebHook() error {
	c, err := cm.proxy.NewClient()
	if err != nil {
		return err
	}

	// Checks if the cert-manager WebHook already exists, if yes, exit immediately
	webHook, err := cm.getWebHook(c)
	if err != nil {
		return errors.Wrap(err, "failed to check if the cert-manager WebHook exists")
	}
	if webHook != nil {
		return nil
	}

	// Gets the cert-manager manifest from the embedded assets and apply it.
	yaml, err := config.Asset(embeddedCertManagerManifestPath)
	if err != nil {
		return err
	}

	objs, err := util.ToUnstructured(yaml)
	if err != nil {
		return errors.Wrap(err, "failed to parse yaml for cert-manager manifest")
	}

	objs = sortResourcesForCreate(objs)
	for _, o := range objs {
		klog.V(3).Infof("Creating: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		if err = c.Create(ctx, &o); err != nil { //nolint
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return errors.Wrapf(err, "failed to create cert-manager component: %s, %s/%s", o.GroupVersionKind(), o.GetNamespace(), o.GetName())
		}
	}

	// Waits for for the cert-manager WebHook to be available.
	if err := wait.PollImmediate(waitCertManagerInterval, waitCertManagerTimeout, func() (bool, error) {
		webHook, err := cm.getWebHook(c)
		if err != nil {
			return false, errors.Wrap(err, "failed to get the cert-manager WebHook")
		}
		if webHook == nil {
			return false, nil
		}

		isWebHookAvailable, err := cm.isWebHookAvailable(webHook)
		if err != nil {
			return false, err
		}

		return isWebHookAvailable, nil
	}); err != nil {
		return errors.Wrapf(err, "failed to scale deployment")
	}

	return nil
}

// getWebHook returns the cert-manager WebHook or nil if it does not exists.
func (cm *certMangerClient) getWebHook(c client.Client) (*unstructured.Unstructured, error) {
	webHook := &unstructured.Unstructured{}
	webHook.SetAPIVersion("apiregistration.k8s.io/v1beta1")
	webHook.SetKind("APIService")
	webHook.SetName("v1beta1.webhook.cert-manager.io")

	key, err := client.ObjectKeyFromObject(webHook)
	if err != nil {
		return nil, err
	}

	err = c.Get(ctx, key, webHook)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, nil
	}

	return webHook, nil
}

// isWebHookAvailable returns true if the cert-manager WebHook has the condition type:Available with status:True.
// This is required to check the WebHook is working and ready to accept requests.
func (cm *certMangerClient) isWebHookAvailable(webHook *unstructured.Unstructured) (bool, error) {
	conditions, found, err := unstructured.NestedSlice(webHook.Object, "status", "conditions")
	if err != nil {
		return false, errors.Wrap(err, "invalid cert-manager WebHook: failed to get conditions")
	}

	// if status.conditions does not exists, we assume the WebHook is still starting
	if !found {
		return false, nil
	}

	// look for the condition with type:Available and status:True or return false
	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			return false, errors.Wrap(err, "invalid cert-manager WebHook: failed to parse conditions")
		}

		conditionType, ok := conditionMap["type"]
		if !ok {
			return false, errors.Wrap(err, "invalid cert-manager WebHook: there are conditions without the type field")
		}

		if conditionType != "Available" {
			continue
		}

		conditionStatus, ok := conditionMap["status"]
		if !ok {
			return false, errors.Wrapf(err, "invalid cert-manager WebHook: there %q condition does not have the status field", "Available")
		}

		if conditionStatus == "True" {
			return true, nil
		}
		return false, nil
	}

	return false, nil
}
