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

package main

import (
	"flag"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	catalogHTTP "sigs.k8s.io/cluster-api/internal/runtime/server"
)

// go run rte/test/rte-implementation-v1alpha2/main.go

var c = catalog.New()
var certDir = flag.String("certDir", "/tmp/rte-implementation-secure/", "path to directory containing tls.crt and tls.key")

func init() {
	_ = runtimehooksv1.AddToCatalog(c)
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	srv := webhook.Server{
		Host:          "127.0.0.1",
		Port:          8083,
		CertDir:       *certDir,
		CertName:      "tls.crt",
		KeyName:       "tls.key",
		WebhookMux:    http.NewServeMux(),
		TLSMinVersion: "1.2",
	}

	operation1Handler, err := catalogHTTP.NewHandlerBuilder().
		WithCatalog(c).
		AddDiscovery(runtimehooksv1.Discovery, doDiscovery). // TODO: this is not strongly typed, but there are type checks when the service starts
		AddExtension(runtimehooksv1.BeforeClusterUpgrade, "install-metrics-database", doInstallMetricsDatabase).
		// TODO: test with more services
		Build()
	if err != nil {
		panic(err)
	}

	srv.WebhookMux.Handle("/", operation1Handler)

	if err := srv.StartStandalone(ctx, nil); err != nil {
		panic(err)
	}
}

// TODO: consider registering extensions with all required data and then auto-generating the discovery func based on that.
// If we want folks to write it manually, make it nicer to do.
func doDiscovery(request *runtimehooksv1.DiscoveryHookRequest, response *runtimehooksv1.DiscoveryHookResponse) error {
	fmt.Println("Discovery/v1alpha1 called")

	response.Status = runtimehooksv1.ResponseStatusSuccess
	response.Extensions = append(response.Extensions, runtimehooksv1.RuntimeExtension{
		Name: "install-metrics-database",
		Hook: runtimehooksv1.Hook{
			APIVersion: runtimehooksv1.GroupVersion.String(),
			Name:       "BeforeClusterUpgrade",
		},
		TimeoutSeconds: pointer.Int32(10),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	})

	return nil
}

func doInstallMetricsDatabase(request *runtimehooksv1.BeforeClusterUpgradeRequest, response *runtimehooksv1.BeforeClusterUpgradeResponse) error {
	fmt.Println("BeforeClusterUpgrade/v1alpha1 called", "cluster", klog.KObj(&request.Cluster))

	response.Status = runtimehooksv1.ResponseStatusSuccess
	response.RetryAfterSeconds = 10

	return nil
}

func toPtr(f runtimehooksv1.FailurePolicy) *runtimehooksv1.FailurePolicy {
	return &f
}
