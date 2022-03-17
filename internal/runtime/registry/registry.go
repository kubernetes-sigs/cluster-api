package registry

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

func New() Registry {
	return &registry{
		registrations: map[string]RuntimeExtensionRegistration{},
	}
}

type registry struct {
	registrations map[string]RuntimeExtensionRegistration
}

type RuntimeExtensionRegistration struct {
	Name             string
	GroupVersionHook catalog.GroupVersionHook
	ClientConfig     ClientConfig
}

type ClientConfig struct {
	WebhookClientConfig runtimev1.WebhookClientConfig
	TimeoutSeconds      *int32
	FailurePolicy       *runtimev1.FailurePolicyType
	NamespaceSelector   *metav1.LabelSelector
}

func (r registry) RegisterRuntimeExtension(ext *runtimev1.Extension) {
	panic("implement me")
}

func (r registry) RemoveRuntimeExtension(ext *runtimev1.Extension) {
	panic("implement me")
}

func (r registry) GetRuntimeExtension(gvh catalog.GroupVersionHook, name string) RuntimeExtensionRegistration {
	panic("implement me")
}

func (r registry) GetRuntimeExtensions(gvh catalog.GroupVersionHook) []RuntimeExtensionRegistration {
	panic("implement me")
}

type Registry interface {
	RemoveRuntimeExtension(ext *runtimev1.Extension)
	RegisterRuntimeExtension(ext *runtimev1.Extension)

	GetRuntimeExtension(gvh catalog.GroupVersionHook, name string) RuntimeExtensionRegistration
	GetRuntimeExtensions(gvh catalog.GroupVersionHook) []RuntimeExtensionRegistration
}
