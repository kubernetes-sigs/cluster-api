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
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/runtime/controllers"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	rtclient "sigs.k8s.io/cluster-api/internal/runtime/client"
)

// go run rte/test/rte-call-simple/main.go

var c = catalog.New()

func init() {
	_ = v1alpha3.AddToCatalog(c)
}

func main() {
	ctx := context.Background()

	// Note: this example is not functional anymore as it requires an entire manager to run the extension controller
	// which we currently don't setup correctly here.
	var mgr ctrl.Manager
	runtimeClient := rtclient.New(rtclient.Options{
		Catalog: c,
	})

	if err := (&controllers.ExtensionConfigReconciler{
		Client:        mgr.GetClient(),
		RuntimeClient: runtimeClient,
	}).SetupWithManager(ctx, mgr, controller.Options{}); err != nil {
		os.Exit(1)
	}

	ext := &runtimev1.ExtensionConfig{}

	runtimeExtensions, err := runtimeClient.Extension(ext).Discover(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(runtimeExtensions)

	in := &v1alpha3.DiscoveryHookRequest{}
	out := &v1alpha3.DiscoveryHookResponse{}

	runtimeClient.Hook(v1alpha3.Discovery).Call(ctx, "http-proxy.patch", in, out)

	runtimeClient.Hook(v1alpha3.Discovery).CallAll(ctx, in, out)

	//runtimeClient = http.NewClientBuilder().
	//	WithCatalog(c).
	//	Host(fmt.Sprintf("http://%s", net.JoinHostPort("127.0.0.1", "8083"))).
	//	Build()
	//
	//
	//if err := runtimeClient.ServiceOld(hook, rtclient.SpecVersion("v1alpha3")).Invoke(ctx, in, out); err != nil {
	//	panic(err)
	//}

	fmt.Printf("Result: %v\n", out.Message)
}
