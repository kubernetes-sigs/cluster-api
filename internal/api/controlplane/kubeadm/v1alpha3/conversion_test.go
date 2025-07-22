//go:build !race

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

package v1alpha3

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	bootstrapv1alpha3 "sigs.k8s.io/cluster-api/internal/api/bootstrap/kubeadm/v1alpha3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
)

const (
	fakeID     = "abcdef"
	fakeSecret = "abcdef0123456789"
)

// Test is disabled when the race detector is enabled (via "//go:build !race" above) because otherwise the fuzz tests would just time out.

func TestFuzzyConversion(t *testing.T) {
	SetAPIVersionGetter(func(gk schema.GroupKind) (string, error) {
		for _, gvk := range testGVKs {
			if gvk.GroupKind() == gk {
				return schema.GroupVersion{
					Group:   gk.Group,
					Version: gvk.Version,
				}.String(), nil
			}
		}
		return "", fmt.Errorf("failed to map GroupKind %s to version", gk.String())
	})

	t.Run("for KubeadmControlPlane", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &controlplanev1.KubeadmControlPlane{},
		Spoke:       &KubeadmControlPlane{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{KubeadmControlPlaneFuzzFuncs},
	}))
}

func KubeadmControlPlaneFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineTemplateSpec,
		hubKubeadmControlPlaneStatus,
		spokeKubeadmControlPlane,
		spokeKubeadmControlPlaneSpec,
		spokeKubeadmControlPlaneStatus,
		spokeDNS,
		spokeKubeadmClusterConfiguration,
		hubBootstrapTokenString,
		spokeKubeadmConfigSpec,
		spokeAPIServer,
		spokeDiscovery,
		hubKubeadmConfigSpec,
		hubNodeRegistrationOptions,
		spokeBootstrapToken,
	}
}

func hubKubeadmConfigSpec(in *bootstrapv1.KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// enforce ControlPlaneComponentHealthCheckSeconds to be equal on init and join configuration
	var initControlPlaneComponentHealthCheckSeconds *int32
	if in.InitConfiguration != nil && in.InitConfiguration.Timeouts != nil {
		initControlPlaneComponentHealthCheckSeconds = in.InitConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds
	}
	if (in.JoinConfiguration != nil && in.JoinConfiguration.Timeouts != nil) || initControlPlaneComponentHealthCheckSeconds != nil {
		if in.JoinConfiguration == nil {
			in.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
		}
		if in.JoinConfiguration.Timeouts == nil {
			in.JoinConfiguration.Timeouts = &bootstrapv1.Timeouts{}
		}
		in.JoinConfiguration.Timeouts.ControlPlaneComponentHealthCheckSeconds = initControlPlaneComponentHealthCheckSeconds
	}
}

func hubNodeRegistrationOptions(in *bootstrapv1.NodeRegistrationOptions, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Taints != nil && *in.Taints == nil {
		in.Taints = nil
	}
}

func hubKubeadmControlPlaneStatus(in *controlplanev1.KubeadmControlPlaneStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Always create struct with at least one mandatory fields.
	if in.Deprecated == nil {
		in.Deprecated = &controlplanev1.KubeadmControlPlaneDeprecatedStatus{}
	}
	if in.Deprecated.V1Beta1 == nil {
		in.Deprecated.V1Beta1 = &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	}

	// nil becomes &0 after hub => spoke => hub conversion
	// This is acceptable as usually Replicas is set and controllers using older apiVersions are not writing MachineSet status.
	if in.Replicas == nil {
		in.Replicas = ptr.To(int32(0))
	}
}

func spokeKubeadmControlPlaneSpec(in *KubeadmControlPlaneSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeKubeadmControlPlaneStatus(in *KubeadmControlPlaneStatus, c randfill.Continue) {
	c.FillNoCustom(in)

	// Make sure ready is consistent with ready replicas, so we can rebuild the info after the round trip.
	in.Ready = in.ReadyReplicas > 0
}

func hubBootstrapTokenString(in *bootstrapv1.BootstrapTokenString, _ randfill.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}

func hubMachineTemplateSpec(in *controlplanev1.KubeadmControlPlaneMachineTemplate, c randfill.Continue) {
	c.FillNoCustom(in)

	// Ensure ref field is always set to realistic values.
	gvk := testGVKs[c.Int31n(4)]
	in.Spec.InfrastructureRef.APIGroup = gvk.Group
	in.Spec.InfrastructureRef.Kind = gvk.Kind
}

func spokeKubeadmControlPlane(in *KubeadmControlPlane, c randfill.Continue) {
	c.FillNoCustom(in)

	// Ensure ref fields are always set to realistic values.
	gvk := testGVKs[c.Int31n(4)]
	in.Spec.InfrastructureTemplate.APIVersion = gvk.GroupVersion().String()
	in.Spec.InfrastructureTemplate.Kind = gvk.Kind
	in.Spec.InfrastructureTemplate.Namespace = in.Namespace
	in.Spec.InfrastructureTemplate.UID = ""
	in.Spec.InfrastructureTemplate.ResourceVersion = ""
	in.Spec.InfrastructureTemplate.FieldPath = ""

	if reflect.DeepEqual(in.Spec.RolloutStrategy, &RolloutStrategy{}) {
		in.Spec.RolloutStrategy = nil
	}
}

func spokeDNS(obj *bootstrapv1alpha3.DNS, c randfill.Continue) {
	c.FillNoCustom(obj)

	// DNS.Type does not exists in v1alpha4, so setting it to empty string in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.Type = ""
}

func spokeKubeadmClusterConfiguration(obj *bootstrapv1alpha3.ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(obj)

	// ClusterConfiguration.UseHyperKubeImage has been removed in v1alpha4, so setting it to false in order to avoid v1alpha3 --> v1alpha4 --> v1alpha3 round trip errors.
	obj.UseHyperKubeImage = false

	// Drop the following fields as they have been removed in v1beta2, so we don't have to preserve them.
	obj.Networking.ServiceSubnet = ""
	obj.Networking.PodSubnet = ""
	obj.Networking.DNSDomain = ""
	obj.KubernetesVersion = ""
	obj.ClusterName = ""
}

func spokeKubeadmConfigSpec(in *bootstrapv1alpha3.KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop UseExperimentalRetryJoin as we intentionally don't preserve it.
	in.UseExperimentalRetryJoin = false

	dropEmptyStringsKubeadmConfigSpec(in)
}

func spokeAPIServer(in *bootstrapv1alpha3.APIServer, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TimeoutForControlPlane != nil {
		in.TimeoutForControlPlane = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeBootstrapToken(in *bootstrapv1alpha3.BootstrapToken, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TTL != nil {
		in.TTL = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeDiscovery(in *bootstrapv1alpha3.Discovery, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Timeout != nil {
		in.Timeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

var testGVKs = []schema.GroupVersionKind{
	{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1beta4",
		Kind:    "KubeadmControlPlane",
	},
	{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1beta7",
		Kind:    "AWSManagedControlPlane",
	},
	{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta3",
		Kind:    "DockerCluster",
	},
	{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta6",
		Kind:    "AWSCluster",
	},
}
