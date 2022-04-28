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
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha2"
	"sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
)

//  go run rte/test/rte-call-webhookregistration/main.go

var s = runtime.NewScheme()
var c = catalog.New()

func init() {
	// v1alpha4.AddToScheme(s)
	// v1beta1.AddToScheme(s)

	v1alpha2.AddToCatalog(c)
	v1alpha3.AddToCatalog(c)
}

type fakeWebHookRegistration struct {
	host    string
	version string
}

var registrations = []fakeWebHookRegistration{
	{
		host:    fmt.Sprintf("http://%s", net.JoinHostPort("127.0.0.1", "8083")),
		version: "v1alpha3",
	},
	{
		host:    fmt.Sprintf("http://%s", net.JoinHostPort("127.0.0.1", "8082")),
		version: "v1alpha2",
	},
}

func main() {
	c1 := &v1alpha4.Cluster{}
	c2 := &v1beta1.Cluster{}
	s.Convert(c1, c2, nil)

	// Doesn't work like this anymore, have to rewrite if we want to use this example again
	//ctx := context.Background()

	//for _, r := range registrations {
		//client.New := http.NewClientBuilder().
		//	WithCatalog(c).
		//	Host(r.host).
		//	Build()
		//
		//in := &v1alpha3.DiscoveryHookRequest{First: 1, Second: "Hello CAPI runtime extensions!"}
		//out := &v1alpha3.DiscoveryHookResponse{}
		//if err := client.ServiceOld(v1alpha3.Discovery, http.SpecVersion(r.version)).Invoke(ctx, in, out); err != nil {
		//	panic(err)
		//}

	//	fmt.Printf("Result: %v\n", out.Message)
	//}
}
