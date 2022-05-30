/*
Copyright 2015 The Kubernetes Authors.

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

/*
-The original file is located at [1].
-[1]: https://github.com/kubernetes/kube-state-metrics/blob/e859b280fcc2/main.go
-
-The original source was adjusted to:
-- support a store.Builder which uses a controller-runtime client instead of client-go.
-- remove sharding functionality to reduce complexity for the initial implementation.
-- use a custom options package.
-- rename the application.
*/

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/prometheus/common/version"
	"k8s.io/klog/v2"
	"k8s.io/kube-state-metrics/v2/pkg/app"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	"sigs.k8s.io/cluster-api/exp/state-metrics/pkg/store"
)

func main() {
	options.DefaultResources = store.DefaultResources
	opts := options.NewOptions()
	opts.AddFlags()

	if err := opts.Parse(); err != nil {
		klog.Fatalf("Parsing flag definitions error: %v", err)
	}

	// reset back to empty, we get the defaults duplicated at `Active resources` otherwise.
	if opts.Resources.String() == "" {
		options.DefaultResources = options.ResourceSet{}
	}

	if opts.Version {
		fmt.Printf("%s\n", version.Print("kube-state-metrics"))
		os.Exit(0)
	}

	if opts.Help {
		opts.Usage()
		os.Exit(0)
	}

	ctx := context.Background()
	if err := app.RunKubeStateMetrics(ctx, opts, store.Factories()...); err != nil {
		klog.Fatalf("Failed to run kube-state-metrics: %v", err)
	}
}
