/*
Copyright 2026 The Kubernetes Authors.

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

package controller

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// ClientWithDeleteResponse is a client that only implements the Delete method.
// This Delete method diverges from the controller-runtime Client because it returns
// the object after deletion when possible.
type ClientWithDeleteResponse interface {
	Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (client.Object, error)
}

// NewClientWithDeleteResponse returns a new ClientWithDeleteResponse.
func NewClientWithDeleteResponse(obj client.Object, objGR schema.GroupResource, scheme *runtime.Scheme, restConfig *rest.Config, httpClient *http.Client) (ClientWithDeleteResponse, error) {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client: failed to get GVK from object")
	}

	restClient, err := apiutil.RESTClientForGVK(gvk, false, false, restConfig,
		serializer.NewCodecFactory(scheme), httpClient)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client: failed to create REST client")
	}

	return &clientWithDeleteResponse{
		objGR:      objGR,
		scheme:     scheme,
		restClient: restClient,
	}, nil
}

type clientWithDeleteResponse struct {
	objGR      schema.GroupResource
	scheme     *runtime.Scheme
	restClient rest.Interface
}

// Delete deletes an object and returns the object after deletion when possible.
// If the object had finalizers at the time of deletion the object is returned with deletionTimestamp / RV set.
// If the object did not have finalizers the object will be gone and the object is not returned.
func (c *clientWithDeleteResponse) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (client.Object, error) {
	deleteOpts := client.DeleteOptions{}
	deleteOpts.ApplyOptions(opts)

	// DeepCopy obj to avoid overwriting the object in Into below.
	// If the object is gone after deletion the apiserver returns metav1.Status
	// and Into below would zero out the obj.
	obj = obj.DeepCopyObject().(client.Object)

	err := c.restClient.Delete().
		NamespaceIfScoped(obj.GetNamespace(), true).
		Resource(c.objGR.Resource).
		Name(obj.GetName()).
		Body(deleteOpts.AsDeleteOptions()).
		Do(ctx).
		Into(obj)
	if err != nil {
		return nil, err
	}

	// If the object had finalizers at the time of deletion the object is returned by the kube-apiserver
	// and obj contains the returned object with deletionTimestamp / RV set.
	if obj.GetResourceVersion() != "" {
		return obj, nil
	}

	return nil, nil
}
