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
	"context"
	"fmt"
	"net"
	"net/http"

	ctrl "sigs.k8s.io/controller-runtime"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha2"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	catalogHTTP "sigs.k8s.io/cluster-api/internal/runtime/server"
)

// go run rte/test/rte-implementation-v1alpha3/main.go

var c = catalog.New()

func init() {
	v1alpha3.AddToCatalog(c)
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
		AddDiscovery(v1alpha2.Discovery, doOperation1). // TODO: this is not strongly typed, but there are type checks when the service starts
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

func doOperation1(in *runtimehooksv1.DiscoveryHookRequest, out *runtimehooksv1.DiscoveryHookResponse) error {
	fmt.Println("Discovery/v1alpha2 called")
	out.Message = fmt.Sprintf("Discovery implementation version v1alpha2")
	return nil
}
