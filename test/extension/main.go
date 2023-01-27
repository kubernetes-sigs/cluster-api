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

// main is the main package for the test extension.
// The test extension serves two goals:
// - to provide a reference implementation of Runtime Extension
// - to implement the Runtime Extension used by Cluster API E2E tests.
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
	// catalog contains all information about RuntimeHooks.
	catalog = runtimecatalog.New()

	// scheme is a Kubernetes runtime scheme containing all the information about API types used by the test extension.
	// NOTE: it is not mandatory to use scheme in custom RuntimeExtension, but working with typed API objects makes code
	// easier to read and less error prone than using unstructured or working with raw json/yaml.
	scheme = runtime.NewScheme()

	// Flags.
	profilerAddress string
	webhookPort     int
	webhookCertDir  string
	logOptions      = logs.NewOptions()
)

func init() {
	// Adds to the catalog all the RuntimeHooks defined in cluster API.
	_ = runtimehooksv1.AddToCatalog(catalog)

	// Adds to the scheme all the API types we used by the test extension.
	_ = infrav1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
}

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	// Initialize logs flags using Kubernetes component-base machinery.
	// NOTE: it is not mandatory to use Kubernetes component-base machinery in custom RuntimeExtension, but it is
	// recommended because it helps in ensuring consistency across different components in the cluster.
	logsv1.AddFlags(logOptions, fs)

	// Add test-extension specific flags
	// NOTE: it is not mandatory to use the same flag names in all RuntimeExtension, but it is recommended when
	// addressing common concerns like profiler-address, webhook-port, webhook-cert-dir etc. because it helps in ensuring
	// consistency across different components in the cluster.

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.IntVar(&webhookPort, "webhook-port", 9443,
		"Webhook Server port")

	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir, only used when webhook-port is specified.")
}

func main() {
	// Creates a logger to be used during the main func using controller runtime utilities
	// NOTE: it is not mandatory to use controller runtime utilities in custom RuntimeExtension, but it is recommended
	// because it makes log from those components similar to log from controllers.
	setupLog := ctrl.Log.WithName("main")

	// Initialize and parse command line flags.
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	// Validates logs flags using Kubernetes component-base machinery and apply them
	// so klog will automatically use the right logger.
	// NOTE: klog is the log of choice of component-base machinery.
	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		setupLog.Error(err, "unable to start extension")
		os.Exit(1)
	}

	// Add the klog logger in the context.
	// NOTE: it is not mandatory to use contextual logging in custom RuntimeExtension, but it is recommended
	// because it allows to use a log stored in the context across the entire chain of calls (without
	// requiring an addition log parameter in all the functions.
	ctrl.SetLogger(klog.Background())

	// Initialize the golang profiler server, if required.
	if profilerAddress != "" {
		setupLog.Info(fmt.Sprintf("Profiler listening for requests at %s", profilerAddress))
		go func() {
			srv := http.Server{Addr: profilerAddress, ReadHeaderTimeout: 2 * time.Second}
			if err := srv.ListenAndServe(); err != nil {
				setupLog.Error(err, "problem running profiler server")
			}
		}()
	}

	// ****************************************************
	//  Create a http server for serving runtime extensions
	// ****************************************************

	webhookServer, err := server.New(server.Options{
		Catalog: catalog,
		Port:    webhookPort,
		CertDir: webhookCertDir,
	})
	if err != nil {
		setupLog.Error(err, "error creating webhook server")
		os.Exit(1)
	}

	// ****************************************************
	//  Add handlers for the Runtime hooks you are
	//  interested in, in the test-extension we are registering all
	//  of them for ensuring proper test coverage
	// ****************************************************

	// Topology Mutation Hooks (Runtime Patches)

	// Create the ExtensionHandlers for the Topology Mutation Hooks.
	// NOTE: it is not mandatory to group all the ExtensionHandlers using a struct, what is important
	// is to have HandlerFunc with the signature defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
	topologyMutationExtensionHandlers := topologymutation.NewExtensionHandlers(scheme)

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.GeneratePatches,
		Name:        "generate-patches",
		HandlerFunc: topologyMutationExtensionHandlers.GeneratePatches,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.ValidateTopology,
		Name:        "validate-topology",
		HandlerFunc: topologyMutationExtensionHandlers.ValidateTopology,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.DiscoverVariables,
		Name:        "discover-variables",
		HandlerFunc: topologyMutationExtensionHandlers.DiscoverVariables,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	// Lifecycle Hooks

	// Gets a client to access the Kubernetes cluster where this RuntimeExtension will be deployed to;
	// this is a requirement specific of the lifecycle hooks implementation for Cluster APIs E2E tests.
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "error getting config for the cluster")
		os.Exit(1)
	}

	client, err := client.New(restConfig, client.Options{})
	if err != nil {
		setupLog.Error(err, "error creating client to the cluster")
		os.Exit(1)
	}

	// Create the ExtensionHandlers for the lifecycle hooks
	// NOTE: it is not mandatory to group all the ExtensionHandlers using a struct, what is important
	// is to have HandlerFunc with the signature defined in sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1.
	lifecycleExtensionHandlers := lifecycle.NewExtensionHandlers(client)

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.BeforeClusterCreate,
		Name:        "before-cluster-create",
		HandlerFunc: lifecycleExtensionHandlers.DoBeforeClusterCreate,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.AfterControlPlaneInitialized,
		Name:        "after-control-plane-initialized",
		HandlerFunc: lifecycleExtensionHandlers.DoAfterControlPlaneInitialized,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.BeforeClusterUpgrade,
		Name:        "before-cluster-upgrade",
		HandlerFunc: lifecycleExtensionHandlers.DoBeforeClusterUpgrade,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.AfterControlPlaneUpgrade,
		Name:        "after-control-plane-upgrade",
		HandlerFunc: lifecycleExtensionHandlers.DoAfterControlPlaneUpgrade,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.AfterClusterUpgrade,
		Name:        "after-cluster-upgrade",
		HandlerFunc: lifecycleExtensionHandlers.DoAfterClusterUpgrade,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	if err := webhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.BeforeClusterDelete,
		Name:        "before-cluster-delete",
		HandlerFunc: lifecycleExtensionHandlers.DoBeforeClusterDelete,
	}); err != nil {
		setupLog.Error(err, "error adding handler")
		os.Exit(1)
	}

	// ****************************************************
	//  Start the https server
	// ****************************************************

	// Setup a context listening for SIGINT.
	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting RuntimeExtension", "version", version.Get().String())
	if err := webhookServer.Start(ctx); err != nil {
		setupLog.Error(err, "error running webhook server")
		os.Exit(1)
	}
}
