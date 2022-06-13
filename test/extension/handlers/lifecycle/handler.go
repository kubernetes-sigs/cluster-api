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
package lifecycle

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

// Handler is the handler for the lifecycle hooks.
type Handler struct {
	Client client.Client
}

// DoBeforeClusterCreate implements the BeforeClusterCreate hook.
func (h *Handler) DoBeforeClusterCreate(ctx context.Context, request *runtimehooksv1.BeforeClusterCreateRequest, response *runtimehooksv1.BeforeClusterCreateResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BeforeClusterCreate is called")
	cluster := request.Cluster
	responseInfo, err := h.responseFromConfigMap(cluster.Name, cluster.Namespace, runtimehooksv1.BeforeClusterCreate)
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.Status = runtimehooksv1.ResponseStatus(responseInfo["Status"])
	retryAfterSeconds, err := strconv.Atoi(responseInfo["RetryAfterSeconds"]) //nolint:gosec
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.RetryAfterSeconds = int32(retryAfterSeconds)
	log.Info(fmt.Sprintf("BeforeClusterCreate responding RetryAfterSeconds: %v\n", response.RetryAfterSeconds))
}

// DoBeforeClusterUpgrade implements the BeforeClusterUpgrade hook.
func (h *Handler) DoBeforeClusterUpgrade(ctx context.Context, request *runtimehooksv1.BeforeClusterUpgradeRequest, response *runtimehooksv1.BeforeClusterUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BeforeClusterUpgrade is called")
	cluster := request.Cluster
	responseInfo, err := h.responseFromConfigMap(cluster.Name, cluster.Namespace, runtimehooksv1.BeforeClusterUpgrade)
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.Status = runtimehooksv1.ResponseStatus(responseInfo["Status"])
	retryAfterSeconds, err := strconv.Atoi(responseInfo["RetryAfterSeconds"]) //nolint:gosec
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.RetryAfterSeconds = int32(retryAfterSeconds)
	log.Info(fmt.Sprintf("BeforeClusterUpgrade responding RetryAfterSeconds: %v\n", response.RetryAfterSeconds))
}

// DoAfterControlPlaneInitialized implements the AfterControlPlaneInitialized hook.
func (h *Handler) DoAfterControlPlaneInitialized(ctx context.Context, request *runtimehooksv1.AfterControlPlaneInitializedRequest, response *runtimehooksv1.AfterControlPlaneInitializedResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("AfterControlPlaneInitialized is called")
	cluster := request.Cluster
	responseInfo, err := h.responseFromConfigMap(cluster.Name, cluster.Namespace, runtimehooksv1.AfterControlPlaneInitialized)
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.Status = runtimehooksv1.ResponseStatus(responseInfo["Status"])
}

// DoAfterControlPlaneUpgrade implements the AfterControlPlaneUpgrade hook.
func (h *Handler) DoAfterControlPlaneUpgrade(ctx context.Context, request *runtimehooksv1.AfterControlPlaneUpgradeRequest, response *runtimehooksv1.AfterControlPlaneUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("AfterControlPlaneUpgrade is called")
	cluster := request.Cluster
	responseInfo, err := h.responseFromConfigMap(cluster.Name, cluster.Namespace, runtimehooksv1.AfterControlPlaneUpgrade)
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.Status = runtimehooksv1.ResponseStatus(responseInfo["Status"])
	retryAfterSeconds, err := strconv.Atoi(responseInfo["RetryAfterSeconds"]) //nolint:gosec
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.RetryAfterSeconds = int32(retryAfterSeconds)
	log.Info(fmt.Sprintf("AfterControlPlaneUpgrade responding RetryAfterSeconds: %v\n", response.RetryAfterSeconds))
}

// DoAfterClusterUpgrade implements the AfterClusterUpgrade hook.
func (h *Handler) DoAfterClusterUpgrade(ctx context.Context, request *runtimehooksv1.AfterClusterUpgradeRequest, response *runtimehooksv1.AfterClusterUpgradeResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("AfterClusterUpgrade is called")
	cluster := request.Cluster
	responseInfo, err := h.responseFromConfigMap(cluster.Name, cluster.Namespace, runtimehooksv1.AfterClusterUpgrade)
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.Status = runtimehooksv1.ResponseStatus(responseInfo["Status"])
}

// DoBeforeClusterDelete implements the BeforeClusterDelete hook.
func (h *Handler) DoBeforeClusterDelete(ctx context.Context, request *runtimehooksv1.BeforeClusterDeleteRequest, response *runtimehooksv1.BeforeClusterDeleteResponse) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("BeforeClusterDelete is called")
	cluster := request.Cluster
	responseInfo, err := h.responseFromConfigMap(cluster.Name, cluster.Namespace, runtimehooksv1.BeforeClusterDelete)
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.Status = runtimehooksv1.ResponseStatus(responseInfo["Status"])
	retryAfterSeconds, err := strconv.Atoi(responseInfo["RetryAfterSeconds"]) //nolint:gosec
	if err != nil {
		response.Status = runtimehooksv1.ResponseStatusFailure
		response.Message = err.Error()
		return
	}
	response.RetryAfterSeconds = int32(retryAfterSeconds)
	log.Info(fmt.Sprintf("BeforeClusterDelete responding RetryAfterSeconds: %v\n", response.RetryAfterSeconds))
}

func (h *Handler) responseFromConfigMap(name, namespace string, hook runtimecatalog.Hook) (map[string]string, error) {
	hookName := runtimecatalog.HookName(hook)
	configMap := &corev1.ConfigMap{}
	configMapName := name + "-hookresponses"
	if err := h.Client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: configMapName}, configMap); err != nil {
		return nil, errors.Wrapf(err, "failed to read the ConfigMap %s/%s", namespace, configMapName)
	}
	m := map[string]string{}
	if err := yaml.Unmarshal([]byte(configMap.Data[hookName]), m); err != nil {
		return nil, errors.Wrapf(err, "failed to read %q response information from ConfigMap", hook)
	}
	return m, nil
}
