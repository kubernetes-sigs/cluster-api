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

// main is the main package for the Test Extension.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/runtime/server"
	"sigs.k8s.io/cluster-api/test/extension/handlers/lifecycle"
	"sigs.k8s.io/cluster-api/test/extension/handlers/topologymutation"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/version"
)

var (
	catalog = runtimecatalog.New()
	scheme  = runtime.NewScheme()

	setupLog = ctrl.Log.WithName("setup")

	// Flags.
	profilerAddress string
	webhookPort     int
	webhookCertDir  string
	logOptions      = logs.NewOptions()
)

func init() {
	_ = infrav1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)

	// Register the RuntimeHook types into the catalog.
	_ = runtimehooksv1.AddToCatalog(catalog)
}

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	logs.AddFlags(fs, logs.SkipLoggingConfigurationFlags())
	logsv1.AddFlags(logOptions, fs)

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.IntVar(&webhookPort, "webhook-port", 9443,
		"Webhook Server port")

	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir, only used when webhook-port is specified.")
}

func main() {
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to start extension")
		os.Exit(1)
	}

	// klog.Background will automatically use the right logger.
	ctrl.SetLogger(klog.Background())

	if profilerAddress != "" {
		setupLog.Info(fmt.Sprintf("Profiler listening for requests at %s", profilerAddress))
		go func() {
			srv := http.Server{Addr: profilerAddress, ReadHeaderTimeout: 2 * time.Second}
			if err := srv.ListenAndServe(); err != nil {
				setupLog.Error(err, "problem running profiler server")
			}
		}()
	}

	ctx := ctrl.SetupSignalHandler()

	webhookServer, err := server.NewServer(server.Options{
		Catalog: catalog,
		Port:    webhookPort,
		CertDir: webhookCertDir,
	})
	if err != nil {
		setupLog.Error(err, "error creating webhook server")
		os.Exit(1)
	}

	// Add the scheme with registered types to the topologyMutationHandler
	topologyMutationHandler := topologymutation.NewHandler(scheme)

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.GeneratePatches,
		Name:           "generate-patches",
		HandlerFunc:    topologyMutationHandler.GeneratePatches,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.ValidateTopology,
		Name:           "validate-topology",
		HandlerFunc:    topologyMutationHandler.ValidateTopology,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	restConfig, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "error getting config for the cluster")
		os.Exit(1)
	}

	c, err := client.New(restConfig, client.Options{})
	if err != nil {
		setupLog.Error(err, "error creating client to the cluster")
		os.Exit(1)
	}
	lifecycleHandler := lifecycle.Handler{Client: c}

	// Lifecycle Hooks
	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.BeforeClusterCreate,
		Name:           "before-cluster-create",
		HandlerFunc:    lifecycleHandler.DoBeforeClusterCreate,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.AfterControlPlaneInitialized,
		Name:           "after-control-plane-initialized",
		HandlerFunc:    lifecycleHandler.DoAfterControlPlaneInitialized,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.BeforeClusterUpgrade,
		Name:           "before-cluster-upgrade",
		HandlerFunc:    lifecycleHandler.DoBeforeClusterUpgrade,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.AfterControlPlaneUpgrade,
		Name:           "after-control-plane-upgrade",
		HandlerFunc:    lifecycleHandler.DoAfterControlPlaneUpgrade,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.AfterClusterUpgrade,
		Name:           "after-cluster-upgrade",
		HandlerFunc:    lifecycleHandler.DoAfterClusterUpgrade,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:           runtimehooksv1.BeforeClusterDelete,
		Name:           "before-cluster-delete",
		HandlerFunc:    lifecycleHandler.DoBeforeClusterDelete,
		TimeoutSeconds: pointer.Int32(5),
		FailurePolicy:  toPtr(runtimehooksv1.FailurePolicyFail),
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	setupLog.Info("starting RuntimeExtension", "version", version.Get().String())
	if err := webhookServer.Start(ctx); err != nil {
		setupLog.Error(err, "error running webhook server")
		os.Exit(1)
	}
}

func toPtr(f runtimehooksv1.FailurePolicy) *runtimehooksv1.FailurePolicy {
	return &f
}
