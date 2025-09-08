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

package extensionconfig

import (
	"context"
	"time"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
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
	ReadOnly       bool
	warmupTimeout  time.Duration
	warmupInterval time.Duration
}

// NeedLeaderElection satisfies the controller runtime LeaderElectionRunnable interface.
// This ensures we warm up the RuntimeSDK registry only after the controller became leader.
// Note: Only after the warmupRunnable is completed the registry becomes ready and thus
// all controllers using the runtime client or registry will wait until warmup is completed.
func (r *warmupRunnable) NeedLeaderElection() bool {
	return true
}

// Start attempts to warm up the registry. It will retry for 60 seconds before returning an error. An error on Start will
// cause the CAPI controller manager to fail.
// We are retrying for 60 seconds to mitigate failures when the CAPI controller manager and RuntimeExtensions
// are started at the same time. After 60 seconds we crash the entire controller to surface the
// issue to users in a timely fashion as it would block reconciliation of all Clusters.
func (r *warmupRunnable) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	if r.warmupInterval == 0 {
		r.warmupInterval = defaultWarmupInterval
	}
	if r.warmupTimeout == 0 {
		r.warmupTimeout = defaultWarmupTimeout
	}
	ctx, cancel := context.WithTimeoutCause(ctx, r.warmupTimeout, errors.New("warmup timeout expired"))
	defer cancel()

	var warmupErr error
	err := wait.PollUntilContextTimeout(ctx, r.warmupInterval, r.warmupTimeout, true, func(ctx context.Context) (done bool, err error) {
		if warmupErr = r.warmupRegistry(ctx); warmupErr != nil {
			log.Error(warmupErr, "ExtensionConfig registry warmup failed")
			return false, nil //nolint:nilerr // Intentionally not returning the error here
		}
		return true, nil
	})
	if err != nil {
		return errors.Wrapf(warmupErr, "ExtensionConfig registry warmup timed out after %s", r.warmupTimeout.String())
	}

	return nil
}

// warmupRegistry attempts to discover all existing ExtensionConfigs and patch their status with discovered Handlers.
// It warms up the registry by passing it the up-to-date list of ExtensionConfigs.
func (r *warmupRunnable) warmupRegistry(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	extensionConfigList := runtimev1.ExtensionConfigList{}
	if err := r.APIReader.List(ctx, &extensionConfigList); err != nil {
		return errors.Wrapf(err, "failed to list ExtensionConfigs")
	}

	var errs []error
	for i := range extensionConfigList.Items {
		extensionConfig := &extensionConfigList.Items[i]

		log := log.WithValues("ExtensionConfig", klog.KObj(extensionConfig))
		ctx := ctrl.LoggerInto(ctx, log)

		// In readOnly mode only validate instead of reconciling CA bundle and running discovery.
		if r.ReadOnly {
			if err := validateExtensionConfig(extensionConfig); err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to validate ExtensionConfig"))
			}
		} else {
			// extensionConfig is equal to original here, but we have to deepcopy so that if extensionConfig is changed original is not changed.
			original := extensionConfig.DeepCopy()
			extensionConfig, err := reconcileExtensionConfig(ctx, r.Client, r.RuntimeClient, original, extensionConfig)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to reconcile ExtensionConfig"))
				continue
			}
			extensionConfigList.Items[i] = *extensionConfig
		}
	}

	// If there was an error in discovery or patching return before committing to the registry.
	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}

	if err := r.RuntimeClient.WarmUp(&extensionConfigList); err != nil {
		return err
	}

	log.Info("The extension registry is warmed up")

	return nil
}
