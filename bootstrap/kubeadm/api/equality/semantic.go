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

package equality

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	kubeadmv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/util/secret"
)

// SemanticMerge takes two KubeConfigSpecs and produces a third that is semantically equivalent to the other two
// by way of merging non-behavior-changing fields from incoming into current.
func SemanticMerge(current, incoming bootstrapv1.KubeadmConfigSpec, cluster *clusterv1.Cluster) bootstrapv1.KubeadmConfigSpec {
	merged := bootstrapv1.KubeadmConfigSpec{}
	current.DeepCopyInto(&merged)

	merged = mergeClusterConfiguration(merged, incoming, cluster)

	// Prefer non-nil init configurations: in most cases the init configuration is ignored and so this is purely representative
	if merged.InitConfiguration == nil && incoming.InitConfiguration != nil {
		merged.InitConfiguration = incoming.InitConfiguration.DeepCopy()
	}

	// The type meta for embedded types is currently ignored
	if merged.ClusterConfiguration != nil && incoming.ClusterConfiguration != nil {
		merged.ClusterConfiguration.TypeMeta = incoming.ClusterConfiguration.TypeMeta
	}

	if merged.InitConfiguration != nil && incoming.InitConfiguration != nil {
		merged.InitConfiguration.TypeMeta = incoming.InitConfiguration.TypeMeta
	}

	if merged.JoinConfiguration != nil && incoming.JoinConfiguration != nil {
		merged.JoinConfiguration.TypeMeta = incoming.JoinConfiguration.TypeMeta
	}

	// The default discovery injected by the kubeadm bootstrap controller is a unique, time-bounded token. However, any
	// valid token has the same effect of joining the machine to the cluster. We consider the following scenarios:
	// 1. current has no join configuration (meaning it was never reconciled) -> do nothing
	// 2. current has a bootstrap token, and incoming has some configured discovery mechanism -> prefer incoming
	// 3. current has a bootstrap token, and incoming has no discovery set -> delete current's discovery config
	// 4. in all other cases, prefer current's configuration
	if merged.JoinConfiguration == nil || merged.JoinConfiguration.Discovery.BootstrapToken == nil {
		return merged
	}

	// current has a bootstrap token, check incoming
	switch {
	case incoming.JoinConfiguration == nil:
		merged.JoinConfiguration.Discovery = kubeadmv1.Discovery{}
	case incoming.JoinConfiguration.Discovery.BootstrapToken != nil:
		fallthrough
	case incoming.JoinConfiguration.Discovery.File != nil:
		incoming.JoinConfiguration.Discovery.DeepCopyInto(&merged.JoinConfiguration.Discovery)
	default:
		// Neither token nor file is set on incoming's Discovery config
		merged.JoinConfiguration.Discovery = kubeadmv1.Discovery{}
	}

	return merged
}

func mergeClusterConfiguration(merged, incoming bootstrapv1.KubeadmConfigSpec, cluster *clusterv1.Cluster) bootstrapv1.KubeadmConfigSpec {
	if merged.ClusterConfiguration == nil && incoming.ClusterConfiguration != nil {
		merged.ClusterConfiguration = incoming.ClusterConfiguration.DeepCopy()
	} else if merged.ClusterConfiguration == nil {
		// incoming's cluster configuration is also nil
		return merged
	}

	// Attempt to reconstruct a cluster config in reverse of reconcileTopLevelObjectSettings
	newCfg := incoming.ClusterConfiguration
	if newCfg == nil {
		newCfg = &kubeadmv1.ClusterConfiguration{}
	}

	if merged.ClusterConfiguration.ControlPlaneEndpoint == cluster.Spec.ControlPlaneEndpoint.String() &&
		newCfg.ControlPlaneEndpoint == "" {
		merged.ClusterConfiguration.ControlPlaneEndpoint = ""
	}

	if newCfg.ClusterName == "" {
		merged.ClusterConfiguration.ClusterName = ""
	}

	if cluster.Spec.ClusterNetwork != nil {
		if merged.ClusterConfiguration.Networking.DNSDomain == cluster.Spec.ClusterNetwork.ServiceDomain && newCfg.Networking.DNSDomain == "" {
			merged.ClusterConfiguration.Networking.DNSDomain = ""
		}

		if merged.ClusterConfiguration.Networking.ServiceSubnet == cluster.Spec.ClusterNetwork.Services.String() &&
			newCfg.Networking.ServiceSubnet == "" {
			merged.ClusterConfiguration.Networking.ServiceSubnet = ""
		}

		if merged.ClusterConfiguration.Networking.PodSubnet == cluster.Spec.ClusterNetwork.Pods.String() &&
			newCfg.Networking.PodSubnet == "" {
			merged.ClusterConfiguration.Networking.PodSubnet = ""
		}
	}

	// We defer to other sources for the version, such as the Machine
	if newCfg.KubernetesVersion == "" {
		merged.ClusterConfiguration.KubernetesVersion = ""
	}

	if merged.ClusterConfiguration.CertificatesDir == secret.DefaultCertificatesDir &&
		newCfg.CertificatesDir == "" {
		merged.ClusterConfiguration.CertificatesDir = ""
	}

	return merged
}
