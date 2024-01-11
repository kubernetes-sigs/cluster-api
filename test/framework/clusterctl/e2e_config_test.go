/*
Copyright 2021 The Kubernetes Authors.

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

package clusterctl

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/internal/goproxy"
	goproxytest "sigs.k8s.io/cluster-api/internal/goproxy/test"
)

func Test_resolveReleaseMarker(t *testing.T) {
	scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
	clientGoproxy := goproxy.NewClient(scheme, host)
	defer teardownGoproxy()

	// setup an handler with fake releases
	muxGoproxy.HandleFunc("/github.com/o/r1/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v1.2.0\n")
		fmt.Fprint(w, "v1.2.1-rc.0\n")
		fmt.Fprint(w, "v1.3.0-rc.0\n")
		fmt.Fprint(w, "v1.3.0-rc.1\n")
	})
	tests := []struct {
		name          string
		releaseMarker string
		want          string
		wantErr       bool
	}{
		{
			name:          "Invalid url",
			releaseMarker: "github.com/o/doesntexist",
			want:          "",
			wantErr:       true,
		},
		{
			name:          "Get stable release",
			releaseMarker: "go://github.com/o/r1@v1.2",
			want:          "1.2.0",
			wantErr:       false,
		},
		{
			name:          "Get latest release",
			releaseMarker: "go://github.com/o/r1@latest-v1.2",
			want:          "1.2.1-rc.0",
			wantErr:       false,
		},
		{
			name:          "Get stable release when there is no stable release in given minor",
			releaseMarker: "go://github.com/o/r1@v1.3",
			want:          "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := resolveReleaseMarker(context.Background(), tt.releaseMarker, clientGoproxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(BeEquivalentTo(tt.want))
		})
	}
}

func Test_E2EConfig_DeepCopy(t *testing.T) {
	g := NewWithT(t)

	config1 := E2EConfig{
		ManagementClusterName: "config1-cluster-name",
		Images: []ContainerImage{
			{
				Name: "config1-image",
			},
		},
		Providers: []ProviderConfig{
			{
				Name: "config1-provider",
			},
		},
		Variables: map[string]string{
			"config1-variable": "config1-variable-value",
		},
		Intervals: map[string][]string{
			"config1-interval": {"2m", "10s"},
		},
	}

	config2 := config1.DeepCopy()

	// Verify everything was copied from config1
	g.Expect(config2.ManagementClusterName).To(Equal("config1-cluster-name"))
	g.Expect(config2.Images).To(Equal([]ContainerImage{
		{
			Name: "config1-image",
		},
	}))
	g.Expect(config2.Providers).To(Equal([]ProviderConfig{
		{
			Name: "config1-provider",
		},
	}))
	g.Expect(config2.Variables).To(Equal(map[string]string{
		"config1-variable": "config1-variable-value",
	}))
	g.Expect(config2.Intervals).To(Equal(map[string][]string{
		"config1-interval": {"2m", "10s"},
	}))

	// Mutate config2
	config2.ManagementClusterName = "config2-cluster-name"
	config2.Images[0].Name = "config2-image"
	config2.Providers[0].Name = "config2-provider"
	config2.Variables["config2-variable"] = "config2-variable-value"
	config2.Intervals["config2-interval"] = []string{"2m", "10s"}

	// Validate config1 was not mutated
	g.Expect(config1.ManagementClusterName).To(Equal("config1-cluster-name"))
	g.Expect(config1.Images).To(Equal([]ContainerImage{
		{
			Name: "config1-image",
		},
	}))
	g.Expect(config1.Providers).To(Equal([]ProviderConfig{
		{
			Name: "config1-provider",
		},
	}))
	g.Expect(config1.Variables).To(Equal(map[string]string{
		"config1-variable": "config1-variable-value",
	}))
	g.Expect(config1.Intervals).To(Equal(map[string][]string{
		"config1-interval": {"2m", "10s"},
	}))
}
