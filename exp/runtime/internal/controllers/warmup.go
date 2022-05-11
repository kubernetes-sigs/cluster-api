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

package controllers

import (
	"context"
	"time"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
)

const (
	defaultWarmupTimeout  = 60 * time.Second
	defaultWarmupInterval = 2 * time.Second
)

var _ manager.LeaderElectionRunnable = &warmupRunnable{}

// warmupRunnable is a controller runtime LeaderElectionRunnable. It warms up the registry on controller start.
type warmupRunnable struct {
	Client         client.Client
	APIReader      client.Reader
	RuntimeClient  runtimeclient.Client
	warmupTimeout  time.Duration
	warmupInterval time.Duration
}

// NeedLeaderElection satisfies the controller runtime LeaderElectionRunnable interface.
// This helps ensure we warm up the RuntimeSDK registry before controllers begin reconciling.
func (r *warmupRunnable) NeedLeaderElection() bool {
	return true
}

// Start attempts to warm up the registry. It will retry for 60 seconds before returning an error. An error on Start will
// cause the CAPI controller manager to fail.
// We are retrying for 60 seconds to mitigate failures when the CAPI controller manager and RuntimeExtensions
// are started at the same time. After 60 seconds we crash the entire controller to surface the
// issue which block reconciliation of all Clusters to users in a timely fashion.
func (r *warmupRunnable) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	if r.warmupInterval == 0 {
		r.warmupInterval = defaultWarmupInterval
	}
	if r.warmupTimeout == 0 {
		r.warmupTimeout = defaultWarmupTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, r.warmupTimeout)
	defer cancel()

	err := wait.PollImmediateWithContext(ctx, r.warmupInterval, r.warmupTimeout, func(ctx context.Context) (done bool, err error) {
		if err = warmupRegistry(ctx, r.Client, r.APIReader, r.RuntimeClient); err != nil {
			log.Error(err, "ExtensionConfig registry warmup failed")
			return false, nil
		}
		return true, nil
	})

	if err != nil {
		return errors.Wrapf(err, "ExtensionConfig registry warmup timed out after  %s", r.warmupTimeout.String())
	}

	return nil
}

// warmupRegistry attempts to discover all existing ExtensionConfigs and patch their Status with discovered Handlers.
// It warms up the registry by passing it the up-to-date list of ExtensionConfigs.
func warmupRegistry(ctx context.Context, client client.Client, reader client.Reader, runtimeClient runtimeclient.Client) error {
	var errs []error

	extensionConfigList := runtimev1.ExtensionConfigList{}
	if err := reader.List(ctx, &extensionConfigList); err != nil {
		return err
	}

	for i := range extensionConfigList.Items {
		extensionConfig := &extensionConfigList.Items[i]
		original := extensionConfig.DeepCopy()

		extensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
		if err != nil {
			errs = append(errs, err)
		}

		// Patch the ExtensionConfig with the updated condition and Handlers if discovered.
		if err = patchExtensionConfig(ctx, client, original, extensionConfig); err != nil {
			errs = append(errs, err)
		}
		extensionConfigList.Items[i] = *extensionConfig
	}

	// If there was some error in discovery or patching return before committing to the Registry.
	if len(errs) != 0 {
		return kerrors.NewAggregate(errs)
	}

	if err := runtimeClient.WarmUp(&extensionConfigList); err != nil {
		return err
	}

	return nil
}
