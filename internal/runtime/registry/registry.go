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
	"k8s.io/apimachinery/pkg/runtime/schema"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

// TODO: handle namespace selector.

var (
	once     sync.Once
	instance ExtensionRegistry
)

// ExtensionRegistry defines all the func an extension registry supports.
type ExtensionRegistry interface {
	// WarmUp can be used to initialise a "cold" extension registry with all the known extensions at a given time.
	// After WarmUp completes the extension registry is considered ready.
	WarmUp(ext *runtimev1.ExtensionConfigList) error

	// IsReady returns true if the extension registry is ready for usage, and this happens
	// after WarmUp is completed.
	IsReady() bool

	// Add all the Runtime Extensions served by a registered runtime extension server.
	// Please note that if the provided registration object already exists, all the registered
	// RuntimeExtensions gets updated according to the newly provide status.
	Add(ext *runtimev1.ExtensionConfig) error

	// Remove all the Runtime Extensions served by a registered runtime extension server.
	Remove(ext *runtimev1.ExtensionConfig) error

	// List all the registered Runtime Extensions for a given Group/Hook.
	List(gh catalog.GroupVersionHook) ([]*RuntimeExtensionRegistration, error)

	// Get the Runtime Extensions with a given name
	Get(name string) (*RuntimeExtensionRegistration, error)
}

// RuntimeExtensionRegistration contain info about a registered Runtime Extension.
type RuntimeExtensionRegistration struct {
	// RegistrationName is the name of the object who originated the registration of this Runtime Extension.
	RegistrationName string

	// The unique name of the Runtime Extension.
	Name string

	// The GroupVersionHook the Runtime Extension implements.
	GroupVersionHook catalog.GroupVersionHook

	ClientConfig   runtimev1.ClientConfig
	TimeoutSeconds *int32
	FailurePolicy  *runtimev1.FailurePolicy
}

type extensionRegistry struct {
	ready bool
	items map[string]*RuntimeExtensionRegistration
	lock  sync.RWMutex
}

// Extensions provide access to all the registered Runtime Extensions.
func Extensions() ExtensionRegistry {
	once.Do(func() {
		instance = extensions()
	})
	return instance
}

func extensions() ExtensionRegistry {
	return &extensionRegistry{
		items: map[string]*RuntimeExtensionRegistration{},
	}
}

// WarmUp can be used to initialise a "cold" extension registry with all the known extensions at a given time.
// After WarmUp completes the extension registry is considered ready.
func (extensions *extensionRegistry) WarmUp(extList *runtimev1.ExtensionConfigList) error {
	if extList == nil {
		return errors.New("invalid argument, when calling WarmUp extList must not be nil")
	}

	extensions.lock.Lock()
	defer extensions.lock.Unlock()

	for i := range extList.Items {
		if err := extensions.add(&extList.Items[i]); err != nil { // TODO: consider if to aggregate errors
			return err
		}
	}

	extensions.ready = true
	return nil
}

// IsReady returns true if the extension registry is ready for usage, and this happens
// after WarmUp is completed.
func (extensions *extensionRegistry) IsReady() bool {
	extensions.lock.RLock()
	defer extensions.lock.RUnlock()

	return extensions.ready
}

// Add all the Runtime Extensions served by a registered runtime extension server.
// Please note that if the provided registration object already exists, all the registered
// RuntimeExtensions gets updated according to the newly provide status.
func (extensions *extensionRegistry) Add(ext *runtimev1.ExtensionConfig) error {
	if !extensions.ready {
		return errors.New("invalid operation: Get cannot called on a registry not yet ready")
	}

	if ext == nil {
		return errors.New("invalid argument, when calling Add ext must not be nil")
	}

	extensions.lock.Lock()
	defer extensions.lock.Unlock()
	return extensions.add(ext)
}

// Remove all the Runtime Extensions served by a registered runtime extension server.
func (extensions *extensionRegistry) Remove(ext *runtimev1.ExtensionConfig) error {
	if !extensions.ready {
		return errors.New("invalid operation: Get cannot called on a registry not yet ready")
	}

	if ext == nil {
		return errors.New("invalid argument, when calling Remove ext must not be nil")
	}

	extensions.lock.Lock()
	defer extensions.lock.Unlock()

	extensions.remove(ext)
	return nil
}

func (extensions *extensionRegistry) remove(ext *runtimev1.ExtensionConfig) {
	for _, e := range extensions.items {
		if e.RegistrationName == ext.Name {
			delete(extensions.items, e.Name)
		}
	}
}

// List all the registered Runtime Extensions for a given Group/Hook..
func (extensions *extensionRegistry) List(gh catalog.GroupVersionHook) ([]*RuntimeExtensionRegistration, error) {
	if !extensions.ready {
		return nil, errors.New("invalid operation: Get cannot called on a registry not yet ready")
	}

	extensions.lock.RLock()
	defer extensions.lock.RUnlock()

	l := make([]*RuntimeExtensionRegistration, 0, 10)
	for _, r := range extensions.items {
		if r.GroupVersionHook.Group == gh.Group && r.GroupVersionHook.Hook == gh.Hook {
			l = append(l, r)
		}
	}
	return l, nil
}

// Get the Runtime Extensions with a given name.
func (extensions *extensionRegistry) Get(name string) (*RuntimeExtensionRegistration, error) {
	if !extensions.ready {
		return nil, errors.New("invalid operation: Get cannot called on a registry not yet ready")
	}

	extensions.lock.RLock()
	defer extensions.lock.RUnlock()

	r := extensions.items[name]
	return r, nil
}

func (extensions *extensionRegistry) add(ext *runtimev1.ExtensionConfig) error {
	extensions.remove(ext)

	for _, e := range ext.Status.Handlers {
		gv, err := schema.ParseGroupVersion(e.RequestHook.APIVersion)
		if err != nil { // TODO: consider if to aggregate errors
			return errors.Wrapf(err, "failed to parse GroupVersion from %q", e.RequestHook.APIVersion)
		}

		r := &RuntimeExtensionRegistration{
			RegistrationName: ext.Name,
			Name:             e.Name,
			GroupVersionHook: catalog.GroupVersionHook{
				Group:   gv.Group,
				Version: gv.Version,
				Hook:    e.RequestHook.Hook,
			},
			ClientConfig:   ext.Spec.ClientConfig,
			TimeoutSeconds: e.TimeoutSeconds,
			FailurePolicy:  e.FailurePolicy,
		}
		extensions.items[r.Name] = r
	}

	return nil
}
