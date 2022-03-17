package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	ctrl "sigs.k8s.io/controller-runtime"

	v1alpha32 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	catalogHTTP "sigs.k8s.io/cluster-api/internal/runtime/server"
)

// go run rte/test/rte-implementation-v1alpha3/main.go

var c = catalog.New()

func init() {
	v1alpha32.AddToCatalog(c)
}

func main() {
	ctx := ctrl.SetupSignalHandler()

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", "8083"))
	if err != nil {
		panic(err)
	}

	fmt.Println("Server started")

	operation1Handler, err := catalogHTTP.NewHandlerBuilder().
		WithCatalog(c).
		AddService(&v1alpha32.DiscoveryHook{}, doOperation1). // TODO: this is not strongly typed, but there are type checks when the service starts
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

func doOperation1(in *v1alpha32.DiscoveryHookRequest, out *v1alpha32.DiscoveryHookResponse) error {
	fmt.Println("DiscoveryHook/v1alpha3 called")
	out.Message = fmt.Sprintf("DiscoveryHook implementation version v1alpha3 - first: %d, second: %s", in.First, in.Second)
	return nil
}
