/*
Copyright 2023 The Kubernetes Authors.

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

package ssa

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Option is the interface for configuration that modifies Options for a patch request.
type Option interface {
	// ApplyToOptions applies this configuration to the given Options.
	ApplyToOptions(*Options)
}

// WithCachingProxy enables caching for the patch request.
// The original and modified object will be used to generate an
// identifier for the request.
// The cache will be used to cache the result of the request.
type WithCachingProxy struct {
	Cache    Cache
	Original client.Object
}

// ApplyToOptions applies WithCachingProxy to the given Options.
func (w WithCachingProxy) ApplyToOptions(in *Options) {
	in.WithCachingProxy = true
	in.Cache = w.Cache
	in.Original = w.Original
}

// Options contains the options for the Patch func.
type Options struct {
	WithCachingProxy bool
	Cache            Cache
	Original         client.Object
}

// Patch executes an SSA patch.
// If WithCachingProxy is set and the request didn't change the object
// we will cache this result, so subsequent calls don't have to run SSA again.
func Patch(ctx context.Context, c client.Client, fieldManager string, modified client.Object, opts ...Option) (client.Object, error) {
	// Calculate the options.
	options := &Options{}
	for _, opt := range opts {
		opt.ApplyToOptions(options)
	}

	var requestIdentifier string
	var err error
	if options.WithCachingProxy {
		// Check if the request is cached.
		requestIdentifier, err = ComputeRequestIdentifier(c.Scheme(), options.Original, modified)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to apply object")
		}
		if options.Cache.Has(requestIdentifier) {
			// If the request is cached return the original object.
			return options.Original, nil
		}
	}

	gvk, err := apiutil.GVKForObject(modified, c.Scheme())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to apply object: failed to get GroupVersionKind of modified object %s", klog.KObj(modified))
	}

	patchOptions := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(fieldManager),
	}
	if err := c.Patch(ctx, modified, client.Apply, patchOptions...); err != nil {
		return nil, errors.Wrapf(err, "failed to apply %s %s", gvk.Kind, klog.KObj(modified))
	}

	if options.WithCachingProxy {
		// If the SSA call did not update the object, add the request to the cache.
		if options.Original.GetResourceVersion() == modified.GetResourceVersion() {
			options.Cache.Add(requestIdentifier)
		}
	}

	return modified, nil
}
