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

package webhooks

import (
	"context"
	"net/http"

	"github.com/pkg/errors"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

func (v *ScaleValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register("/validate-scale-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplane", &webhook.Admission{
		Handler: v,
	})
	return nil
}

// +kubebuilder:webhook:verbs=update,path=/validate-scale-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplane,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes/scale,versions=v1beta1,name=validation-scale.kubeadmcontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

// ScaleValidator validates KCP for replicas.
type ScaleValidator struct {
	Client  client.Reader
	decoder *admission.Decoder
}

// Handle will validate for number of replicas.
func (v *ScaleValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	scale := &autoscalingv1.Scale{}

	err := v.decoder.Decode(req, scale)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, errors.Wrapf(err, "failed to decode Scale resource"))
	}

	kcp := &controlplanev1.KubeadmControlPlane{}
	kcpKey := types.NamespacedName{Namespace: scale.ObjectMeta.Namespace, Name: scale.ObjectMeta.Name}
	if err = v.Client.Get(ctx, kcpKey, kcp); err != nil {
		return admission.Errored(http.StatusInternalServerError, errors.Wrapf(err, "failed to get KubeadmControlPlane %s/%s", scale.ObjectMeta.Namespace, scale.ObjectMeta.Name))
	}

	if scale.Spec.Replicas == 0 {
		return admission.Denied("replicas cannot be 0")
	}

	externalEtcd := false
	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
		if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External != nil {
			externalEtcd = true
		}
	}

	if !externalEtcd {
		if scale.Spec.Replicas%2 == 0 {
			return admission.Denied("replicas cannot be an even number when etcd is stacked")
		}
	}

	return admission.Allowed("")
}

// InjectDecoder injects the decoder.
// ScaleValidator implements admission.DecoderInjector.
// A decoder will be automatically injected.
func (v *ScaleValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
