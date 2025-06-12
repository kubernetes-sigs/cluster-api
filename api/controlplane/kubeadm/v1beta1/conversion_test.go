//go:build !race

/*
Copyright 2025 The Kubernetes Authors.

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

	bootstrapv1beta1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
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
	t.Run("for KubeadmControlPlaneTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &controlplanev1.KubeadmControlPlaneTemplate{},
		Spoke:       &KubeadmControlPlaneTemplate{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{KubeadmControlPlaneTemplateFuzzFuncs},
	}))
}

func KubeadmControlPlaneFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		hubMachineTemplateSpec,
		hubKubeadmControlPlaneStatus,
		spokeKubeadmControlPlane,
		spokeKubeadmControlPlaneStatus,
		spokeKubeadmConfigSpec,
		spokeClusterConfiguration,
		hubBootstrapTokenString,
		spokeBootstrapTokenString,
		spokeAPIServer,
		spokeDiscovery,
		hubKubeadmConfigSpec,
		hubNodeRegistrationOptions,
		spokeRemediationStrategy,
		spokeKubeadmControlPlaneMachineTemplate,
		spokeBootstrapToken,
	}
}

func KubeadmControlPlaneTemplateFuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		spokeKubeadmConfigSpec,
		spokeClusterConfiguration,
		hubBootstrapTokenString,
		spokeBootstrapTokenString,
		spokeAPIServer,
		spokeDiscovery,
		hubKubeadmConfigSpec,
		hubNodeRegistrationOptions,
		spokeKubeadmControlPlaneTemplate,
		spokeRemediationStrategy,
		spokeKubeadmControlPlaneTemplateMachineTemplate,
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

func hubBootstrapTokenString(in *bootstrapv1.BootstrapTokenString, _ randfill.Continue) {
	in.ID = fakeID
	in.Secret = fakeSecret
}

func spokeBootstrapTokenString(in *bootstrapv1beta1.BootstrapTokenString, _ randfill.Continue) {
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
	in.Spec.MachineTemplate.InfrastructureRef.APIVersion = gvk.GroupVersion().String()
	in.Spec.MachineTemplate.InfrastructureRef.Kind = gvk.Kind
	in.Spec.MachineTemplate.InfrastructureRef.Namespace = in.Namespace
	in.Spec.MachineTemplate.InfrastructureRef.UID = ""
	in.Spec.MachineTemplate.InfrastructureRef.ResourceVersion = ""
	in.Spec.MachineTemplate.InfrastructureRef.FieldPath = ""

	if reflect.DeepEqual(in.Spec.RolloutBefore, &RolloutBefore{}) {
		in.Spec.RolloutBefore = nil
	}
	if in.Spec.RolloutStrategy != nil {
		if reflect.DeepEqual(in.Spec.RolloutStrategy.RollingUpdate, &RollingUpdate{}) {
			in.Spec.RolloutStrategy.RollingUpdate = nil
		}
	}
	if reflect.DeepEqual(in.Spec.RolloutStrategy, &RolloutStrategy{}) {
		in.Spec.RolloutStrategy = nil
	}
	if reflect.DeepEqual(in.Spec.RolloutAfter, &metav1.Time{}) {
		in.Spec.RolloutAfter = nil
	}
	if reflect.DeepEqual(in.Spec.MachineNamingStrategy, &MachineNamingStrategy{}) {
		in.Spec.MachineNamingStrategy = nil
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

func spokeKubeadmControlPlaneStatus(in *KubeadmControlPlaneStatus, c randfill.Continue) {
	c.FillNoCustom(in)
	// Drop empty structs with only omit empty fields.
	if in.V1Beta2 != nil {
		if reflect.DeepEqual(in.V1Beta2, &KubeadmControlPlaneV1Beta2Status{}) {
			in.V1Beta2 = nil
		}
	}

	// Make sure ready is consistent with ready replicas, so we can rebuild the info after the round trip.
	in.Ready = in.ReadyReplicas > 0

	dropEmptyStringsKubeadmControlPlaneStatus(in)
}

func spokeAPIServer(in *bootstrapv1beta1.APIServer, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TimeoutForControlPlane != nil {
		in.TimeoutForControlPlane = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeDiscovery(in *bootstrapv1beta1.Discovery, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.Timeout != nil {
		in.Timeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeKubeadmConfigSpec(in *bootstrapv1beta1.KubeadmConfigSpec, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop UseExperimentalRetryJoin as we intentionally don't preserve it.
	in.UseExperimentalRetryJoin = false

	dropEmptyStringsKubeadmConfigSpec(in)
}

func spokeClusterConfiguration(in *bootstrapv1beta1.ClusterConfiguration, c randfill.Continue) {
	c.FillNoCustom(in)

	// Drop the following fields as they have been removed in v1beta2, so we don't have to preserve them.
	in.Networking.ServiceSubnet = ""
	in.Networking.PodSubnet = ""
	in.Networking.DNSDomain = ""
	in.KubernetesVersion = ""
	in.ClusterName = ""
}

func spokeKubeadmControlPlaneTemplate(in *KubeadmControlPlaneTemplate, c randfill.Continue) {
	c.FillNoCustom(in)

	if reflect.DeepEqual(in.Spec.Template.Spec.RolloutBefore, &RolloutBefore{}) {
		in.Spec.Template.Spec.RolloutBefore = nil
	}
	if in.Spec.Template.Spec.RolloutStrategy != nil {
		if reflect.DeepEqual(in.Spec.Template.Spec.RolloutStrategy.RollingUpdate, &RollingUpdate{}) {
			in.Spec.Template.Spec.RolloutStrategy.RollingUpdate = nil
		}
	}
	if reflect.DeepEqual(in.Spec.Template.Spec.RolloutStrategy, &RolloutStrategy{}) {
		in.Spec.Template.Spec.RolloutStrategy = nil
	}
	if reflect.DeepEqual(in.Spec.Template.Spec.RolloutAfter, &metav1.Time{}) {
		in.Spec.Template.Spec.RolloutAfter = nil
	}
	if reflect.DeepEqual(in.Spec.Template.Spec.MachineNamingStrategy, &MachineNamingStrategy{}) {
		in.Spec.Template.Spec.MachineNamingStrategy = nil
	}
}

func spokeRemediationStrategy(in *RemediationStrategy, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.MinHealthyPeriod != nil {
		in.MinHealthyPeriod = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	in.RetryPeriod = metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second}
}

func spokeKubeadmControlPlaneMachineTemplate(in *KubeadmControlPlaneMachineTemplate, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeKubeadmControlPlaneTemplateMachineTemplate(in *KubeadmControlPlaneTemplateMachineTemplate, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.NodeDrainTimeout != nil {
		in.NodeDrainTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeVolumeDetachTimeout != nil {
		in.NodeVolumeDetachTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if in.NodeDeletionTimeout != nil {
		in.NodeDeletionTimeout = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
}

func spokeBootstrapToken(in *bootstrapv1beta1.BootstrapToken, c randfill.Continue) {
	c.FillNoCustom(in)

	if in.TTL != nil {
		in.TTL = ptr.To[metav1.Duration](metav1.Duration{Duration: time.Duration(c.Int31()) * time.Second})
	}
	if reflect.DeepEqual(in.Expires, &metav1.Time{}) {
		in.Expires = nil
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
