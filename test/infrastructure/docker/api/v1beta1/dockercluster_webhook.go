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

package v1beta1

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (c *DockerCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/mutate-infrastructure-cluster-x-k8s-io-v1beta1-dockercluster", &webhook.Admission{
		Handler: &DockerClusterMutator{},
	})
	return ctrl.NewWebhookManagedBy(mgr).
		For(c).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-dockercluster,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,versions=v1beta1,name=default.dockercluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Defaulter = &DockerCluster{}

// DockerClusterMutator validates KCP for replicas.
// +kubebuilder:object:generate=false
type DockerClusterMutator struct {
	decoder *admission.Decoder
}

// Handle will validate for number of replicas.
func (v *DockerClusterMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	dct := &DockerCluster{}

	err := v.decoder.DecodeRaw(req.Object, dct)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, errors.Wrapf(err, "failed to decode DockerCluster resource"))
	}

	if req.Operation == admissionv1.Create {
		defaultDockerClusterSpec(&dct.Spec)
	}
	if req.Operation == admissionv1.Update {
		oldDct := &DockerCluster{}

		if err := v.decoder.DecodeRaw(req.OldObject, oldDct); err != nil {
			return admission.Errored(http.StatusBadRequest, errors.Wrapf(err, "failed to decode DockerCluster resource"))
		}

		updateOpts := &metav1.UpdateOptions{}
		if err := v.decoder.DecodeRaw(req.Options, updateOpts); err != nil {
			return admission.Errored(http.StatusBadRequest, errors.Wrapf(err, "failed to decode UpdateOptions resource"))
		}

		if updateOpts.FieldManager == "capi-topology" {
			dct.Spec.Subnets4 = oldDct.Spec.Subnets4
		}
	}

	// Create the patch
	marshalled, err := json.Marshal(dct)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshalled)
}

// InjectDecoder injects the decoder.
// DockerClusterMutator implements admission.DecoderInjector.
// A decoder will be automatically injected.
func (v *DockerClusterMutator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (c *DockerCluster) Default() {
	defaultDockerClusterSpec(&c.Spec)
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-dockercluster,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,versions=v1beta1,name=validation.dockercluster.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &DockerCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (c *DockerCluster) ValidateCreate() error {
	if allErrs := validateDockerClusterSpec(c.Spec); len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("DockerCluster").GroupKind(), c.Name, allErrs)
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (c *DockerCluster) ValidateUpdate(old runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (c *DockerCluster) ValidateDelete() error {
	return nil
}

func defaultDockerClusterSpec(s *DockerClusterSpec) {
	for i, _ := range s.Subnets1 {
		if s.Subnets1[i].UUID == nil || *s.Subnets1[i].UUID == "dummy" {
			s.Subnets1[i].UUID = pointer.String(string(uuid.NewUUID()))
		}
	}
	for i, _ := range s.Subnets2 {
		if s.Subnets2[i].UUID == nil || *s.Subnets2[i].UUID == "dummy" {
			s.Subnets2[i].UUID = pointer.String(string(uuid.NewUUID()))
		}
	}
	for i, _ := range s.Subnets3 {
		if s.Subnets3[i].UUID == nil || *s.Subnets3[i].UUID == "dummy" {
			s.Subnets3[i].UUID = pointer.String(string(uuid.NewUUID()))
		}
	}
}

func validateDockerClusterSpec(s DockerClusterSpec) field.ErrorList {
	return nil
}
