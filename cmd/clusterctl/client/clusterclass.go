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

package client

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

// addClusterClassIfMissing returns a Template that includes the base template and adds any cluster class definitions that
// are references in the template. If the cluster class referenced already exists in the cluster it is not added to the
// template.
func addClusterClassIfMissing(template Template, clusterClassClient repository.ClusterClassClient, clusterClient cluster.Client, targetNamespace string, listVariablesOnly bool) (Template, error) {
	classes, err := clusterClassNamesFromTemplate(template)
	if err != nil {
		return nil, err
	}
	// If the template does not reference any ClusterClass, return early.
	if len(classes) == 0 {
		return template, nil
	}

	clusterClassesTemplate, err := fetchMissingClusterClassTemplates(clusterClassClient, clusterClient, classes, targetNamespace, listVariablesOnly)
	if err != nil {
		return nil, err
	}

	// We intentionally render the ClusterClass before the Cluster resource, as the Cluster
	// is depending on the ClusterClass.
	mergedTemplate, err := repository.MergeTemplates(clusterClassesTemplate, template)
	if err != nil {
		return nil, err
	}

	return mergedTemplate, nil
}

// clusterClassNamesFromTemplate returns the list of cluster classes referenced
// by custers defined in the template. If not clusters are defined in the template
// or if no cluster uses a cluster class it returns an empty list.
func clusterClassNamesFromTemplate(template Template) ([]string, error) {
	classes := []string{}

	// loop thorugh all the objects and if the object is a cluster
	// check and see if cluster.spec.topology.class is defined.
	// If defined, add value to the result.
	for i := range template.Objs() {
		obj := template.Objs()[i]
		if obj.GroupVersionKind().GroupKind() != clusterv1.GroupVersion.WithKind("Cluster").GroupKind() {
			continue
		}
		cluster := &clusterv1.Cluster{}
		if err := scheme.Scheme.Convert(&obj, cluster, nil); err != nil {
			return nil, errors.Wrap(err, "failed to convert object to Cluster")
		}
		if cluster.Spec.Topology == nil {
			continue
		}
		classes = append(classes, cluster.Spec.Topology.Class)
	}
	return classes, nil
}

// fetchMissingClusterClassTemplates returns a list of templates for cluster classes that do not yet exist
// in the cluster. If the cluster is not initialized, all the ClusterClasses are added.
func fetchMissingClusterClassTemplates(clusterClassClient repository.ClusterClassClient, clusterClient cluster.Client, classes []string, targetNamespace string, listVariablesOnly bool) (Template, error) {
	// first check if the cluster is initialized.
	// If it is initialized:
	//    For every ClusterClass check if it already exists in the cluster.
	//    If the ClusterClass already exists there is nothing further to do.
	//    If not, get the ClusterClass from the repository
	// If it is not initialized:
	//    For every ClusterClass fetch the class definition from the repository.

	// Check if the cluster is initialized
	clusterInitialized := false
	var err error
	if err := clusterClient.Proxy().CheckClusterAvailable(); err == nil {
		clusterInitialized, err = clusterClient.ProviderInventory().CheckCAPIInstalled()
		if err != nil {
			return nil, errors.Wrap(err, "failed to check if the cluster is initialized")
		}
	}
	var c client.Client
	if clusterInitialized {
		c, err = clusterClient.Proxy().NewClient()
		if err != nil {
			return nil, err
		}
	}

	// Get the templates for all ClusterClasses and associated objects if the target
	// CluterClass does not exits in the cluster.
	templates := []repository.Template{}
	for _, class := range classes {
		if clusterInitialized {
			exists, err := clusterClassExists(c, class, targetNamespace)
			if err != nil {
				return nil, err
			}
			if exists {
				continue
			}
		}
		// The cluster is either not initialized or the ClusterClass does not yet exist in the cluster.
		// Fetch the cluster class to install.
		clusterClassTemplate, err := clusterClassClient.Get(class, targetNamespace, listVariablesOnly)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get the cluster class template for %q", class)
		}

		// If any of the objects in the ClusterClass template already exist in the cluster then
		// we should error out.
		// We do this to avoid adding partial items from the template in the output YAML. This ensures
		// that we do not add a ClusterClass (and associated objects) who definition is unknown.
		if clusterInitialized {
			for _, obj := range clusterClassTemplate.Objs() {
				if exists, err := objExists(c, obj); err != nil {
					return nil, err
				} else if exists {
					return nil, fmt.Errorf("%s(%s) already exists in the cluster", obj.GetName(), obj.GetObjectKind().GroupVersionKind())
				}
			}
		}
		templates = append(templates, clusterClassTemplate)
	}

	merged, err := repository.MergeTemplates(templates...)
	if err != nil {
		return nil, err
	}

	return merged, nil
}

func clusterClassExists(c client.Client, class, targetNamespace string) (bool, error) {
	clusterClass := &clusterv1.ClusterClass{}
	if err := c.Get(context.TODO(), client.ObjectKey{Name: class, Namespace: targetNamespace}, clusterClass); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed to check if ClusterClass %q exists in the cluster", class)
	}
	return true, nil
}

func objExists(c client.Client, obj unstructured.Unstructured) (bool, error) {
	o := obj.DeepCopy()
	if err := c.Get(context.TODO(), client.ObjectKeyFromObject(o), o); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
