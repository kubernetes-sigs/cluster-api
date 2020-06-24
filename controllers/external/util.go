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

package external

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

const (
	// TemplateSuffix is the object kind suffix used by infrastructure references associated
	// with MachineSet or MachineDeployments.
	TemplateSuffix = "Template"
)

// Get uses the client and reference to get an external, unstructured object.
func Get(ctx context.Context, c client.Client, ref *corev1.ObjectReference, namespace string) (*unstructured.Unstructured, error) {
	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetKind(ref.Kind)
	obj.SetName(ref.Name)
	key := client.ObjectKey{Name: obj.GetName(), Namespace: namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s external object %q/%q", obj.GetKind(), key.Namespace, key.Name)
	}
	return obj, nil
}

type CloneTemplateInput struct {
	// Client is the controller runtime client.
	// +required
	Client client.Client

	// TemplateRef is a reference to the template that needs to be cloned.
	// +required
	TemplateRef *corev1.ObjectReference

	// Namespace is the Kubernetes namespace the cloned object should be created into.
	// +required
	Namespace string

	// ClusterName is the cluster this object is linked to.
	// +required
	ClusterName string

	// OwnerRef is an optional OwnerReference to attach to the cloned object.
	// +optional
	OwnerRef *metav1.OwnerReference

	// Labels is an optional map of labels to be added to the object.
	// +optional
	Labels map[string]string
}

// CloneTemplate uses the client and the reference to create a new object from the template.
func CloneTemplate(ctx context.Context, in *CloneTemplateInput) (*corev1.ObjectReference, error) {
	from, err := Get(ctx, in.Client, in.TemplateRef, in.Namespace)
	if err != nil {
		return nil, err
	}
	generateTemplateInput := &GenerateTemplateInput{
		Template:    from,
		TemplateRef: in.TemplateRef,
		Namespace:   in.Namespace,
		ClusterName: in.ClusterName,
		OwnerRef:    in.OwnerRef,
		Labels:      in.Labels,
	}
	to, err := GenerateTemplate(generateTemplateInput)
	if err != nil {
		return nil, err
	}

	// Create the external clone.
	if err := in.Client.Create(context.Background(), to); err != nil {
		return nil, err
	}

	return GetObjectReference(to), nil
}

// GenerateTemplate input is everything needed to generate a new template.
type GenerateTemplateInput struct {
	// Template is the TemplateRef turned into an unstructured.
	// +required
	Template *unstructured.Unstructured

	// TemplateRef is a reference to the template that needs to be cloned.
	// +required
	TemplateRef *corev1.ObjectReference

	// Namespace is the Kubernetes namespace the cloned object should be created into.
	// +required
	Namespace string

	// ClusterName is the cluster this object is linked to.
	// +required
	ClusterName string

	// OwnerRef is an optional OwnerReference to attach to the cloned object.
	// +optional
	OwnerRef *metav1.OwnerReference

	// Labels is an optional map of labels to be added to the object.
	// +optional
	Labels map[string]string
}

func GenerateTemplate(in *GenerateTemplateInput) (*unstructured.Unstructured, error) {
	template, found, err := unstructured.NestedMap(in.Template.Object, "spec", "template")
	if !found {
		return nil, errors.Errorf("missing Spec.Template on %v %q", in.Template.GroupVersionKind(), in.Template.GetName())
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve Spec.Template map on %v %q", in.Template.GroupVersionKind(), in.Template.GetName())
	}

	// Create the unstructured object from the template.
	to := &unstructured.Unstructured{Object: template}
	to.SetResourceVersion("")
	to.SetFinalizers(nil)
	to.SetUID("")
	to.SetSelfLink("")
	to.SetName(names.SimpleNameGenerator.GenerateName(in.Template.GetName() + "-"))
	to.SetNamespace(in.Namespace)

	if to.GetAnnotations() == nil {
		to.SetAnnotations(map[string]string{})
	}
	annotations := to.GetAnnotations()
	annotations[clusterv1.TemplateClonedFromNameAnnotation] = in.TemplateRef.Name
	annotations[clusterv1.TemplateClonedFromGroupKindAnnotation] = in.TemplateRef.GroupVersionKind().GroupKind().String()
	to.SetAnnotations(annotations)

	// Set labels.
	labels := to.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for key, value := range in.Labels {
		labels[key] = value
	}
	labels[clusterv1.ClusterLabelName] = in.ClusterName
	to.SetLabels(labels)

	// Set the owner reference.
	if in.OwnerRef != nil {
		to.SetOwnerReferences([]metav1.OwnerReference{*in.OwnerRef})
	}

	// Set the object APIVersion.
	if to.GetAPIVersion() == "" {
		to.SetAPIVersion(in.Template.GetAPIVersion())
	}

	// Set the object Kind and strip the word "Template" if it's a suffix.
	if to.GetKind() == "" {
		to.SetKind(strings.TrimSuffix(in.Template.GetKind(), TemplateSuffix))
	}
	return to, nil
}

// GetObjectReference converts an unstructured into object reference.
func GetObjectReference(obj *unstructured.Unstructured) *corev1.ObjectReference {
	return &corev1.ObjectReference{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		UID:        obj.GetUID(),
	}
}

// FailuresFrom returns the FailureReason and FailureMessage fields from the external object status.
func FailuresFrom(obj *unstructured.Unstructured) (string, string, error) {
	failureReason, _, err := unstructured.NestedString(obj.Object, "status", "failureReason")
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to determine failureReason on %v %q",
			obj.GroupVersionKind(), obj.GetName())
	}
	failureMessage, _, err := unstructured.NestedString(obj.Object, "status", "failureMessage")
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to determine failureMessage on %v %q",
			obj.GroupVersionKind(), obj.GetName())
	}
	return failureReason, failureMessage, nil
}

// IsReady returns true if the Status.Ready field on an external object is true.
func IsReady(obj *unstructured.Unstructured) (bool, error) {
	ready, found, err := unstructured.NestedBool(obj.Object, "status", "ready")
	if err != nil {
		return false, errors.Wrapf(err, "failed to determine %v %q readiness",
			obj.GroupVersionKind(), obj.GetName())
	}
	return ready && found, nil
}

// IsInitialized returns true if the Status.Initialized field on an external object is true.
func IsInitialized(obj *unstructured.Unstructured) (bool, error) {
	initialized, found, err := unstructured.NestedBool(obj.Object, "status", "initialized")
	if err != nil {
		return false, errors.Wrapf(err, "failed to determine %v %q initialized",
			obj.GroupVersionKind(), obj.GetName())
	}
	return initialized && found, nil
}
