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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
)

// Get uses the client and reference to get an external, unstructured object.
func Get(ctx context.Context, c client.Reader, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, errors.Errorf("cannot get object - object reference not set")
	}
	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetKind(ref.Kind)
	obj.SetName(ref.Name)
	obj.SetNamespace(ref.Namespace)
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %s", obj.GetKind(), klog.KRef(ref.Namespace, ref.Name))
	}
	return obj, nil
}

// GetObjectFromContractVersionedRef uses the client and reference to get an external, unstructured object.
func GetObjectFromContractVersionedRef(ctx context.Context, c client.Reader, ref clusterv1.ContractVersionedObjectReference, namespace string) (*unstructured.Unstructured, error) {
	if !ref.IsDefined() {
		return nil, errors.Errorf("cannot get object - object reference not set")
	}

	metadata, err := contract.GetGKMetadata(ctx, c, schema.GroupKind{Group: ref.APIGroup, Kind: ref.Kind})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// We want to surface the NotFound error only for the referenced object, so we use a generic error in case CRD is not found.
			return nil, errors.Errorf("failed to get object from ref: %v", err.Error())
		}
		return nil, errors.Wrapf(err, "failed to get object from ref")
	}

	_, latestAPIVersion, err := contract.GetLatestContractAndAPIVersionFromContract(metadata, contract.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get object from ref")
	}

	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(schema.GroupVersion{Group: ref.APIGroup, Version: latestAPIVersion}.String())
	obj.SetKind(ref.Kind)
	obj.SetName(ref.Name)
	obj.SetNamespace(namespace)
	if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %s", obj.GetKind(), klog.KRef(namespace, ref.Name))
	}
	return obj, nil
}

// Delete uses the client and reference to delete an external, unstructured object.
func Delete(ctx context.Context, c client.Writer, ref *corev1.ObjectReference) error {
	obj := new(unstructured.Unstructured)
	obj.SetAPIVersion(ref.APIVersion)
	obj.SetKind(ref.Kind)
	obj.SetName(ref.Name)
	obj.SetNamespace(ref.Namespace)
	if err := c.Delete(ctx, obj); err != nil {
		return errors.Wrapf(err, "failed to delete %s %s", obj.GetKind(), klog.KRef(ref.Namespace, ref.Name))
	}
	return nil
}

// CreateFromTemplateInput is the input to CreateFromTemplate.
type CreateFromTemplateInput struct {
	// Client is the controller runtime client.
	Client client.Client

	// TemplateRef is a reference to the template that needs to be cloned.
	TemplateRef *corev1.ObjectReference

	// Namespace is the Kubernetes namespace the cloned object should be created into.
	Namespace string

	// Name is used as the name of the generated object, if set.
	// If it isn't set the template name will be used as prefix to generate a name instead.
	Name string

	// ClusterName is the cluster this object is linked to.
	ClusterName string

	// OwnerRef is an optional OwnerReference to attach to the cloned object.
	// +optional
	OwnerRef *metav1.OwnerReference

	// Labels is an optional map of labels to be added to the object.
	// +optional
	Labels map[string]string

	// Annotations is an optional map of annotations to be added to the object.
	// +optional
	Annotations map[string]string
}

// CreateFromTemplate uses the client and the reference to create a new object from the template.
func CreateFromTemplate(ctx context.Context, in *CreateFromTemplateInput) (*unstructured.Unstructured, clusterv1.ContractVersionedObjectReference, error) {
	from, err := Get(ctx, in.Client, in.TemplateRef)
	if err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, err
	}
	generateTemplateInput := &GenerateTemplateInput{
		Template:    from,
		TemplateRef: in.TemplateRef,
		Namespace:   in.Namespace,
		Name:        in.Name,
		ClusterName: in.ClusterName,
		OwnerRef:    in.OwnerRef,
		Labels:      in.Labels,
		Annotations: in.Annotations,
	}
	to, err := GenerateTemplate(generateTemplateInput)
	if err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, err
	}

	// Create the external clone.
	if err := in.Client.Create(ctx, to); err != nil {
		return nil, clusterv1.ContractVersionedObjectReference{}, err
	}

	return to, clusterv1.ContractVersionedObjectReference{
		APIGroup: to.GroupVersionKind().Group,
		Kind:     to.GetKind(),
		Name:     to.GetName(),
	}, nil
}

// GenerateTemplateInput is the input needed to generate a new template.
type GenerateTemplateInput struct {
	// Template is the TemplateRef turned into an unstructured.
	Template *unstructured.Unstructured

	// TemplateRef is a reference to the template that needs to be cloned.
	TemplateRef *corev1.ObjectReference

	// Namespace is the Kubernetes namespace the cloned object should be created into.
	Namespace string

	// Name is used as the name of the generated object, if set.
	// If it isn't set the template name will be used as prefix to generate a name instead.
	Name string

	// ClusterName is the cluster this object is linked to.
	ClusterName string

	// OwnerRef is an optional OwnerReference to attach to the cloned object.
	// +optional
	OwnerRef *metav1.OwnerReference

	// Labels is an optional map of labels to be added to the object.
	// +optional
	Labels map[string]string

	// Annotations is an optional map of annotations to be added to the object.
	// +optional
	Annotations map[string]string
}

// GenerateTemplate generates an object with the given template input.
func GenerateTemplate(in *GenerateTemplateInput) (*unstructured.Unstructured, error) {
	template, found, err := unstructured.NestedMap(in.Template.Object, "spec", "template")
	if !found {
		// Tolerate template objects without `spec.template`.
		// Intentionally using an empty template as replacement.
		template = nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve Spec.Template map on %v %q", in.Template.GroupVersionKind(), in.Template.GetName())
	}

	// Create the unstructured object from the template.
	to := &unstructured.Unstructured{Object: template}
	to.SetResourceVersion("")
	to.SetFinalizers(nil)
	to.SetUID("")
	to.SetSelfLink("")
	to.SetName(in.Name)
	if to.GetName() == "" {
		to.SetName(names.SimpleNameGenerator.GenerateName(in.Template.GetName() + "-"))
	}
	to.SetNamespace(in.Namespace)

	// Set annotations.
	annotations := to.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	for key, value := range in.Annotations {
		annotations[key] = value
	}
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
	labels[clusterv1.ClusterNameLabel] = in.ClusterName
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
		to.SetKind(strings.TrimSuffix(in.Template.GetKind(), clusterv1.TemplateSuffix))
	}
	return to, nil
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
