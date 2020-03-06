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

package util

import (
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

const (
	deploymentKind          = "Deployment"
	controllerContainerName = "manager"
)

// InspectImages identifies the container images required to install the objects defined in the objs.
// NB. The implemented approach is specific for the provider components YAML & for the cert-manager manifest; it is not
// intended to cover all the possible objects used to deploy containers existing in Kubernetes.
func InspectImages(objs []unstructured.Unstructured) ([]string, error) {
	images := []string{}

	for i := range objs {
		o := objs[i]
		if o.GetKind() == deploymentKind {
			d := &appsv1.Deployment{}
			if err := scheme.Scheme.Convert(&o, d, nil); err != nil {
				return nil, err
			}

			for _, c := range d.Spec.Template.Spec.Containers {
				images = append(images, c.Image)
			}

			for _, c := range d.Spec.Template.Spec.InitContainers {
				images = append(images, c.Image)
			}
		}
	}

	return images, nil
}

// IsClusterResource returns true if the resource kind is cluster wide (not namespaced).
func IsClusterResource(kind string) bool {
	return !IsResourceNamespaced(kind)
}

// IsResourceNamespaced returns true if the resource kind is namespaced.
func IsResourceNamespaced(kind string) bool {
	switch kind {
	case "Namespace",
		"Node",
		"PersistentVolume",
		"PodSecurityPolicy",
		"CertificateSigningRequest",
		"ClusterRoleBinding",
		"ClusterRole",
		"VolumeAttachment",
		"StorageClass",
		"CSIDriver",
		"CSINode",
		"ValidatingWebhookConfiguration",
		"MutatingWebhookConfiguration",
		"CustomResourceDefinition",
		"PriorityClass",
		"RuntimeClass":
		return false
	default:
		return true
	}
}

// IsSharedResource returns true if the resource lifecycle is shared.
func IsSharedResource(o unstructured.Unstructured) bool {
	lifecycle, ok := o.GetLabels()[clusterctlv1.ClusterctlResourceLifecyleLabelName]
	if !ok {
		return false
	}
	if lifecycle == string(clusterctlv1.ResourceLifecycleShared) {
		return true
	}
	return false
}

// FixImages alters images using the give alter func
// NB. The implemented approach is specific for the provider components YAML & for the cert-manager manifest; it is not
// intended to cover all the possible objects used to deploy containers existing in Kubernetes.
func FixImages(objs []unstructured.Unstructured, alterImageFunc func(image string) (string, error)) ([]unstructured.Unstructured, error) {
	// look for resources of kind Deployment and alter the image
	for i := range objs {
		o := &objs[i]
		if o.GetKind() != deploymentKind {
			continue
		}

		// Convert Unstructured into a typed object
		d := &appsv1.Deployment{}
		if err := scheme.Scheme.Convert(o, d, nil); err != nil {
			return nil, err
		}

		// Alter the image
		for j := range d.Spec.Template.Spec.Containers {
			container := d.Spec.Template.Spec.Containers[j]
			image, err := alterImageFunc(container.Image)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to fix image for container %s in deployment %s", container.Name, d.Name)
			}
			container.Image = image
			d.Spec.Template.Spec.Containers[j] = container
		}

		for j := range d.Spec.Template.Spec.InitContainers {
			container := d.Spec.Template.Spec.InitContainers[j]
			image, err := alterImageFunc(container.Image)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to fix image for init container %s in deployment %s", container.Name, d.Name)
			}
			container.Image = image
			d.Spec.Template.Spec.InitContainers[j] = container
		}

		// Convert typed object back to Unstructured
		if err := scheme.Scheme.Convert(d, o, nil); err != nil {
			return nil, err
		}
		objs[i] = *o
	}
	return objs, nil
}
