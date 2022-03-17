package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"path/filepath"

	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	catalogHTTP "sigs.k8s.io/cluster-api/internal/runtime/server"
)

// go run rte/test/rte-implementation-v1alpha2/main.go

var c = catalog.New()
var certDir = flag.String("certDir", "", "path to directory containing tls.crt and tls.key")
var enableTLS = flag.Bool("enableTLS", false, "enables TLS webserver")

func init() {
	_ = runtimehooksv1.AddToCatalog(c)
}

func main() {
	flag.Parse()

	ctx := ctrl.SetupSignalHandler()

	var listener net.Listener
	var err error

	if *enableTLS {
		if *certDir == "" {
			panic(errors.New("expected certDir to find path to tls.crt and tls.key"))
		}

		// see https://github.com/kubernetes-sigs/controller-runtime/blob/master/pkg/webhook/server.go#L210
		certPath := filepath.Join(*certDir, "tls.crt")
		keyPath := filepath.Join(*certDir, "tls.key")

		certWatcher, err := certwatcher.New(certPath, keyPath)
		if err != nil {
			panic(err)
		}

		go func() {
			if err := certWatcher.Start(ctx); err != nil {
				fmt.Printf("error: certificate watcher error %v\n", err)
			}
		}()

		cfg := &tls.Config{ //nolint:gosec
			NextProtos:     []string{"h2"},
			GetCertificate: certWatcher.GetCertificate,
			MinVersion:     tls.VersionTLS10,
		}
		listener, err = tls.Listen("tcp", net.JoinHostPort("127.0.0.1", "8083"), cfg)
		if err != nil {
			panic(err)
		}
	} else {
		listener, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", "8082"))
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Server started")

	operation1Handler, err := catalogHTTP.NewHandlerBuilder().
		WithCatalog(c).
		AddDiscovery(runtimehooksv1.Discovery, doDiscovery). // TODO: this is not strongly typed, but there are type checks when the service starts
		AddExtension(runtimehooksv1.BeforeClusterUpgrade, "install-metrics-database", doInstallMetricsDatabase).
		// TODO: test with more services
		Build()
	if err != nil {
		panic(err)
	}

	srv := &http.Server{
		Handler: operation1Handler,
	}

	go func() {
		<-ctx.Done()

		// TODO: use a context with reasonable timeout
		if err := srv.Shutdown(context.Background()); err != nil {
			// Error from closing listeners, or context timeout
			panic("error shutting down the HTTP server")
		}
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
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

	return nil
}

func toPtr(f runtimehooksv1.FailurePolicy) *runtimehooksv1.FailurePolicy {
	return &f
}
