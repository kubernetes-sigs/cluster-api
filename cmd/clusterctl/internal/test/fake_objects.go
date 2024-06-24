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

package test

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	fakebootstrap "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/bootstrap"
	fakecontrolplane "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/controlplane"
	fakeexternal "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/external"
	fakeinfrastructure "sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/infrastructure"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/util"
)

type FakeCluster struct {
	namespace             string
	name                  string
	controlPlane          *FakeControlPlane
	machinePools          []*FakeMachinePool
	machineDeployments    []*FakeMachineDeployment
	machineSets           []*FakeMachineSet
	machines              []*FakeMachine
	withCloudConfigSecret bool
	withCredentialSecret  bool
	topologyClass         *string
}

// NewFakeCluster return a FakeCluster that can generate a cluster object, all its own ancillary objects:
// - the clusterInfrastructure object
// - the kubeconfig secret object (if there is no a control plane object)
// - a user defined ca secret
// and all the objects for the defined FakeControlPlane, FakeMachinePools, FakeMachineDeployments, FakeMachineSets, FakeMachines
// Nb. if there is no a control plane object, the first FakeMachine gets a generated sa secret.
func NewFakeCluster(namespace, name string) *FakeCluster {
	return &FakeCluster{
		namespace: namespace,
		name:      name,
	}
}

func (f *FakeCluster) WithControlPlane(fakeControlPlane *FakeControlPlane) *FakeCluster {
	f.controlPlane = fakeControlPlane
	return f
}

func (f *FakeCluster) WithMachinePools(fakeMachinePool ...*FakeMachinePool) *FakeCluster {
	f.machinePools = append(f.machinePools, fakeMachinePool...)
	return f
}

func (f *FakeCluster) WithCloudConfigSecret() *FakeCluster {
	f.withCloudConfigSecret = true
	return f
}

func (f *FakeCluster) WithCredentialSecret() *FakeCluster {
	f.withCredentialSecret = true
	return f
}

func (f *FakeCluster) WithMachineDeployments(fakeMachineDeployment ...*FakeMachineDeployment) *FakeCluster {
	f.machineDeployments = append(f.machineDeployments, fakeMachineDeployment...)
	return f
}

func (f *FakeCluster) WithMachineSets(fakeMachineSet ...*FakeMachineSet) *FakeCluster {
	f.machineSets = append(f.machineSets, fakeMachineSet...)
	return f
}

func (f *FakeCluster) WithMachines(fakeMachine ...*FakeMachine) *FakeCluster {
	f.machines = append(f.machines, fakeMachine...)
	return f
}

func (f *FakeCluster) WithTopologyClass(class string) *FakeCluster {
	f.topologyClass = &class
	return f
}

func (f *FakeCluster) Objs() []client.Object {
	clusterInfrastructure := &fakeinfrastructure.GenericInfrastructureCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeinfrastructure.GroupVersion.String(),
			Kind:       "GenericInfrastructureCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
			// OwnerReferences: cluster, Added by the cluster controller (see below) -- RECONCILED
			// Labels: cluster.x-k8s.io/cluster-name=cluster, Added by the cluster controller (see below) -- RECONCILED
		},
	}

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
			// Labels: cluster.x-k8s.io/cluster-name=cluster MISSING??
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: clusterInfrastructure.APIVersion,
				Kind:       clusterInfrastructure.Kind,
				Name:       clusterInfrastructure.Name,
				Namespace:  clusterInfrastructure.Namespace,
			},
		},
	}

	if f.topologyClass != nil {
		cluster.Spec.Topology = &clusterv1.Topology{Class: *f.topologyClass}
	}

	// Ensure the cluster gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(cluster)

	clusterInfrastructure.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		},
	})
	clusterInfrastructure.SetLabels(map[string]string{
		clusterv1.ClusterNameLabel: cluster.Name,
	})

	caSecret := &corev1.Secret{ // provided by the user -- ** NOT RECONCILED **
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name + "-ca",
			Namespace: f.namespace,
		},
	}

	objs := []client.Object{
		cluster,
		clusterInfrastructure,
		caSecret,
	}

	if f.withCloudConfigSecret {
		cloudSecret := &corev1.Secret{ // provided by the user -- ** NOT RECONCILED **
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.name + "-cloud-config",
				Namespace: f.namespace,
			},
		}

		cloudSecret.SetLabels(map[string]string{
			clusterctlv1.ClusterctlMoveLabel: "",
		})
		objs = append(objs, cloudSecret)
	}

	if f.withCredentialSecret {
		credentialSecret := &corev1.Secret{ // provided by the user -- ** NOT RECONCILED **
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.name + "-credentials",
				Namespace: f.namespace,
			},
		}
		credentialSecret.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: cluster.APIVersion,
				Kind:       cluster.Kind,
				Name:       cluster.Name,
				UID:        cluster.UID,
			},
		})
		objs = append(objs, credentialSecret)
	}

	// if the cluster has a control plane object
	if f.controlPlane != nil {
		// Adds the objects for the controlPlane
		objs = append(objs, f.controlPlane.Objs(cluster)...)
	} else {
		// Adds the kubeconfig object generated by the cluster controller -- ** NOT RECONCILED **
		kubeconfigSecret := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.name + "-kubeconfig",
				Namespace: f.namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: cluster.APIVersion,
						Kind:       cluster.Kind,
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
				// Labels: cluster.x-k8s.io/cluster-name=cluster MISSING??
			},
		}
		objs = append(objs, kubeconfigSecret)
	}

	// Adds the objects for the machinePools
	for _, machinePool := range f.machinePools {
		objs = append(objs, machinePool.Objs(cluster)...)
	}

	// Adds the objects for the machineDeployments
	for _, machineDeployment := range f.machineDeployments {
		objs = append(objs, machineDeployment.Objs(cluster)...)
	}

	// Adds the objects for the machineSets directly attached to the cluster
	for _, machineSet := range f.machineSets {
		objs = append(objs, machineSet.Objs(cluster, nil)...)
	}

	// Adds the objects for the machines directly attached to the cluster
	// Nb. In case there is no control plane, the first machine is arbitrarily used to simulate the generation of certificates Secrets implemented by the bootstrap controller.
	for i, machine := range f.machines {
		generateCerts := false
		if f.controlPlane == nil && i == 0 {
			generateCerts = true
		}
		objs = append(objs, machine.Objs(cluster, generateCerts, nil, nil, nil)...)
	}

	// Ensure all the objects gets UID.
	// Nb. This adds UID to all the objects; it does not change the UID explicitly sets in advance for the objects involved in the object graphs.
	for _, o := range objs {
		setUID(o)
	}

	return objs
}

type FakeControlPlane struct {
	name     string
	machines []*FakeMachine
}

// NewFakeControlPlane return a FakeControlPlane that can generate a controlPlane object, all its own ancillary objects:
// - the controlPlaneInfrastructure template object
// - the kubeconfig secret object
// - a generated sa secret
// and all the objects for the defined FakeMachines.
func NewFakeControlPlane(name string) *FakeControlPlane {
	return &FakeControlPlane{
		name: name,
	}
}

func (f *FakeControlPlane) WithMachines(fakeMachine ...*FakeMachine) *FakeControlPlane {
	f.machines = append(f.machines, fakeMachine...)
	return f
}

func (f *FakeControlPlane) Objs(cluster *clusterv1.Cluster) []client.Object {
	controlPlaneInfrastructure := &fakeinfrastructure.GenericInfrastructureMachineTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeinfrastructure.GroupVersion.String(),
			Kind:       "GenericInfrastructureMachineTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the control plane  controller (see below) -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			// Labels: MISSING
		},
	}

	controlPlane := &fakecontrolplane.GenericControlPlane{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakecontrolplane.GroupVersion.String(),
			Kind:       "GenericControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the control plane  controller (see below) -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			Labels: map[string]string{ // cluster.x-k8s.io/cluster-name=cluster, Added by the control plane controller (see below) -- RECONCILED
				clusterv1.ClusterNameLabel: cluster.Name,
			},
		},
		Spec: fakecontrolplane.GenericControlPlaneSpec{
			MachineTemplate: fakecontrolplane.GenericMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: controlPlaneInfrastructure.APIVersion,
					Kind:       controlPlaneInfrastructure.Kind,
					Namespace:  controlPlaneInfrastructure.Namespace,
					Name:       controlPlaneInfrastructure.Name,
				},
			},
		},
	}

	// Ensure the controlPlane gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(controlPlane)

	// sets the reference from the cluster to the plane object
	cluster.Spec.ControlPlaneRef = &corev1.ObjectReference{
		APIVersion: controlPlane.APIVersion,
		Kind:       controlPlane.Kind,
		Namespace:  controlPlane.Namespace,
		Name:       controlPlane.Name,
	}

	// Adds the kubeconfig object generated by the control plane  controller -- ** NOT RECONCILED **
	kubeconfigSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.GetName() + "-kubeconfig",
			Namespace: cluster.GetNamespace(),
			// Labels: cluster.x-k8s.io/cluster-name=cluster MISSING??
		},
	}
	kubeconfigSecret.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(controlPlane, controlPlane.GroupVersionKind())})

	// Adds one of the certificate secret object generated by the control plane controller -- ** NOT RECONCILED **
	saSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.GetName() + "-sa",
			Namespace: cluster.GetNamespace(),
			// Labels: cluster.x-k8s.io/cluster-name=cluster MISSING??
		},
	}
	saSecret.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(controlPlane, controlPlane.GroupVersionKind())})

	objs := []client.Object{
		controlPlane,
		controlPlaneInfrastructure,
		kubeconfigSecret,
		saSecret,
	}

	// Adds the objects for the machines controlled by the controlPlane
	for _, machine := range f.machines {
		objs = append(objs, machine.Objs(cluster, false, nil, nil, controlPlane)...)
	}

	return objs
}

type FakeMachinePool struct {
	name            string
	bootstrapConfig *clusterv1.Bootstrap
	machines        []*FakeMachine
}

// NewFakeMachinePool return a FakeMachinePool that can generate a MachinePool object, all its own ancillary objects:
// - the machinePoolInfrastructure object
// - the machinePoolBootstrap object.
func NewFakeMachinePool(name string) *FakeMachinePool {
	return &FakeMachinePool{
		name: name,
	}
}

func (f *FakeMachinePool) WithStaticBootstrapConfig() *FakeMachinePool {
	f.bootstrapConfig = NewStaticBootstrapConfig(f.name)
	return f
}

func (f *FakeMachinePool) WithMachines(fakeMachine ...*FakeMachine) *FakeMachinePool {
	f.machines = append(f.machines, fakeMachine...)
	return f
}

func (f *FakeMachinePool) Objs(cluster *clusterv1.Cluster) []client.Object {
	machinePoolInfrastructure := &fakeinfrastructure.GenericInfrastructureMachineTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeinfrastructure.GroupVersion.String(),
			Kind:       "GenericInfrastructureMachineTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the machinePool controller (mirrors machinePool.spec.ClusterName) -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			// Labels: MISSING
		},
	}

	machinePoolBootstrap := &fakebootstrap.GenericBootstrapConfigTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakebootstrap.GroupVersion.String(),
			Kind:       "GenericBootstrapConfigTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the machinePool controller (mirrors machinePool.spec.ClusterName) -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			// Labels: MISSING
		},
	}

	bootstrapConfig := f.bootstrapConfig
	if bootstrapConfig == nil {
		bootstrapConfig = NewBootstrapConfigTemplate(machinePoolBootstrap)
	}

	machinePool := &clusterv1.MachinePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachinePool",
			APIVersion: expv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the machinePool controller (mirrors machinePool.spec.ClusterName) -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name, // Added by the machinePool controller (mirrors machinePoolt.spec.ClusterName) -- RECONCILED
			},
		},
		Spec: clusterv1.MachinePoolSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: machinePoolInfrastructure.APIVersion,
						Kind:       machinePoolInfrastructure.Kind,
						Name:       machinePoolInfrastructure.Name,
						Namespace:  machinePoolInfrastructure.Namespace,
					},
					Bootstrap: *bootstrapConfig,
				},
			},
			ClusterName: cluster.Name,
		},
	}

	// Ensure the machinePool gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(machinePool)

	objs := []client.Object{
		machinePool,
		machinePoolInfrastructure,
	}

	// if the bootstrapConfig doesn't use a static secret, add the GenericBootstrapConfigTemplate to the object list
	if bootstrapConfig.ConfigRef != nil {
		objs = append(objs, machinePoolBootstrap)
	}

	for _, machine := range f.machines {
		objs = append(objs, machine.Objs(cluster, false, nil, machinePool, nil)...)
	}

	return objs
}

func NewFakeInfrastructureTemplate(name string) *fakeinfrastructure.GenericInfrastructureMachineTemplate {
	return &fakeinfrastructure.GenericInfrastructureMachineTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeinfrastructure.GroupVersion.String(),
			Kind:       "GenericInfrastructureMachineTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			// OwnerReference Added by the machine set controller -- RECONCILED
			// Labels: MISSING
		},
	}
}

// NewStaticBootstrapConfig return a clusterv1.Bootstrap where
// - the ConfigRef is nil
// - the DataSecretName contains the name of the static data secret.
func NewStaticBootstrapConfig(name string) *clusterv1.Bootstrap {
	return &clusterv1.Bootstrap{
		DataSecretName: ptr.To(name + "-bootstrap-secret"),
	}
}

func NewBootstrapConfigTemplate(machineBootstrapTemplate *fakebootstrap.GenericBootstrapConfigTemplate) *clusterv1.Bootstrap {
	return &clusterv1.Bootstrap{
		ConfigRef: &corev1.ObjectReference{
			APIVersion: machineBootstrapTemplate.APIVersion,
			Kind:       machineBootstrapTemplate.Kind,
			Name:       machineBootstrapTemplate.Name,
			Namespace:  machineBootstrapTemplate.Namespace,
		},
	}
}

func NewBootstrapConfig(machineBootstrap *fakebootstrap.GenericBootstrapConfig) *clusterv1.Bootstrap {
	return &clusterv1.Bootstrap{
		ConfigRef: &corev1.ObjectReference{
			APIVersion: machineBootstrap.APIVersion,
			Kind:       machineBootstrap.Kind,
			Name:       machineBootstrap.Name,
			Namespace:  machineBootstrap.Namespace,
		},
	}
}

type FakeMachineDeployment struct {
	name                         string
	machineSets                  []*FakeMachineSet
	sharedInfrastructureTemplate *fakeinfrastructure.GenericInfrastructureMachineTemplate
	bootstrapConfig              *clusterv1.Bootstrap
}

// NewFakeMachineDeployment return a FakeMachineDeployment that can generate a MachineDeployment object, all its own ancillary objects:
// - the machineDeploymentInfrastructure template object
// - the machineDeploymentBootstrap template object
// and all the objects for the defined FakeMachineSet.
func NewFakeMachineDeployment(name string) *FakeMachineDeployment {
	return &FakeMachineDeployment{
		name: name,
	}
}

func (f *FakeMachineDeployment) WithMachineSets(fakeMachineSet ...*FakeMachineSet) *FakeMachineDeployment {
	f.machineSets = append(f.machineSets, fakeMachineSet...)
	return f
}

func (f *FakeMachineDeployment) WithStaticBootstrapConfig() *FakeMachineDeployment {
	f.bootstrapConfig = NewStaticBootstrapConfig(f.name)
	return f
}

func (f *FakeMachineDeployment) WithInfrastructureTemplate(infrastructureTemplate *fakeinfrastructure.GenericInfrastructureMachineTemplate) *FakeMachineDeployment {
	f.sharedInfrastructureTemplate = infrastructureTemplate
	return f
}

func (f *FakeMachineDeployment) Objs(cluster *clusterv1.Cluster) []client.Object {
	// infra template can be either shared or specific to the machine deployment
	machineDeploymentInfrastructure := f.sharedInfrastructureTemplate
	if machineDeploymentInfrastructure == nil {
		machineDeploymentInfrastructure = NewFakeInfrastructureTemplate(f.name)
	}
	machineDeploymentInfrastructure.Namespace = cluster.Namespace
	machineDeploymentInfrastructure.SetOwnerReferences(util.EnsureOwnerRef(machineDeploymentInfrastructure.GetOwnerReferences(), // Added by the machine set controller -- RECONCILED
		metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		},
	))
	setUID(machineDeploymentInfrastructure)

	machineDeploymentBootstrap := &fakebootstrap.GenericBootstrapConfigTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakebootstrap.GroupVersion.String(),
			Kind:       "GenericBootstrapConfigTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the machine set controller -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			// Labels: MISSING
		},
	}

	bootstrapConfig := f.bootstrapConfig
	if bootstrapConfig == nil {
		bootstrapConfig = NewBootstrapConfigTemplate(machineDeploymentBootstrap)
	}

	machineDeployment := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ // Added by the machineDeployment controller (mirrors machineDeployment.spec.ClusterName) -- RECONCILED
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
				},
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name, // Added by the machineDeployment controller (mirrors machineDeployment.spec.ClusterName) -- RECONCILED
			},
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					InfrastructureRef: corev1.ObjectReference{
						APIVersion: machineDeploymentInfrastructure.APIVersion,
						Kind:       machineDeploymentInfrastructure.Kind,
						Name:       machineDeploymentInfrastructure.Name,
						Namespace:  machineDeploymentInfrastructure.Namespace,
					},
					Bootstrap: *bootstrapConfig,
				},
			},
			ClusterName: cluster.Name,
		},
	}

	// Ensure the machineDeployment gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(machineDeployment)

	objs := []client.Object{
		machineDeployment,
	}

	// if the bootstrapConfig doesn't use a static secret, add the GenericBootstrapConfigTemplate to the object list
	if bootstrapConfig.ConfigRef != nil {
		objs = append(objs, machineDeploymentBootstrap)
	}

	// if the infra template is specific to the machine deployment, add it to the object list
	if f.sharedInfrastructureTemplate == nil {
		objs = append(objs, machineDeploymentInfrastructure)
	}

	// Adds the objects for the machineSets
	for _, machineSet := range f.machineSets {
		objs = append(objs, machineSet.Objs(cluster, machineDeployment)...)
	}

	return objs
}

type FakeMachineSet struct {
	name                         string
	machines                     []*FakeMachine
	sharedInfrastructureTemplate *fakeinfrastructure.GenericInfrastructureMachineTemplate
	bootstrapConfig              *clusterv1.Bootstrap
}

// NewFakeMachineSet return a FakeMachineSet that can generate a MachineSet object, all its own ancillary objects:
// - the machineSetInfrastructure template object (only if not controlled by a MachineDeployment)
// - the machineSetBootstrap template object (only if not controlled by a MachineDeployment)
// and all the objects for the defined FakeMachine.
func NewFakeMachineSet(name string) *FakeMachineSet {
	return &FakeMachineSet{
		name: name,
	}
}

func (f *FakeMachineSet) WithMachines(fakeMachine ...*FakeMachine) *FakeMachineSet {
	f.machines = append(f.machines, fakeMachine...)
	return f
}

func (f *FakeMachineSet) WithStaticBootstrapConfig() *FakeMachineSet {
	f.bootstrapConfig = NewStaticBootstrapConfig(f.name)
	return f
}

func (f *FakeMachineSet) WithInfrastructureTemplate(infrastructureTemplate *fakeinfrastructure.GenericInfrastructureMachineTemplate) *FakeMachineSet {
	f.sharedInfrastructureTemplate = infrastructureTemplate
	return f
}

func (f *FakeMachineSet) Objs(cluster *clusterv1.Cluster, machineDeployment *clusterv1.MachineDeployment) []client.Object {
	machineSet := &clusterv1.MachineSet{ // Created by machineDeployment controller
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineSet",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			// Owner reference set by machineSet controller or by machineDeployment controller (see below)
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name, // Added by the machineSet controller (mirrors machineSet.spec.ClusterName) -- RECONCILED
			},
		},
		Spec: clusterv1.MachineSetSpec{
			ClusterName: cluster.Name,
		},
	}

	// Ensure the machineSet gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(machineSet)

	objs := make([]client.Object, 0)

	if machineDeployment != nil {
		// If this machineSet belong to a machineDeployment, it is controlled by it / ownership set by the machineDeployment controller  -- ** NOT RECONCILED **
		machineSet.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(machineDeployment, machineDeployment.GroupVersionKind())})

		// additionally the machine has ref to the same infra and bootstrap templates defined in the MachineDeployment
		machineSet.Spec.Template.Spec.InfrastructureRef = *machineDeployment.Spec.Template.Spec.InfrastructureRef.DeepCopy()
		machineSet.Spec.Template.Spec.Bootstrap.ConfigRef = machineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef.DeepCopy()

		objs = append(objs, machineSet)
	} else {
		// If this machineSet does not belong to a machineDeployment, it is owned by the cluster / ownership set by the machineSet controller -- RECONCILED
		machineSet.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		}})

		// additionally the machineSet has ref to dedicated infra and bootstrap templates

		// infra template can be either shared or specific to the machine set
		machineSetInfrastructure := f.sharedInfrastructureTemplate
		if machineSetInfrastructure == nil {
			machineSetInfrastructure = NewFakeInfrastructureTemplate(f.name)
		}
		machineSetInfrastructure.Namespace = cluster.Namespace
		machineSetInfrastructure.OwnerReferences = append(machineSetInfrastructure.OwnerReferences, // Added by the machine set controller -- RECONCILED
			metav1.OwnerReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       cluster.Name,
				UID:        cluster.UID,
			},
		)
		setUID(machineSetInfrastructure)

		machineSet.Spec.Template.Spec.InfrastructureRef = corev1.ObjectReference{
			APIVersion: machineSetInfrastructure.APIVersion,
			Kind:       machineSetInfrastructure.Kind,
			Name:       machineSetInfrastructure.Name,
			Namespace:  machineSetInfrastructure.Namespace,
		}

		objs = append(objs, machineSet)

		machineSetBootstrap := &fakebootstrap.GenericBootstrapConfigTemplate{
			TypeMeta: metav1.TypeMeta{
				APIVersion: fakebootstrap.GroupVersion.String(),
				Kind:       "GenericBootstrapConfigTemplate",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      f.name,
				Namespace: cluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{ // Added by the machine set controller -- RECONCILED
					{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       cluster.Name,
						UID:        cluster.UID,
					},
				},
				// Labels: MISSING
			},
		}

		bootstrapConfig := f.bootstrapConfig
		if bootstrapConfig == nil {
			bootstrapConfig = NewBootstrapConfigTemplate(machineSetBootstrap)
		}

		machineSet.Spec.Template.Spec.Bootstrap = *bootstrapConfig

		// if the bootstrapConfig doesn't use a static secret, add the GenericBootstrapConfigTemplate to the object list
		if bootstrapConfig.ConfigRef != nil {
			objs = append(objs, machineSetBootstrap)
		}

		// if the infra template is specific to the machine set, add it to the object list
		if f.sharedInfrastructureTemplate == nil {
			objs = append(objs, machineSetInfrastructure)
		}
	}

	// Adds the objects for the machines controlled by the machineSet
	for _, machine := range f.machines {
		objs = append(objs, machine.Objs(cluster, false, machineSet, nil, nil)...)
	}

	return objs
}

type FakeMachine struct {
	name            string
	bootstrapConfig *clusterv1.Bootstrap
}

// NewFakeMachine return a FakeMachine that can generate a Machine object, all its own ancillary objects:
// - the machineInfrastructure object
// - the machineBootstrap object and the related bootstrapDataSecret
// If there is no a control plane object in the cluster, the first FakeMachine gets a generated sa secret.
func NewFakeMachine(name string) *FakeMachine {
	return &FakeMachine{
		name: name,
	}
}

func (f *FakeMachine) WithStaticBootstrapConfig() *FakeMachine {
	f.bootstrapConfig = NewStaticBootstrapConfig(f.name)
	return f
}

func (f *FakeMachine) Objs(cluster *clusterv1.Cluster, generateCerts bool, machineSet *clusterv1.MachineSet, machinePool *clusterv1.MachinePool, controlPlane *fakecontrolplane.GenericControlPlane) []client.Object {
	machineInfrastructure := &fakeinfrastructure.GenericInfrastructureMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeinfrastructure.GroupVersion.String(),
			Kind:       "GenericInfrastructureMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			// OwnerReferences: machine, Added by the machine controller (see below) -- RECONCILED
			// Labels: cluster.x-k8s.io/cluster-name=cluster, Added by the machine controller (see  below) -- RECONCILED
		},
	}

	bootstrapDataSecretName := f.name

	machineBootstrap := &fakebootstrap.GenericBootstrapConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakebootstrap.GroupVersion.String(),
			Kind:       "GenericBootstrapConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			// OwnerReferences: machine, Added by the machine controller (see below) -- RECONCILED
			// Labels: cluster.x-k8s.io/cluster-name=cluster, Added by the machine controller (see below) -- RECONCILED
		},
		Status: fakebootstrap.GenericBootstrapConfigStatus{
			DataSecretName: &bootstrapDataSecretName,
		},
	}

	bootstrapConfig := f.bootstrapConfig
	if bootstrapConfig == nil {
		bootstrapConfig = NewBootstrapConfig(machineBootstrap)
		bootstrapConfig.DataSecretName = &bootstrapDataSecretName
	}

	// Ensure the machineBootstrap gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(machineBootstrap)

	bootstrapDataSecret := &corev1.Secret{ // generated by the bootstrap controller -- ** NOT RECONCILED **
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapDataSecretName,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(machineBootstrap, machineBootstrap.GroupVersionKind()),
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name, // derives from Config -(ownerRef)-> machine.spec.ClusterName
			},
		},
	}

	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: cluster.Namespace,
			// Owner reference set by machine controller or by machineSet controller (see below)
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: cluster.Name, // Added by the machine controller (mirrors machine.spec.ClusterName) -- RECONCILED
			},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: machineInfrastructure.APIVersion,
				Kind:       machineInfrastructure.Kind,
				Name:       machineInfrastructure.Name,
				Namespace:  cluster.Namespace,
			},
			ClusterName: cluster.Name,
		},
	}

	// Ensure the machine gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(machine)

	var additionalObjs []client.Object

	switch {
	case machineSet != nil:
		// If this machine belong to a machineSet, it is controlled by it / ownership set by the machineSet controller -- ** NOT RECONCILED ?? **
		machine.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(machineSet, machineSet.GroupVersionKind())})
	case controlPlane != nil:
		// If this machine belong to a controlPlane, it is controlled by it / ownership set by the controlPlane controller -- ** NOT RECONCILED ?? **
		machine.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(controlPlane, controlPlane.GroupVersionKind())})
		// Sets the MachineControlPlane Label
		machine.Labels[clusterv1.MachineControlPlaneLabel] = ""
	case machinePool != nil:
		// If this machine belong to a machinePool, it is controlled by it / ownership set by the machinePool controller -- ** NOT RECONCILED **
		machine.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(machinePool, machinePool.GroupVersionKind())})
		// Sets the MachinePoolNameLabel
		machine.Labels[clusterv1.MachinePoolNameLabel] = machinePool.Name
	default:
		// If this machine does not belong to a machineSet or to a control plane, it is owned by the cluster / ownership set by the machine controller -- RECONCILED
		machine.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: cluster.APIVersion,
			Kind:       cluster.Kind,
			Name:       cluster.Name,
			UID:        cluster.UID,
		}})

		// Adds one of the certificate secret object generated by the bootstrap config controller -- ** NOT RECONCILED **
		if generateCerts {
			saSecret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cluster.GetName() + "-sa",
					Namespace: cluster.GetNamespace(),
					// Labels: cluster.x-k8s.io/cluster-name=cluster MISSING??
				},
			}
			// Set controlled by the machineBootstrap / ownership set by the bootstrap config controller -- ** NOT RECONCILED ?? **
			saSecret.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(machineBootstrap, machineBootstrap.GroupVersionKind())})

			additionalObjs = append(additionalObjs, saSecret)
		}
	}

	machineInfrastructure.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: machine.APIVersion,
			Kind:       machine.Kind,
			Name:       machine.Name,
			UID:        machine.UID,
		},
	})
	machineInfrastructure.SetLabels(map[string]string{
		clusterv1.ClusterNameLabel: machine.Spec.ClusterName,
	})

	objs := []client.Object{
		machine,
		machineInfrastructure,
	}

	if machinePool == nil {
		machine.Spec.Bootstrap = *bootstrapConfig
		if machine.Spec.Bootstrap.ConfigRef != nil {
			machineBootstrap.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: machine.APIVersion,
					Kind:       machine.Kind,
					Name:       machine.Name,
					UID:        machine.UID,
				},
			})
			machineBootstrap.SetLabels(map[string]string{
				clusterv1.ClusterNameLabel: machine.Spec.ClusterName,
			})

			objs = append(objs, bootstrapDataSecret, machineBootstrap)
		}
	}

	objs = append(objs, additionalObjs...)

	return objs
}

type FakeClusterResourceSet struct {
	name       string
	namespace  string
	secrets    []*corev1.Secret
	configMaps []*corev1.ConfigMap
	clusters   []*clusterv1.Cluster
}

// NewFakeClusterResourceSet return a FakeClusterResourceSet that can generate a ClusterResourceSet object, all its own ancillary objects:
// - the Secret/ConfigMap defining resources
// - the bindings that are created when a ClusterResourceSet is applied to a cluster.
func NewFakeClusterResourceSet(namespace, name string) *FakeClusterResourceSet {
	return &FakeClusterResourceSet{
		name:      name,
		namespace: namespace,
	}
}

func (f *FakeClusterResourceSet) WithSecret(name string) *FakeClusterResourceSet {
	f.secrets = append(f.secrets, &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.namespace,
		},
		// No data are required for the sake of move tests
	})
	return f
}

func (f *FakeClusterResourceSet) WithConfigMap(name string) *FakeClusterResourceSet {
	f.configMaps = append(f.configMaps, &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: f.namespace,
		},
		// No data are required for the sake of move tests
	})
	return f
}

func (f *FakeClusterResourceSet) ApplyToCluster(cluster *clusterv1.Cluster) *FakeClusterResourceSet {
	if f.namespace != cluster.Namespace {
		panic("A ClusterResourceSet can only be applied to a cluster in the same namespace")
	}
	f.clusters = append(f.clusters, cluster)
	return f
}

func (f *FakeClusterResourceSet) Objs() []client.Object {
	crs := &addonsv1.ClusterResourceSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterResourceSet",
			APIVersion: addonsv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
		},
		Spec: addonsv1.ClusterResourceSetSpec{
			Resources: []addonsv1.ResourceRef{},
		},
	}

	// Ensure the ClusterResourceSet gets a UID to be used by dependant objects for creating OwnerReferences.
	setUID(crs)

	objs := []client.Object{crs}

	// Ensures all the resources of type Secret are created and listed as a ClusterResourceSet resources
	for i := range f.secrets {
		secret := f.secrets[i]

		// secrets are owned by the ClusterResourceSet / ownership set by the ClusterResourceSet controller
		secret.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: crs.APIVersion,
			Kind:       crs.Kind,
			Name:       crs.Name,
			UID:        crs.UID,
		}})

		crs.Spec.Resources = append(crs.Spec.Resources, addonsv1.ResourceRef{
			Name: secret.Name,
			Kind: secret.Kind,
		})

		objs = append(objs, secret)
	}

	// Ensures all the resources of type ConfigMap are created and listed as a ClusterResourceSet resources
	for i := range f.configMaps {
		configMap := f.configMaps[i]

		// configMap are owned by the ClusterResourceSet / ownership set by the ClusterResourceSet controller
		configMap.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: crs.APIVersion,
			Kind:       crs.Kind,
			Name:       crs.Name,
			UID:        crs.UID,
		}})

		crs.Spec.Resources = append(crs.Spec.Resources, addonsv1.ResourceRef{
			Name: configMap.Name,
			Kind: configMap.Kind,
		})

		objs = append(objs, configMap)
	}

	// Ensures all the binding with the clusters where resources are applied.
	for _, cluster := range f.clusters {
		binding := &addonsv1.ClusterResourceSetBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterResourceSetBinding",
				APIVersion: addonsv1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
			Spec: addonsv1.ClusterResourceSetBindingSpec{
				ClusterName: cluster.Name,
				Bindings: []*addonsv1.ResourceSetBinding{
					{
						ClusterResourceSetName: crs.Name,
					},
				},
			},
		}

		binding.SetOwnerReferences([]metav1.OwnerReference{
			// binding are owned by the ClusterResourceSet / ownership set by the ClusterResourceSet controller
			{
				APIVersion: crs.APIVersion,
				Kind:       crs.Kind,
				Name:       crs.Name,
				UID:        crs.UID,
			},
		})

		objs = append(objs, binding)

		resourceSetBinding := addonsv1.ResourceSetBinding{
			ClusterResourceSetName: crs.Name,
			Resources:              []addonsv1.ResourceBinding{},
		}
		binding.Spec.Bindings = append(binding.Spec.Bindings, &resourceSetBinding)

		// creates map entries for each cluster/resource of type Secret
		for _, secret := range f.secrets {
			resourceSetBinding.Resources = append(resourceSetBinding.Resources, addonsv1.ResourceBinding{ResourceRef: addonsv1.ResourceRef{
				Name: secret.Name,
				Kind: "Secret",
			}})
		}

		// creates map entries for each cluster/resource of type ConfigMap
		for _, configMap := range f.configMaps {
			resourceSetBinding.Resources = append(resourceSetBinding.Resources, addonsv1.ResourceBinding{ResourceRef: addonsv1.ResourceRef{
				Name: configMap.Name,
				Kind: "ConfigMap",
			}})
		}
	}

	// Ensure all the objects gets UID.
	// Nb. This adds UID to all the objects; it does not change the UID explicitly sets in advance for the objects involved in the object graphs.
	for _, o := range objs {
		setUID(o)
	}

	return objs
}

type FakeExternalObject struct {
	name      string
	namespace string
}

// NewFakeExternalObject generates a new external object (a CR not related to the Cluster).
func NewFakeExternalObject(namespace, name string) *FakeExternalObject {
	return &FakeExternalObject{
		name:      name,
		namespace: namespace,
	}
}

func (f *FakeExternalObject) Objs() []client.Object {
	externalObj := &fakeexternal.GenericExternalObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeexternal.GroupVersion.String(),
			Kind:       "GenericExternalObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
		},
	}
	setUID(externalObj)

	return []client.Object{externalObj}
}

type FakeClusterExternalObject struct {
	name string
}

// NewFakeClusterExternalObject generates a new global external object (a CR not related to the Cluster).
func NewFakeClusterExternalObject(name string) *FakeClusterExternalObject {
	return &FakeClusterExternalObject{
		name: name,
	}
}

func (f *FakeClusterExternalObject) Objs() []client.Object {
	externalObj := &fakeexternal.GenericClusterExternalObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeexternal.GroupVersion.String(),
			Kind:       "GenericClusterExternalObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: f.name,
		},
	}
	setUID(externalObj)

	return []client.Object{externalObj}
}

type FakeClusterInfrastructureIdentity struct {
	name            string
	secretNamespace string
}

// NewFakeClusterInfrastructureIdentity generates a new global cluster identity object.
func NewFakeClusterInfrastructureIdentity(name string) *FakeClusterInfrastructureIdentity {
	return &FakeClusterInfrastructureIdentity{
		name: name,
	}
}

func (f *FakeClusterInfrastructureIdentity) WithSecretIn(namespace string) *FakeClusterInfrastructureIdentity {
	f.secretNamespace = namespace
	return f
}

func (f *FakeClusterInfrastructureIdentity) Objs() []client.Object {
	identityObj := &fakeinfrastructure.GenericClusterInfrastructureIdentity{
		TypeMeta: metav1.TypeMeta{
			APIVersion: fakeinfrastructure.GroupVersion.String(),
			Kind:       "GenericClusterInfrastructureIdentity",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: f.name,
		},
	}
	setUID(identityObj)
	objs := []client.Object{identityObj}

	if f.secretNamespace != "" {
		secret := NewSecret(f.secretNamespace, fmt.Sprintf("%s-credentials", f.name))
		setUID(secret)

		secret.SetOwnerReferences(append(secret.OwnerReferences, metav1.OwnerReference{
			APIVersion: identityObj.APIVersion,
			Kind:       identityObj.Kind,
			Name:       identityObj.Name,
			UID:        identityObj.UID,
		}))
		objs = append(objs, secret)
	}

	return objs
}

// NewSecret generates a new secret with the given namespace and name.
func NewSecret(namespace, name string) *corev1.Secret {
	s := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	setUID(s)
	return s
}

// SelectClusterObj finds and returns a Cluster with the given name and namespace, if any.
func SelectClusterObj(objs []client.Object, namespace, name string) *clusterv1.Cluster {
	for _, o := range objs {
		if o.GetObjectKind().GroupVersionKind().GroupKind() != clusterv1.GroupVersion.WithKind("Cluster").GroupKind() {
			continue
		}

		if o.GetName() == name && o.GetNamespace() == namespace {
			// Converts the object to cluster
			// NB. Convert returns an object without version/kind, so we are enforcing those values back.
			cluster := &clusterv1.Cluster{}
			if err := FakeScheme.Convert(o, cluster, nil); err != nil {
				panic(fmt.Sprintf("failed to convert %s to cluster: %v", o.GetObjectKind(), err))
			}
			cluster.APIVersion = o.GetObjectKind().GroupVersionKind().GroupVersion().String()
			cluster.Kind = o.GetObjectKind().GroupVersionKind().Kind
			return cluster
		}
	}
	return nil
}

// setUID assigns a UID to the object, so test objects are uniquely identified.
// NB. In order to make debugging easier we are using a human readable, deterministic string (instead of a random UID).
func setUID(obj client.Object) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		panic(fmt.Sprintf("failed to get accessor for test object: %v", err))
	}
	uid := fmt.Sprintf("%s, %s", obj.GetObjectKind().GroupVersionKind().String(), klog.KObj(accessor))
	accessor.SetUID(types.UID(uid))
}

// FakeClusterCustomResourceDefinition returns a fake CRD object for the given group/versions/kind.
func FakeClusterCustomResourceDefinition(group string, kind string, versions ...string) *apiextensionsv1.CustomResourceDefinition {
	crd := fakeCRD(group, kind, versions)
	crd.Spec.Scope = apiextensionsv1.ClusterScoped
	return crd
}

// FakeNamespacedCustomResourceDefinition returns a fake CRD object for the given group/versions/kind.
func FakeNamespacedCustomResourceDefinition(group string, kind string, versions ...string) *apiextensionsv1.CustomResourceDefinition {
	crd := fakeCRD(group, kind, versions)
	crd.Spec.Scope = apiextensionsv1.NamespaceScoped
	return crd
}

func fakeCRD(group string, kind string, versions []string) *apiextensionsv1.CustomResourceDefinition {
	crd := &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       apiextensionsv1.SchemeGroupVersion.String(),
			APIVersion: "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", strings.ToLower(kind), group), // NB. this technically should use plural(kind), but for the sake of test what really matters is to generate a unique name
			Labels: map[string]string{
				clusterctlv1.ClusterctlLabel: "",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{ // NB. the spec contains only what is strictly required by the move test
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: kind,
			},
		},
	}

	for i, version := range versions {
		// set the first version as a storage version
		versionObj := apiextensionsv1.CustomResourceDefinitionVersion{Name: version}
		if i == 0 {
			versionObj.Storage = true
		}
		crd.Spec.Versions = append(crd.Spec.Versions, versionObj)
	}
	return crd
}

// FakeCRDList returns FakeCustomResourceDefinitions for all the Types used in the test object graph.
func FakeCRDList() []*apiextensionsv1.CustomResourceDefinition {
	version := clusterv1.GroupVersion.Version

	// Ensure CRD for external objects is set as for "force move"
	externalCRD := FakeNamespacedCustomResourceDefinition(fakeexternal.GroupVersion.Group, "GenericExternalObject", version)
	externalCRD.Labels[clusterctlv1.ClusterctlMoveLabel] = ""

	clusterExternalCRD := FakeClusterCustomResourceDefinition(fakeexternal.GroupVersion.Group, "GenericClusterExternalObject", version)
	clusterExternalCRD.Labels[clusterctlv1.ClusterctlMoveLabel] = ""

	// Ensure CRD for GenericClusterInfrastructureIdentity is set for "force move hierarchy"
	clusterInfrastructureIdentityCRD := FakeClusterCustomResourceDefinition(fakeinfrastructure.GroupVersion.Group, "GenericClusterInfrastructureIdentity", version)
	clusterInfrastructureIdentityCRD.Labels[clusterctlv1.ClusterctlMoveHierarchyLabel] = ""

	return []*apiextensionsv1.CustomResourceDefinition{
		FakeNamespacedCustomResourceDefinition(clusterv1.GroupVersion.Group, "Cluster", version),
		FakeNamespacedCustomResourceDefinition(clusterv1.GroupVersion.Group, "ClusterClass", version),
		FakeNamespacedCustomResourceDefinition(clusterv1.GroupVersion.Group, "Machine", version),
		FakeNamespacedCustomResourceDefinition(clusterv1.GroupVersion.Group, "MachineDeployment", version),
		FakeNamespacedCustomResourceDefinition(clusterv1.GroupVersion.Group, "MachineSet", version),
		FakeNamespacedCustomResourceDefinition(expv1.GroupVersion.Group, "MachinePool", version),
		FakeNamespacedCustomResourceDefinition(addonsv1.GroupVersion.Group, "ClusterResourceSet", version),
		FakeNamespacedCustomResourceDefinition(addonsv1.GroupVersion.Group, "ClusterResourceSetBinding", version),
		FakeNamespacedCustomResourceDefinition(fakecontrolplane.GroupVersion.Group, "GenericControlPlane", version),
		FakeNamespacedCustomResourceDefinition(fakecontrolplane.GroupVersion.Group, "GenericControlPlaneTemplate", version),
		FakeNamespacedCustomResourceDefinition(fakeinfrastructure.GroupVersion.Group, "GenericInfrastructureCluster", version),
		FakeNamespacedCustomResourceDefinition(fakeinfrastructure.GroupVersion.Group, "GenericInfrastructureClusterTemplate", version),
		FakeNamespacedCustomResourceDefinition(fakeinfrastructure.GroupVersion.Group, "GenericInfrastructureMachine", version),
		FakeNamespacedCustomResourceDefinition(fakeinfrastructure.GroupVersion.Group, "GenericInfrastructureMachineTemplate", version),
		FakeNamespacedCustomResourceDefinition(fakebootstrap.GroupVersion.Group, "GenericBootstrapConfig", version),
		FakeNamespacedCustomResourceDefinition(fakebootstrap.GroupVersion.Group, "GenericBootstrapConfigTemplate", version),
		externalCRD,
		clusterExternalCRD,
		clusterInfrastructureIdentityCRD,
	}
}

type FakeClusterClass struct {
	namespace                                 string
	name                                      string
	infrastructureClusterTemplate             *unstructured.Unstructured
	controlPlaneTemplate                      *unstructured.Unstructured
	controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	workerMachineDeploymentClasses            []*FakeMachineDeploymentClass
}

func NewFakeClusterClass(namespace, name string) *FakeClusterClass {
	return &FakeClusterClass{
		namespace: namespace,
		name:      name,
	}
}

func (f *FakeClusterClass) WithInfrastructureClusterTemplate(tmpl *unstructured.Unstructured) *FakeClusterClass {
	f.infrastructureClusterTemplate = tmpl
	return f
}

func (f *FakeClusterClass) WithControlPlaneTemplate(tmpl *unstructured.Unstructured) *FakeClusterClass {
	f.controlPlaneTemplate = tmpl
	return f
}

func (f *FakeClusterClass) WithControlPlaneInfrastructureTemplate(tmpl *unstructured.Unstructured) *FakeClusterClass {
	f.controlPlaneInfrastructureMachineTemplate = tmpl
	return f
}

func (f *FakeClusterClass) WithWorkerMachineDeploymentClasses(classes []*FakeMachineDeploymentClass) *FakeClusterClass {
	f.workerMachineDeploymentClasses = classes
	return f
}

func (f *FakeClusterClass) Objs() []client.Object {
	// objMap map where the key is the object to which the owner reference to the cluster class should be added
	// and the value dictates if the onwner ref needs to be added.
	// This map also dual functions as a way to de-duplicate and template objects that are reused.
	objMap := map[client.Object]bool{}

	// If no infrastructure cluster template is provided create a generic infrastructure cluster template to use in the cluster class.
	if f.infrastructureClusterTemplate == nil {
		f.infrastructureClusterTemplate = builder.InfrastructureClusterTemplate(f.namespace, f.name).Build()
	}
	objMap[f.infrastructureClusterTemplate] = true

	// If no controlplane template is provided create a generic controlplane template to use in the cluster class.
	if f.controlPlaneTemplate == nil {
		f.controlPlaneTemplate = builder.ControlPlaneTemplate(f.namespace, f.name).Build()
	}
	objMap[f.controlPlaneTemplate] = true

	clusterClassBuilder := builder.ClusterClass(f.namespace, f.name).
		WithInfrastructureClusterTemplate(f.infrastructureClusterTemplate).
		WithControlPlaneTemplate(f.controlPlaneTemplate)

	if f.controlPlaneInfrastructureMachineTemplate != nil {
		clusterClassBuilder.WithControlPlaneInfrastructureMachineTemplate(f.controlPlaneInfrastructureMachineTemplate)
		objMap[f.controlPlaneInfrastructureMachineTemplate] = true
	}

	if len(f.workerMachineDeploymentClasses) > 0 {
		mdClasses := []clusterv1.MachineDeploymentClass{}
		for _, fakeMDClass := range f.workerMachineDeploymentClasses {
			mdClasses = append(mdClasses, *fakeMDClass.Obj())
			objMap[fakeMDClass.bootstrapTemplate] = true
			objMap[fakeMDClass.infrastructureTemplate] = true
		}
		clusterClassBuilder.WithWorkerMachineDeploymentClasses(mdClasses...)
	}

	clusterClass := clusterClassBuilder.Build()
	objMap[clusterClass] = false

	for o := range objMap {
		setUID(o)
	}

	for o, setOwnerReference := range objMap {
		if setOwnerReference {
			if err := controllerutil.SetOwnerReference(clusterClass, o, FakeScheme); err != nil {
				panic(err)
			}
		}
	}

	objs := []client.Object{}
	for o := range objMap {
		objs = append(objs, o)
	}
	return objs
}

type FakeMachineDeploymentClass struct {
	class                  string
	namespace              string // Used when creating the default bootstrap and the infra machine templates
	infrastructureTemplate *unstructured.Unstructured
	bootstrapTemplate      *unstructured.Unstructured
}

func NewFakeMachineDeploymentClass(namespace, class string) *FakeMachineDeploymentClass {
	return &FakeMachineDeploymentClass{
		class:     class,
		namespace: namespace,
	}
}

func (f *FakeMachineDeploymentClass) WithInfrastructureMachineTemplate(tmpl *unstructured.Unstructured) *FakeMachineDeploymentClass {
	f.infrastructureTemplate = tmpl
	return f
}

func (f *FakeMachineDeploymentClass) WithBootstrapTemplate(tmpl *unstructured.Unstructured) *FakeMachineDeploymentClass {
	f.bootstrapTemplate = tmpl
	return f
}

func (f *FakeMachineDeploymentClass) Obj() *clusterv1.MachineDeploymentClass {
	if f.infrastructureTemplate == nil {
		f.infrastructureTemplate = builder.InfrastructureMachineTemplate(f.namespace, f.class).Build()
	}
	if f.bootstrapTemplate == nil {
		f.bootstrapTemplate = builder.BootstrapTemplate(f.namespace, f.class).Build()
	}

	return builder.MachineDeploymentClass(f.class).
		WithInfrastructureTemplate(f.infrastructureTemplate).
		WithBootstrapTemplate(f.bootstrapTemplate).
		Build()
}
