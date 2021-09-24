/*
Copyright 2021 The Kubernetes Authors.

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
	"fmt"
	"strings"

	"github.com/gobuffalo/flect"
	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster/internal/dryrun"
	"sigs.k8s.io/cluster-api/controllers/topology"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TopologyClient has methods to work with ClusterClass and ManagedTopologies.
type TopologyClient interface {
	DryRun(in *DryRunInput) (*DryRunOutput, error)
}

// topologyClient implements TopologyClient.
type topologyClient struct {
}

// ensure topologyClient implements TopologyClient.
var _ TopologyClient = &topologyClient{}

// newTopologyClient returns a TopologyClient.
func newTopologyClient() TopologyClient {
	return &topologyClient{}
}

// DryRunInput defines the input for DryRun.
type DryRunInput struct {
	File []byte
}

// DryRunOutput defines the output for DryRun.
type DryRunOutput struct {
	Objs []unstructured.Unstructured
}

func (t *topologyClient) DryRun(in *DryRunInput) (*DryRunOutput, error) {
	// Transform the File in a list of unstructured objects.
	uObjs, err := utilyaml.ToUnstructured(in.File)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse yaml file")
	}

	request := ctrl.Request{}
	crds := map[string]bool{}
	objs := make([]client.Object, 0, len(uObjs))
	for i := range uObjs {
		obj := &uObjs[i]

		// If the object does not have a namespace, set it mimicking kubectl behaviour
		// when you apply objects and your context has namespace set to empty or default.
		if obj.GetNamespace() == "" {
			obj.SetNamespace(metav1.NamespaceDefault)
		}

		// If the object is a ClusterClass, applies defaults.
		if obj.GetObjectKind().GroupVersionKind() == clusterv1.GroupVersion.WithKind("ClusterClass") {
			cc := &clusterv1.ClusterClass{}
			if err := localScheme.Convert(obj, cc, nil); err != nil {
				return nil, errors.Wrap(err, "failed to convert object to ClusterClass")
			}
			cc.Default()
			if err := localScheme.Convert(cc, obj, nil); err != nil {
				return nil, errors.Wrap(err, "failed to convert ClusterClass to object")
			}
		}

		// Add the object to the list of objects to be created.
		objs = append(objs, obj)

		// If the object is provider object, create a CRD thus making the fake environment ready
		// for UpdateReferenceAPIContract to work.
		// NOTE: CRD should be created only once.
		gvk := obj.GetObjectKind().GroupVersionKind()
		if strings.HasSuffix(gvk.Group, ".cluster.x-k8s.io") && !crds[gvk.String()] {
			crd := &apiextensionsv1.CustomResourceDefinition{
				TypeMeta: metav1.TypeMeta{
					APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
					Kind:       "CustomResourceDefinition",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s.%s", flect.Pluralize(strings.ToLower(gvk.Kind)), gvk.Group),
					Labels: map[string]string{
						clusterv1.GroupVersion.String(): "v1beta1",
					},
				},
			}
			objs = append(objs, crd)
			crds[gvk.String()] = true
		}

		// If the object is the Cluster, built the reconcile request.
		if objs[i].GetObjectKind().GroupVersionKind() == clusterv1.GroupVersion.WithKind("Cluster") {
			request.Namespace = objs[i].GetNamespace()
			request.Name = objs[i].GetName()
		}
	}

	// TODO: implement support for reading objects from the Cluster, thus allow dry running more use cases:
	//   rebase, change of variables etc. etc.

	// TODO: add validation in case there is no Cluster  no request

	// Create a DryRun client with the object to test.
	client := dryrun.NewClient(objs)

	// Create the topology reconciler using the DryRun client.
	// TODO: make it possible to pass an externalTracker (or make the external tracker not mandatory) thus
	//   allowing to dry run follow up requests.
	reconciler := topology.ClusterReconciler{
		Client:                    client,
		APIReader:                 client,
		UnstructuredCachingClient: client,
	}

	// Trigger reconcile
	if _, err = reconciler.Reconcile(context.TODO(), request); err != nil {
		return nil, errors.Wrap(err, "failed to dry run the topology controller")
	}

	// Retrieve the list of modified objects.
	modified, err := client.ListModified(context.TODO())
	if err != nil {
		return nil, err
	}

	return &DryRunOutput{
		Objs: modified,
	}, err
}
