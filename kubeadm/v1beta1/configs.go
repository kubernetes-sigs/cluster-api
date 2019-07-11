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

package v1beta1

import (
	kubeadmv1beta1 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
)

// InitConfigurationOption will set various options on kubeadm's InitConfiguration type.
type InitConfigurationOption func(*kubeadmv1beta1.InitConfiguration)

// SetInitConfigurationOptions will run all the options provided against some existing InitConfiguration.
// Use this to customize an existing InitConfiguration.
func SetInitConfigurationOptions(base *kubeadmv1beta1.InitConfiguration, opts ...InitConfigurationOption) {
	for _, opt := range opts {
		opt(base)
	}
}

// WithNodeRegistrationOptions will set the node registration options.
func WithNodeRegistrationOptions(nro kubeadmv1beta1.NodeRegistrationOptions) InitConfigurationOption {
	return func(c *kubeadmv1beta1.InitConfiguration) {
		c.NodeRegistration = nro
	}
}

// ClusterConfigurationOption is a type of function that sets options on ak kubeadm ClusterConfiguration.
type ClusterConfigurationOption func(*kubeadmv1beta1.ClusterConfiguration)

// SetClusterConfigurationOptions will run all options on the passed in ClusterConfiguration.
// Use this to customize an existing ClusterConfiguration.
func SetClusterConfigurationOptions(base *kubeadmv1beta1.ClusterConfiguration, opts ...ClusterConfigurationOption) {
	for _, opt := range opts {
		opt(base)
	}
}

// WithClusterName sets the cluster name on a ClusterConfiguration.
func WithClusterName(name string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		c.ClusterName = name
	}
}

// WithKubernetesVersion sets the kubernetes version on a ClusterConfiguration.
func WithKubernetesVersion(version string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		c.KubernetesVersion = version
	}
}

// WithAPIServerExtraArgs sets the kube-apiserver's extra arguments in the ClusterConfiguration.
func WithAPIServerExtraArgs(args map[string]string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		if c.APIServer.ControlPlaneComponent.ExtraArgs == nil {
			c.APIServer.ControlPlaneComponent.ExtraArgs = map[string]string{}
		}
		for key, value := range args {
			c.APIServer.ControlPlaneComponent.ExtraArgs[key] = value
		}
	}
}

// WithControllerManagerExtraArgs sets the controller manager's extra arguments in the ClusterConfiguration.
func WithControllerManagerExtraArgs(args map[string]string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		if c.ControllerManager.ExtraArgs == nil {
			c.ControllerManager.ExtraArgs = map[string]string{}
		}
		for key, value := range args {
			c.ControllerManager.ExtraArgs[key] = value
		}
	}
}

// WithClusterNetworkFromClusterNetworkingConfig maps a cluster api ClusterNetworkingConfig to a kubeadm Networking object.
func WithClusterNetworkFromClusterNetworkingConfig(serviceDomain, podSubnet, serviceSubnet string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		c.Networking.DNSDomain = serviceDomain
		c.Networking.PodSubnet = podSubnet
		c.Networking.ServiceSubnet = serviceSubnet
	}
}

// WithControlPlaneEndpoint sets the control plane endpoint, usually the load balancer in front of the kube-apiserver(s).
func WithControlPlaneEndpoint(endpoint string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		if c.ControlPlaneEndpoint == "" {
			c.ControlPlaneEndpoint = endpoint
		}
	}
}

// WithAPIServerCertificateSANs will add the names provided to the kube-apiserver's list of certificate SANs.
func WithAPIServerCertificateSANs(names ...string) ClusterConfigurationOption {
	return func(c *kubeadmv1beta1.ClusterConfiguration) {
		c.APIServer.CertSANs = append(c.APIServer.CertSANs, names...)
	}
}

// JoinConfigurationOption is a type of function that sets options on a kubeadm JoinConfiguration.
type JoinConfigurationOption func(*kubeadmv1beta1.JoinConfiguration)

// SetJoinConfigurationOptions runs a set of options against a JoinConfiguration.
// Use this to customize an existing JoinConfiguration.
func SetJoinConfigurationOptions(base *kubeadmv1beta1.JoinConfiguration, opts ...JoinConfigurationOption) {
	for _, opt := range opts {
		opt(base)
	}
}

// WithBootstrapTokenDiscovery sets the BootstrapToken on the Discovery field to the passed in value.
func WithBootstrapTokenDiscovery(bootstrapTokenDiscovery *kubeadmv1beta1.BootstrapTokenDiscovery) JoinConfigurationOption {
	return func(j *kubeadmv1beta1.JoinConfiguration) {
		j.Discovery.BootstrapToken = bootstrapTokenDiscovery
	}
}

// WithLocalAPIEndpointAndPort sets the local advertise address and port the kube-apiserver will advertise it is listening on.
func WithLocalAPIEndpointAndPort(endpoint string, port int) JoinConfigurationOption {
	return func(j *kubeadmv1beta1.JoinConfiguration) {
		if j.ControlPlane == nil {
			j.ControlPlane = &kubeadmv1beta1.JoinControlPlane{}
		}
		j.ControlPlane.LocalAPIEndpoint.AdvertiseAddress = endpoint
		j.ControlPlane.LocalAPIEndpoint.BindPort = int32(port)
	}
}

// WithJoinNodeRegistrationOptions will set the NodeRegistrationOptions on the JoinConfiguration.
func WithJoinNodeRegistrationOptions(nro kubeadmv1beta1.NodeRegistrationOptions) JoinConfigurationOption {
	return func(j *kubeadmv1beta1.JoinConfiguration) {
		j.NodeRegistration = nro
	}
}
