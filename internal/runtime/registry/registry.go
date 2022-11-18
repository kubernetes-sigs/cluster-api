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

package registry

import (
	"sync"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

// ExtensionRegistry defines the funcs of a RuntimeExtension registry.
type ExtensionRegistry interface {
	// WarmUp can be used to initialize a "cold" RuntimeExtension registry with all
	// known runtimev1.ExtensionConfigs at a given time.
	// After WarmUp completes the RuntimeExtension registry is considered ready.
	WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error

	// IsReady returns true if the RuntimeExtension registry is ready for usage.
	// This happens after WarmUp is completed.
	IsReady() bool

	// Add adds all RuntimeExtensions of the given ExtensionConfig.
	// Please note that if the ExtensionConfig has been added before, the
	// corresponding registry entries will get updated/replaced with the
	// one from the newly provided ExtensionConfig.
	Add(extensionConfig *runtimev1.ExtensionConfig) error

	// Remove removes all RuntimeExtensions corresponding to the provided ExtensionConfig.
	Remove(extensionConfig *runtimev1.ExtensionConfig) error

	// List lists all registered RuntimeExtensions for a given catalog.GroupHook.
	List(gh runtimecatalog.GroupHook) ([]*ExtensionRegistration, error)

	// Get gets the RuntimeExtensions with the given name.
	Get(name string) (*ExtensionRegistration, error)
}

// ExtensionRegistration contains information about a registered RuntimeExtension.
type ExtensionRegistration struct {
	// Name is the unique name of the RuntimeExtension.
	Name string

	// ExtensionConfigName is the name of the corresponding ExtensionConfig.
	ExtensionConfigName string

	// GroupVersionHook is the GroupVersionHook that the RuntimeExtension implements.
	GroupVersionHook runtimecatalog.GroupVersionHook

	// NamespaceSelector limits the objects by namespace for which a Runtime Extension is called.
	NamespaceSelector labels.Selector

	// ClientConfig is the ClientConfig to communicate with the RuntimeExtension.
	ClientConfig runtimev1.ClientConfig

	// TimeoutSeconds is the timeout duration used for calls to the RuntimeExtension.
	TimeoutSeconds *int32

	// FailurePolicy defines how failures in calls to the RuntimeExtension should be handled by a client.
	FailurePolicy *runtimev1.FailurePolicy

	// Settings captures additional information sent in call to the RuntimeExtensions.
	Settings map[string]string
}

// extensionRegistry is an implementation of ExtensionRegistry.
type extensionRegistry struct {
	// ready represents if the registry has been warmed up.
	ready bool
	// items contains the registry entries.
	items map[string]*ExtensionRegistration
	// lock is used to synchronize access to fields of the extensionRegistry.
	lock sync.RWMutex
}

// New returns a new ExtensionRegistry.
func New() ExtensionRegistry {
	return &extensionRegistry{
		items: map[string]*ExtensionRegistration{},
	}
}

// WarmUp can be used to initialize a "cold" RuntimeExtension registry with all
// known runtimev1.ExtensionConfigs at a given time.
// After WarmUp completes the RuntimeExtension registry is considered ready.
func (r *extensionRegistry) WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error {
	if extensionConfigList == nil {
		return errors.New("failed to warm up registry: invalid argument: when calling WarmUp ExtensionConfigList must not be nil")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.ready {
		return errors.New("failed to warm up registry: invalid operation: WarmUp cannot be called on a registry which has already been warmed up")
	}

	var allErrs []error
	for i := range extensionConfigList.Items {
		if err := r.add(&extensionConfigList.Items[i]); err != nil {
			allErrs = append(allErrs, err)
		}
	}
	if len(allErrs) > 0 {
		// Reset the map, so that the next WarmUp can start with an empty map
		// and doesn't inherit entries from this failed WarmUp.
		r.items = map[string]*ExtensionRegistration{}
		return errors.Wrapf(kerrors.NewAggregate(allErrs), "failed to warm up registry")
	}

	r.ready = true
	return nil
}

// IsReady returns true if the RuntimeExtension registry is ready for usage.
// This happens after WarmUp is completed.
func (r *extensionRegistry) IsReady() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.ready
}

// Add adds all RuntimeExtensions of the given ExtensionConfig.
// Please note that if the ExtensionConfig has been added before, the
// corresponding registry entries will get updated/replaced with the
// one from the newly provided ExtensionConfig.
func (r *extensionRegistry) Add(extensionConfig *runtimev1.ExtensionConfig) error {
	if extensionConfig == nil {
		return errors.New("failed to add ExtensionConfig to registry: invalid argument: when calling Add extensionConfig must not be nil")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.ready {
		return errors.Errorf("failed to add ExtensionConfig %q to registry: invalid operation: Add cannot be called on a registry which has not been warmed up", extensionConfig.Name)
	}

	return r.add(extensionConfig)
}

// Remove removes all RuntimeExtensions corresponding to the provided ExtensionConfig.
func (r *extensionRegistry) Remove(extensionConfig *runtimev1.ExtensionConfig) error {
	if extensionConfig == nil {
		return errors.New("failed to remove ExtensionConfig from registry: invalid argument: when calling Remove ExtensionConfig must not be nil")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.ready {
		return errors.Errorf("failed to remove ExtensionConfig %q from registry: invalid operation: Remove cannot be called on a registry which has not been warmed up", extensionConfig.Name)
	}

	r.remove(extensionConfig)
	return nil
}

func (r *extensionRegistry) remove(extensionConfig *runtimev1.ExtensionConfig) {
	for _, e := range r.items {
		if e.ExtensionConfigName == extensionConfig.Name {
			delete(r.items, e.Name)
		}
	}
}

// List lists all registered RuntimeExtensions for a given catalog.GroupHook.
func (r *extensionRegistry) List(gh runtimecatalog.GroupHook) ([]*ExtensionRegistration, error) {
	if gh.Group == "" {
		return nil, errors.New("failed to list extension handlers: invalid argument: when calling List gh.Group must not be empty")
	}
	if gh.Hook == "" {
		return nil, errors.New("failed to list extension handlers: invalid argument: when calling List gh.Hook must not be empty")
	}

	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.ready {
		return nil, errors.Errorf("failed to list extension handlers for GroupHook %q: invalid operation: List cannot be called on a registry which has not been warmed up", gh.String())
	}

	l := []*ExtensionRegistration{}
	for _, registration := range r.items {
		if registration.GroupVersionHook.Group == gh.Group && registration.GroupVersionHook.Hook == gh.Hook {
			l = append(l, registration)
		}
	}
	return l, nil
}

// Get gets the RuntimeExtensions with the given name.
func (r *extensionRegistry) Get(name string) (*ExtensionRegistration, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.ready {
		return nil, errors.Errorf("failed to get extension handler %q from registry: invalid operation: Get cannot be called on a registry not yet ready", name)
	}

	registration, ok := r.items[name]
	if !ok {
		return nil, errors.Errorf("failed to get extension handler %q from registry: handler with name %q has not been registered", name, name)
	}

	return registration, nil
}

func (r *extensionRegistry) add(extensionConfig *runtimev1.ExtensionConfig) error {
	r.remove(extensionConfig)

	// Create a selector from the NamespaceSelector defined in the extensionConfig spec.
	selector, err := metav1.LabelSelectorAsSelector(extensionConfig.Spec.NamespaceSelector)
	if err != nil {
		return errors.Wrapf(err, "failed to add ExtensionConfig %q to registry: failed to create namespaceSelector", extensionConfig.Name)
	}

	var allErrs []error
	registrations := []*ExtensionRegistration{}
	for _, e := range extensionConfig.Status.Handlers {
		gv, err := schema.ParseGroupVersion(e.RequestHook.APIVersion)
		if err != nil {
			allErrs = append(allErrs, errors.Wrapf(err, "failed to add extension handler %q to registry: failed to parse GroupVersion %q of handler %q", e.Name, e.RequestHook.APIVersion, e.Name))
			continue
		}

		// Registrations will only be added to the registry if no errors occur (all or nothing).
		registrations = append(registrations, &ExtensionRegistration{
			ExtensionConfigName: extensionConfig.Name,
			Name:                e.Name,
			GroupVersionHook: runtimecatalog.GroupVersionHook{
				Group:   gv.Group,
				Version: gv.Version,
				Hook:    e.RequestHook.Hook,
			},
			NamespaceSelector: selector,
			ClientConfig:      extensionConfig.Spec.ClientConfig,
			TimeoutSeconds:    e.TimeoutSeconds,
			FailurePolicy:     e.FailurePolicy,
			Settings:          extensionConfig.Spec.Settings,
		})
	}

	if len(allErrs) > 0 {
		return errors.Wrapf(kerrors.NewAggregate(allErrs), "failed to add ExtensionConfig %q to registry", extensionConfig.Name)
	}

	for _, registration := range registrations {
		r.items[registration.Name] = registration
	}

	return nil
}
