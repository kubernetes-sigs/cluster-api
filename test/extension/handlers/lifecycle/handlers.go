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

// Package lifecycle contains the handlers for the lifecycle hooks.
//
// The implementation of the handlers is specifically designed for Cluster API E2E tests use cases.
// When implementing custom RuntimeExtension, it is only required to expose HandlerFunc with the
// signature defined in sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1.
package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

const (
	extensionConfigNameKey = "extensionConfigName"
)

// ExtensionHandlers provides a common struct shared across the lifecycle hook handlers; this is convenient
// because in Cluster API's E2E tests all of them are using a controller runtime client and the same set of func
// to work with the config map where preloaded answers for lifecycle hooks are stored.
// NOTE: it is not mandatory to use a ExtensionHandlers in custom RuntimeExtension, what is important
// is to expose HandlerFunc with the signature defined in sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1.
type ExtensionHandlers struct {
	client client.Client
}

// NewExtensionHandlers returns a ExtensionHandlers for the lifecycle hooks handlers.
func NewExtensionHandlers(client client.Client) *ExtensionHandlers {
	return &ExtensionHandlers{
		client: client,
	}
}

// DoBeforeClusterCreate implements the HandlerFunc for the BeforeClusterCreate hook.
// The hook answers with the response stored in a well know config map, thus allowing E2E tests to
// control the hook behaviour during a test.
// NOTE: custom RuntimeExtension, must implement the body of this func according to the specific use case.
func (m *ExtensionHandlers) DoBeforeClusterCreate(ctx context.Context, request *runtimehooksv1.BeforeClusterCreateRequest, response *runtimehooksv1.BeforeClusterCreateResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(&request.Cluster))
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("BeforeClusterCreate is called")

	settings := request.GetSettings()

	if err := m.readResponseFromConfigMap(ctx, &request.Cluster, runtimehooksv1.BeforeClusterCreate, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	if err := m.recordCallInConfigMap(ctx, &request.Cluster, runtimehooksv1.BeforeClusterCreate, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
	}
}

// DoBeforeClusterUpgrade implements the HandlerFunc for the BeforeClusterUpgrade hook.
// The hook answers with the response stored in a well know config map, thus allowing E2E tests to
// control the hook behaviour during a test.
// NOTE: custom RuntimeExtension, must implement the body of this func according to the specific use case.
func (m *ExtensionHandlers) DoBeforeClusterUpgrade(ctx context.Context, request *runtimehooksv1.BeforeClusterUpgradeRequest, response *runtimehooksv1.BeforeClusterUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(&request.Cluster))
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("BeforeClusterUpgrade is called")

	settings := request.GetSettings()

	if err := m.readResponseFromConfigMap(ctx, &request.Cluster, runtimehooksv1.BeforeClusterUpgrade, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}

	if err := m.recordCallInConfigMap(ctx, &request.Cluster, runtimehooksv1.BeforeClusterUpgrade, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
	}
}

// DoAfterControlPlaneInitialized implements the HandlerFunc for the AfterControlPlaneInitialized hook.
// The hook answers with the response stored in a well know config map, thus allowing E2E tests to
// control the hook behaviour during a test.
// NOTE: custom RuntimeExtension, must implement the body of this func according to the specific use case.
func (m *ExtensionHandlers) DoAfterControlPlaneInitialized(ctx context.Context, request *runtimehooksv1.AfterControlPlaneInitializedRequest, response *runtimehooksv1.AfterControlPlaneInitializedResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(&request.Cluster))
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("AfterControlPlaneInitialized is called")

	settings := request.GetSettings()

	if err := m.readResponseFromConfigMap(ctx, &request.Cluster, runtimehooksv1.AfterControlPlaneInitialized, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}

	if err := m.recordCallInConfigMap(ctx, &request.Cluster, runtimehooksv1.AfterControlPlaneInitialized, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
	}
}

// DoAfterControlPlaneUpgrade implements the HandlerFunc for the AfterControlPlaneUpgrade hook.
// The hook answers with the response stored in a well know config map, thus allowing E2E tests to
// control the hook behaviour during a test.
// NOTE: custom RuntimeExtension, must implement the body of this func according to the specific use case.
func (m *ExtensionHandlers) DoAfterControlPlaneUpgrade(ctx context.Context, request *runtimehooksv1.AfterControlPlaneUpgradeRequest, response *runtimehooksv1.AfterControlPlaneUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(&request.Cluster))
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("AfterControlPlaneUpgrade is called")

	attributes := []string{request.KubernetesVersion}
	settings := request.GetSettings()

	if err := m.readResponseFromConfigMap(ctx, &request.Cluster, runtimehooksv1.AfterControlPlaneUpgrade, attributes, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}

	if err := m.recordCallInConfigMap(ctx, &request.Cluster, runtimehooksv1.AfterControlPlaneUpgrade, attributes, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
	}
}

// DoAfterClusterUpgrade implements the HandlerFunc for the AfterClusterUpgrade hook.
// The hook answers with the response stored in a well know config map, thus allowing E2E tests to
// control the hook behaviour during a test.
// NOTE: custom RuntimeExtension, must implement the body of this func according to the specific use case.
func (m *ExtensionHandlers) DoAfterClusterUpgrade(ctx context.Context, request *runtimehooksv1.AfterClusterUpgradeRequest, response *runtimehooksv1.AfterClusterUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(&request.Cluster))
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("AfterClusterUpgrade is called")

	settings := request.GetSettings()

	if err := m.readResponseFromConfigMap(ctx, &request.Cluster, runtimehooksv1.AfterClusterUpgrade, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}

	if err := m.recordCallInConfigMap(ctx, &request.Cluster, runtimehooksv1.AfterClusterUpgrade, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
	}
}

// DoBeforeClusterDelete implements the HandlerFunc for the BeforeClusterDelete hook.
// The hook answers with the response stored in a well know config map, thus allowing E2E tests to
// control the hook behaviour during a test.
// NOTE: custom RuntimeExtension, must implement the body of this func according to the specific use case.
func (m *ExtensionHandlers) DoBeforeClusterDelete(ctx context.Context, request *runtimehooksv1.BeforeClusterDeleteRequest, response *runtimehooksv1.BeforeClusterDeleteResponse) {
	log := ctrl.LoggerFrom(ctx).WithValues("Cluster", klog.KObj(&request.Cluster))
	ctx = ctrl.LoggerInto(ctx, log)
	log.Info("BeforeClusterDelete is called")

	settings := request.GetSettings()

	if err := m.readResponseFromConfigMap(ctx, &request.Cluster, runtimehooksv1.BeforeClusterDelete, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	if err := m.recordCallInConfigMap(ctx, &request.Cluster, runtimehooksv1.BeforeClusterDelete, nil, settings, response); err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
	}

	// TODO: consider if to cleanup the ConfigMap after gating Cluster deletion.
}

func (m *ExtensionHandlers) readResponseFromConfigMap(ctx context.Context, cluster *clusterv1beta1.Cluster, hook runtimecatalog.Hook, attributes []string, settings map[string]string, response runtimehooksv1.ResponseObject) error {
	hookName := computeHookName(hook, attributes)
	configMap := &corev1.ConfigMap{}
	if _, ok := settings[extensionConfigNameKey]; !ok {
		return errors.New(extensionConfigNameKey + " mest be set in settings")
	}
	configMapName := configMapName(cluster.Name, settings[extensionConfigNameKey])
	log := ctrl.LoggerFrom(ctx)
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: configMapName}, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			// A ConfigMap of responses does not exist. Create one now.
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: cluster.Namespace,
				},
			}
			if err := m.client.Create(ctx, configMap); err != nil {
				return errors.Wrapf(err, "failed to create the ConfigMap %s", klog.KRef(cluster.Namespace, configMapName))
			}
			log.Info(fmt.Sprintf("Created ConfigMap %s", configMapName))
		} else {
			return errors.Wrapf(err, "failed to read the ConfigMap %s", klog.KRef(cluster.Namespace, configMapName))
		}
	}
	data, ok := configMap.Data[hookName+"-preloadedResponse"]
	if !ok {
		// If there is no preloadedResponse for the given hook, create one with blocking responses if "defaultAllHandlersToBlocking" is set to "true" in the settings.
		// This allows the test-extension to have non-blocking behavior by default but can be switched to blocking as needed, example: during E2E testing.
		retryAfterSeconds := 0
		if settings["defaultAllHandlersToBlocking"] == "true" {
			retryAfterSeconds = 5
		}

		switch runtimecatalog.HookName(hook) {
		// Blocking hooks are set to return RetryAfterSeconds initially. These will be changed during the test.
		case "BeforeClusterCreate":
			data = fmt.Sprintf(`{"Status": "Success", "RetryAfterSeconds": %d}`, retryAfterSeconds)
		case "BeforeClusterUpgrade":
			data = fmt.Sprintf(`{"Status": "Success", "RetryAfterSeconds": %d}`, retryAfterSeconds)
		case "AfterControlPlaneUpgrade":
			data = fmt.Sprintf(`{"Status": "Success", "RetryAfterSeconds": %d}`, retryAfterSeconds)
		case "BeforeClusterDelete":
			data = fmt.Sprintf(`{"Status": "Success", "RetryAfterSeconds": %d}`, retryAfterSeconds)

		// Non-blocking hooks are set to Status:Success.
		case "AfterControlPlaneInitialized":
			data = `{"Status": "Success"}`
		case "AfterClusterUpgrade":
			data = `{"Status": "Success"}`
		}
	}

	if err := yaml.Unmarshal([]byte(data), response); err != nil {
		return errors.Wrapf(err, "failed to read %q response information from ConfigMap", hook)
	}
	if r, ok := response.(runtimehooksv1.RetryResponseObject); ok {
		log := ctrl.LoggerFrom(ctx)
		log.Info(fmt.Sprintf("%s response is %s. retry: %v", hookName, r.GetStatus(), r.GetRetryAfterSeconds()))
	}
	return nil
}

func (m *ExtensionHandlers) recordCallInConfigMap(ctx context.Context, cluster *clusterv1beta1.Cluster, hook runtimecatalog.Hook, attributes []string, settings map[string]string, response runtimehooksv1.ResponseObject) error {
	hookName := computeHookName(hook, attributes)
	configMap := &corev1.ConfigMap{}
	if _, ok := settings[extensionConfigNameKey]; !ok {
		return errors.New(extensionConfigNameKey + " must be set in runtime extension settings")
	}
	configMapName := configMapName(cluster.Name, settings[extensionConfigNameKey])
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: configMapName}, configMap); err != nil {
		return errors.Wrapf(err, "failed to read the ConfigMap %s", klog.KRef(cluster.Namespace, configMapName))
	}
	var patch client.Patch
	if r, ok := response.(runtimehooksv1.RetryResponseObject); ok {
		patch = client.RawPatch(types.MergePatchType,
			[]byte(fmt.Sprintf(`{"data":{"%s-actualResponseStatus": "Status: %s, RetryAfterSeconds: %v"}}`, hookName, r.GetStatus(), r.GetRetryAfterSeconds())))
	} else {
		// Patch the actualResponseStatus with the returned value
		patch = client.RawPatch(types.MergePatchType,
			[]byte(fmt.Sprintf(`{"data":{"%s-actualResponseStatus":"%s"}}`, hookName, response.GetStatus()))) //nolint:gocritic
	}
	if err := m.client.Patch(ctx, configMap, patch); err != nil {
		return errors.Wrapf(err, "failed to update the ConfigMap %s", klog.KRef(cluster.Namespace, configMapName))
	}
	return nil
}

func configMapName(clusterName, extensionConfigName string) string {
	return fmt.Sprintf("%s-%s-test-extension-hookresponses", clusterName, extensionConfigName)
}

func computeHookName(hook runtimecatalog.Hook, attributes []string) string {
	// Note: + is not a valid character for ConfigMap keys (only alphanumeric characters, '-', '_' or '.')
	return strings.ReplaceAll(strings.Join(append([]string{runtimecatalog.HookName(hook)}, attributes...), "-"), "+", "_")
}
