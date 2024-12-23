/*
Copyright 2022 The Kubernetes Authors.

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

package topologymutation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	apiyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/yaml"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/desiredstate"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	"sigs.k8s.io/cluster-api/util/contract"
	"sigs.k8s.io/cluster-api/webhooks"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func TestHandler(t *testing.T) {
	g := NewWithT(t)

	// Enable RuntimeSDK for this test so we can use RuntimeExtensions.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	// Get a scope based on the Cluster and ClusterClass.
	cluster := getCluster()
	clusterVariableImageRepository := "kindest"
	cluster.Spec.Topology.Variables = []clusterv1.ClusterVariable{
		{
			Name:  "imageRepository",
			Value: apiextensionsv1.JSON{Raw: []byte("\"" + clusterVariableImageRepository + "\"")},
		},
	}
	clusterClassFile := "./testdata/clusterclass-quick-start-runtimesdk.yaml"
	s, err := getScope(cluster, clusterClassFile)
	g.Expect(err).ToNot(HaveOccurred())

	// Create a RuntimeClient that is backed by our Runtime Extension.
	runtimeClient := &injectRuntimeClient{
		runtimeExtension: NewExtensionHandlers(testScheme),
	}

	// Create a ClusterClassReconciler.
	fakeClient, mgr, err := createClusterClassFakeClientAndManager(s.Blueprint)
	g.Expect(err).ToNot(HaveOccurred())
	clusterClassReconciler := controllers.ClusterClassReconciler{
		Client:        fakeClient,
		RuntimeClient: runtimeClient,
	}
	err = clusterClassReconciler.SetupWithManager(ctx, mgr, controller.Options{})
	g.Expect(err).ToNot(HaveOccurred())

	// Create a desired state generator.
	desiredStateGenerator := desiredstate.NewGenerator(nil, nil, runtimeClient)

	// Note: as of today we don't have to set any fields and also don't have to call
	// SetupWebhookWithManager because DefaultAndValidateVariables doesn't need any of that.
	clusterWebhook := webhooks.Cluster{}

	// Reconcile ClusterClass.
	// Note: this also reconciles variables from inline and external variables into ClusterClass.status.
	_, err = clusterClassReconciler.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: s.Blueprint.ClusterClass.Namespace,
			Name:      s.Blueprint.ClusterClass.Name,
		}})
	g.Expect(err).ToNot(HaveOccurred())
	// Overwrite ClusterClass in blueprint with the reconciled ClusterClass.
	err = clusterClassReconciler.Client.Get(ctx, client.ObjectKeyFromObject(s.Blueprint.ClusterClass), s.Blueprint.ClusterClass)
	g.Expect(err).ToNot(HaveOccurred())

	// Run variable defaulting and validation on the Cluster object.
	errs := clusterWebhook.DefaultAndValidateVariables(ctx, s.Current.Cluster, nil, s.Blueprint.ClusterClass)
	g.Expect(errs.ToAggregate()).ToNot(HaveOccurred())

	// Return the desired state.
	desiredState, err := desiredStateGenerator.Generate(ctx, s)
	g.Expect(err).ToNot(HaveOccurred())

	dockerClusterImageRepository, found, err := unstructured.NestedString(desiredState.InfrastructureCluster.Object, "spec", "loadBalancer", "imageRepository")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(dockerClusterImageRepository).To(Equal(clusterVariableImageRepository))
}

func getCluster() *clusterv1.Cluster {
	return &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-name-test",
			Namespace: "namespace-name-test",
		},
		Spec: clusterv1.ClusterSpec{
			ClusterNetwork: &clusterv1.ClusterNetwork{
				Pods: &clusterv1.NetworkRanges{
					CIDRBlocks: []string{"192.168.0.0/16"},
				},
			},
			Topology: &clusterv1.Topology{
				Version: "v1.29.0",
				// NOTE: Class name must match the ClusterClass name.
				Class: "quick-start-runtimesdk",
				ControlPlane: clusterv1.ControlPlaneTopology{
					Replicas: ptr.To[int32](1),
				},
				Workers: &clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Name: "md-test1",
							// NOTE: MachineDeploymentClass name must match what is defined in ClusterClass packages.
							Class:    "default-worker",
							Replicas: ptr.To[int32](1),
						},
					},
				},
			},
		},
	}
}

func createClusterClassFakeClientAndManager(blueprint *scope.ClusterBlueprint) (client.Client, manager.Manager, error) {
	scheme := runtime.NewScheme()
	_ = clusterv1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	objs := []client.Object{
		blueprint.ClusterClass,
	}

	unstructuredObjs := []*unstructured.Unstructured{
		blueprint.InfrastructureClusterTemplate,
		blueprint.ControlPlane.Template,
		blueprint.ControlPlane.InfrastructureMachineTemplate,
	}
	for _, md := range blueprint.MachineDeployments {
		unstructuredObjs = append(unstructuredObjs, md.InfrastructureMachineTemplate, md.BootstrapTemplate)
	}
	for _, mp := range blueprint.MachinePools {
		unstructuredObjs = append(unstructuredObjs, mp.InfrastructureMachinePoolTemplate, mp.BootstrapTemplate)
	}

	objAlreadyAdded := sets.Set[string]{}
	crdAlreadyAdded := sets.Set[string]{}
	for _, unstructuredObj := range unstructuredObjs {
		if !objAlreadyAdded.Has(unstructuredObj.GroupVersionKind().Kind + "/" + unstructuredObj.GetName()) {
			objs = append(objs, unstructuredObj)
			objAlreadyAdded.Insert(unstructuredObj.GroupVersionKind().Kind + "/" + unstructuredObj.GetName())
		}

		crd := generateCRDForUnstructured(unstructuredObj)
		if !crdAlreadyAdded.Has(crd.Name) {
			objs = append(objs, crd)
			crdAlreadyAdded.Insert(crd.Name)
		}
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&clusterv1.ClusterClass{}).Build()

	mgr, err := ctrl.NewManager(&rest.Config{}, ctrl.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}

	return fakeClient, mgr, err
}

func generateCRDForUnstructured(u *unstructured.Unstructured) *apiextensionsv1.CustomResourceDefinition {
	gvk := u.GroupVersionKind()
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: contract.CalculateCRDName(gvk.Group, gvk.Kind),
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): gvk.Version,
			},
		},
	}
}

// getScope gets blueprint (ClusterClass) and current state based on cluster and clusterClassFile.
func getScope(cluster *clusterv1.Cluster, clusterClassFile string) (*scope.Scope, error) {
	clusterClassYAML, err := os.ReadFile(clusterClassFile) //nolint:gosec // reading a file in tests is not a security issue.
	if err != nil {
		return nil, err
	}

	// Get all objects by groupVersionKindName.
	parsedObjects, err := parseObjects(clusterClassYAML)
	if err != nil {
		return nil, err
	}

	s := scope.New(cluster)
	s.Current.ControlPlane = &scope.ControlPlaneState{}
	s.Blueprint = &scope.ClusterBlueprint{
		Topology:           cluster.Spec.Topology,
		ControlPlane:       &scope.ControlPlaneBlueprint{},
		MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
		MachinePools:       map[string]*scope.MachinePoolBlueprint{},
	}

	// Get ClusterClass and referenced templates.
	// ClusterClass
	s.Blueprint.ClusterClass = mustFind(findObject[*clusterv1.ClusterClass](parsedObjects, groupVersionKindName{
		Kind: "ClusterClass",
	}))
	// Set paused condition for ClusterClass
	v1beta2conditions.Set(s.Blueprint.ClusterClass, metav1.Condition{
		Type:               clusterv1.PausedV1Beta2Condition,
		Status:             metav1.ConditionFalse,
		Reason:             clusterv1.NotPausedV1Beta2Reason,
		ObservedGeneration: s.Blueprint.ClusterClass.GetGeneration(),
	})
	// InfrastructureClusterTemplate
	s.Blueprint.InfrastructureClusterTemplate = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(s.Blueprint.ClusterClass.Spec.Infrastructure.Ref)))

	// ControlPlane
	s.Blueprint.ControlPlane.Template = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(s.Blueprint.ClusterClass.Spec.ControlPlane.Ref)))
	if s.Blueprint.HasControlPlaneInfrastructureMachine() {
		s.Blueprint.ControlPlane.InfrastructureMachineTemplate = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(s.Blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref)))
	}
	if s.Blueprint.HasControlPlaneMachineHealthCheck() {
		s.Blueprint.ControlPlane.MachineHealthCheck = s.Blueprint.ClusterClass.Spec.ControlPlane.MachineHealthCheck
	}

	// MachineDeployments.
	for _, machineDeploymentClass := range s.Blueprint.ClusterClass.Spec.Workers.MachineDeployments {
		machineDeploymentBlueprint := &scope.MachineDeploymentBlueprint{}
		machineDeploymentClass.Template.Metadata.DeepCopyInto(&machineDeploymentBlueprint.Metadata)
		machineDeploymentBlueprint.InfrastructureMachineTemplate = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(machineDeploymentClass.Template.Infrastructure.Ref)))
		machineDeploymentBlueprint.BootstrapTemplate = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(machineDeploymentClass.Template.Bootstrap.Ref)))
		if machineDeploymentClass.MachineHealthCheck != nil {
			machineDeploymentBlueprint.MachineHealthCheck = machineDeploymentClass.MachineHealthCheck
		}
		s.Blueprint.MachineDeployments[machineDeploymentClass.Class] = machineDeploymentBlueprint
	}

	// MachinePools.
	for _, machinePoolClass := range s.Blueprint.ClusterClass.Spec.Workers.MachinePools {
		machinePoolBlueprint := &scope.MachinePoolBlueprint{}
		machinePoolClass.Template.Metadata.DeepCopyInto(&machinePoolBlueprint.Metadata)
		machinePoolBlueprint.InfrastructureMachinePoolTemplate = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(machinePoolClass.Template.Infrastructure.Ref)))
		machinePoolBlueprint.BootstrapTemplate = mustFind(findObject[*unstructured.Unstructured](parsedObjects, refToGroupVersionKindName(machinePoolClass.Template.Bootstrap.Ref)))
		s.Blueprint.MachinePools[machinePoolClass.Class] = machinePoolBlueprint
	}

	return s, nil
}

type groupVersionKindName struct {
	Name       string
	APIVersion string
	Kind       string
}

// parseObjects parses objects in clusterClassYAML and returns them by groupVersionKindName.
func parseObjects(clusterClassYAML []byte) (map[groupVersionKindName]runtime.Object, error) {
	// Only adding clusterv1 as we want to parse everything else as Unstructured,
	// because everything else is stored as Unstructured in Scope.
	scheme := runtime.NewScheme()
	_ = clusterv1.AddToScheme(scheme)
	universalDeserializer := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	parsedObjects := map[groupVersionKindName]runtime.Object{}
	// Inspired by cluster-api/util/yaml.ToUnstructured
	reader := apiyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(clusterClassYAML)))
	for {
		// Read one YAML document at a time, until io.EOF is returned
		objectBytes, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, errors.Wrapf(err, "failed to read YAML")
		}
		if len(objectBytes) == 0 {
			break
		}

		obj, gvk, err := universalDeserializer.Decode(objectBytes, nil, nil)
		// Unmarshal to Unstructured if the type of the object is not registered.
		if runtime.IsNotRegisteredError(err) {
			u := &unstructured.Unstructured{}
			if err := yaml.Unmarshal(objectBytes, u); err != nil {
				return nil, err
			}
			parsedObjects[groupVersionKindName{
				Name:       u.GetName(),
				APIVersion: u.GroupVersionKind().GroupVersion().String(),
				Kind:       u.GroupVersionKind().Kind,
			}] = u
			continue
		}
		// Return if we got another error
		if err != nil {
			return nil, err
		}

		// Add the unmarshalled typed object.
		metaObj, ok := obj.(metav1.Object)
		if !ok {
			return nil, errors.Errorf("found an object which is not a metav1.Object")
		}
		parsedObjects[groupVersionKindName{
			Name:       metaObj.GetName(),
			APIVersion: gvk.GroupVersion().String(),
			Kind:       gvk.Kind,
		}] = obj
	}
	return parsedObjects, nil
}

func mustFind[K runtime.Object](obj K, err error) K {
	if err != nil {
		panic(err)
	}
	return obj
}

// findObject looks up an object with the given groupVersionKindName in the given objects map.
func findObject[K runtime.Object](objects map[groupVersionKindName]runtime.Object, groupVersionKindName groupVersionKindName) (K, error) {
	var res K
	var alreadyFound bool
	for gvkn, obj := range objects {
		if groupVersionKindName.Name != "" && groupVersionKindName.Name != gvkn.Name {
			continue
		}
		if groupVersionKindName.APIVersion != "" && groupVersionKindName.APIVersion != gvkn.APIVersion {
			continue
		}
		if groupVersionKindName.Kind != "" && groupVersionKindName.Kind != gvkn.Kind {
			continue
		}

		if alreadyFound {
			return res, errors.Errorf("found multiple objects matching %v", groupVersionKindName)
		}

		objK, ok := obj.(K)
		if !ok {
			return res, errors.Errorf("found an object matching %v, but it has the wrong type", groupVersionKindName)
		}
		res = objK
		alreadyFound = true
	}

	return res, nil
}

func refToGroupVersionKindName(ref *corev1.ObjectReference) groupVersionKindName {
	return groupVersionKindName{
		APIVersion: ref.APIVersion,
		Kind:       ref.Kind,
		Name:       ref.Name,
	}
}

type TopologyMutationHook interface {
	DiscoverVariables(ctx context.Context, req *runtimehooksv1.DiscoverVariablesRequest, resp *runtimehooksv1.DiscoverVariablesResponse)
	GeneratePatches(ctx context.Context, req *runtimehooksv1.GeneratePatchesRequest, resp *runtimehooksv1.GeneratePatchesResponse)
	ValidateTopology(ctx context.Context, req *runtimehooksv1.ValidateTopologyRequest, resp *runtimehooksv1.ValidateTopologyResponse)
}

var _ runtimeclient.Client = &injectRuntimeClient{}

// injectRuntimeClient implements a runtimeclient.Client.
// It allows us to plug a TopologyMutationHook into Cluster and ClusterClass controllers.
type injectRuntimeClient struct {
	runtimeExtension TopologyMutationHook
}

func (i injectRuntimeClient) CallExtension(ctx context.Context, hook runtimecatalog.Hook, _ metav1.Object, _ string, req runtimehooksv1.RequestObject, resp runtimehooksv1.ResponseObject, _ ...runtimeclient.CallExtensionOption) error {
	// Note: We have to copy the requests. Otherwise we could get side effect by Runtime Extensions
	// modifying the request instead of properly returning a response. Also after Unmarshal,
	// only the Raw fields in runtime.RawExtension fields should be filled out and Object should be nil.
	// This wouldn't be the case without the copy.
	switch runtimecatalog.HookName(hook) {
	case runtimecatalog.HookName(runtimehooksv1.DiscoverVariables):
		reqCopy, err := copyObject[runtimehooksv1.DiscoverVariablesRequest](req.(*runtimehooksv1.DiscoverVariablesRequest))
		if err != nil {
			return err
		}
		i.runtimeExtension.DiscoverVariables(ctx, reqCopy, resp.(*runtimehooksv1.DiscoverVariablesResponse))
		if resp.GetStatus() == runtimehooksv1.ResponseStatusFailure {
			return errors.Errorf("failed to call extension handler: got failure response: %v", resp.GetMessage())
		}
		return nil
	case runtimecatalog.HookName(runtimehooksv1.GeneratePatches):
		reqCopy, err := copyObject[runtimehooksv1.GeneratePatchesRequest](req.(*runtimehooksv1.GeneratePatchesRequest))
		if err != nil {
			return err
		}
		i.runtimeExtension.GeneratePatches(ctx, reqCopy, resp.(*runtimehooksv1.GeneratePatchesResponse))
		if resp.GetStatus() == runtimehooksv1.ResponseStatusFailure {
			return errors.Errorf("failed to call extension handler: got failure response: %v", resp.GetMessage())
		}
		return nil
	case runtimecatalog.HookName(runtimehooksv1.ValidateTopology):
		reqCopy, err := copyObject[runtimehooksv1.ValidateTopologyRequest](req.(*runtimehooksv1.ValidateTopologyRequest))
		if err != nil {
			return err
		}
		i.runtimeExtension.ValidateTopology(ctx, reqCopy, resp.(*runtimehooksv1.ValidateTopologyResponse))
		if resp.GetStatus() == runtimehooksv1.ResponseStatusFailure {
			return errors.Errorf("failed to call extension handler: got failure response: %v", resp.GetMessage())
		}
		return nil
	}
	panic("implement me")
}

// copyObject copies an object with json Marshal & Unmarshal.
func copyObject[T any](obj *T) (*T, error) {
	objCopy := new(T)

	reqBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(reqBytes, &objCopy); err != nil {
		return nil, err
	}

	return objCopy, nil
}

func (i injectRuntimeClient) WarmUp(_ *runtimev1.ExtensionConfigList) error {
	panic("implement me")
}

func (i injectRuntimeClient) IsReady() bool {
	panic("implement me")
}

func (i injectRuntimeClient) Discover(_ context.Context, _ *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	panic("implement me")
}

func (i injectRuntimeClient) Register(_ *runtimev1.ExtensionConfig) error {
	panic("implement me")
}

func (i injectRuntimeClient) Unregister(_ *runtimev1.ExtensionConfig) error {
	panic("implement me")
}

func (i injectRuntimeClient) CallAllExtensions(_ context.Context, _ runtimecatalog.Hook, _ metav1.Object, _ runtimehooksv1.RequestObject, _ runtimehooksv1.ResponseObject) error {
	panic("implement me")
}
