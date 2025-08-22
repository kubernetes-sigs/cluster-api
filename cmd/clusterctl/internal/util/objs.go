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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

const (
	deploymentKind          = "Deployment"
	daemonSetKind           = "DaemonSet"
	controllerContainerName = "manager"
)

// InspectImages identifies the container images required to install the objects defined in the objs.
// NB. The implemented approach is specific for the provider components YAML & for the cert-manager manifest; it is not
// intended to cover all the possible objects used to deploy containers existing in Kubernetes.
func InspectImages(objs []unstructured.Unstructured) ([]string, error) {
	images := []string{}

	for i := range objs {
		o := objs[i]

		var podSpec corev1.PodSpec

		switch o.GetKind() {
		case deploymentKind:
			d := &appsv1.Deployment{}
			if err := scheme.Scheme.Convert(&o, d, nil); err != nil {
				return nil, err
			}
			podSpec = d.Spec.Template.Spec
		case daemonSetKind:
			d := &appsv1.DaemonSet{}
			if err := scheme.Scheme.Convert(&o, d, nil); err != nil {
				return nil, err
			}
			podSpec = d.Spec.Template.Spec
		default:
			continue
		}

		for _, c := range podSpec.Containers {
			images = append(images, c.Image)
		}

		for _, c := range podSpec.InitContainers {
			images = append(images, c.Image)
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

// FixImages alters images using the give alter func
// NB. The implemented approach is specific for the provider components YAML & for the cert-manager manifest; it is not
// intended to cover all the possible objects used to deploy containers existing in Kubernetes.
func FixImages(objs []unstructured.Unstructured, alterImageFunc func(image string) (string, error)) ([]unstructured.Unstructured, error) {
	for i := range objs {
		if err := fixDeploymentImages(&objs[i], alterImageFunc); err != nil {
			return nil, err
		}
		if err := fixDaemonSetImages(&objs[i], alterImageFunc); err != nil {
			return nil, err
		}
	}
	return objs, nil
}

func fixDeploymentImages(o *unstructured.Unstructured, alterImageFunc func(image string) (string, error)) error {
	if o.GetKind() != deploymentKind {
		return nil
	}

	// Convert Unstructured into a typed object
	d := &appsv1.Deployment{}
	if err := scheme.Scheme.Convert(o, d, nil); err != nil {
		return err
	}

	if err := fixPodSpecImages(&d.Spec.Template.Spec, alterImageFunc); err != nil {
		return errors.Wrapf(err, "failed to fix containers in deployment %s", d.Name)
	}

	// Convert typed object back to Unstructured
	return scheme.Scheme.Convert(d, o, nil)
}

func fixDaemonSetImages(o *unstructured.Unstructured, alterImageFunc func(image string) (string, error)) error {
	if o.GetKind() != daemonSetKind {
		return nil
	}

	// Convert Unstructured into a typed object
	d := &appsv1.DaemonSet{}
	if err := scheme.Scheme.Convert(o, d, nil); err != nil {
		return err
	}

	if err := fixPodSpecImages(&d.Spec.Template.Spec, alterImageFunc); err != nil {
		return errors.Wrapf(err, "failed to fix containers in deamonSet %s", d.Name)
	}
	// Convert typed object back to Unstructured
	return scheme.Scheme.Convert(d, o, nil)
}

func fixPodSpecImages(podSpec *corev1.PodSpec, alterImageFunc func(image string) (string, error)) error {
	if err := fixContainersImage(podSpec.Containers, alterImageFunc); err != nil {
		return errors.Wrapf(err, "failed to fix containers")
	}
	if err := fixContainersImage(podSpec.InitContainers, alterImageFunc); err != nil {
		return errors.Wrapf(err, "failed to fix init containers")
	}
	return nil
}

func fixContainersImage(containers []corev1.Container, alterImageFunc func(image string) (string, error)) error {
	for j := range containers {
		container := &containers[j]
		image, err := alterImageFunc(container.Image)
		if err != nil {
			return errors.Wrapf(err, "failed to fix image for container %s", container.Name)
		}
		container.Image = image
	}
	return nil
}

// IsDeploymentWithManager return true if obj is a deployment containing a pod with at least one container named 'manager',
// that according to the clusterctl contract, identifies the provider's controller.
func IsDeploymentWithManager(obj unstructured.Unstructured) bool {
	if obj.GroupVersionKind().Kind == deploymentKind {
		var dep appsv1.Deployment
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &dep); err != nil {
			return false
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.Name == controllerContainerName {
				return true
			}
		}
	}
	return false
}
