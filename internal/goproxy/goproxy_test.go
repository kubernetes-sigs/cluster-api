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

package goproxy

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"

	goproxytest "sigs.k8s.io/cluster-api/internal/goproxy/test"
)

func TestClient_GetVersions(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second

	scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
	clientGoproxy := NewClient(scheme, host)
	defer teardownGoproxy()

	// setup an handler for returning 2 fake releases
	muxGoproxy.HandleFunc("/github.com/o/r1/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v1.1.0\n")
		fmt.Fprint(w, "v0.2.0\n")
	})

	// setup an handler for returning 2 fake releases for v1
	muxGoproxy.HandleFunc("/github.com/o/r2/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v1.1.0\n")
		fmt.Fprint(w, "v0.2.0\n")
	})
	// setup an handler for returning 2 fake releases for v2
	muxGoproxy.HandleFunc("/github.com/o/r2/v2/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v2.0.1\n")
		fmt.Fprint(w, "v2.0.0\n")
	})

	tests := []struct {
		name         string
		gomodulePath string
		want         semver.Versions
		wantErr      bool
	}{
		{
			"No versions",
			"github.com/o/doesntexist",
			nil,
			true,
		},
		{
			"Two versions < v2",
			"github.com/o/r1",
			semver.Versions{
				semver.MustParse("0.2.0"),
				semver.MustParse("1.1.0"),
			},
			false,
		},
		{
			"Multiple versiosn including > v1",
			"github.com/o/r2",
			semver.Versions{
				semver.MustParse("0.2.0"),
				semver.MustParse("1.1.0"),
				semver.MustParse("2.0.0"),
				semver.MustParse("2.0.1"),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			g := NewWithT(t)

			got, err := clientGoproxy.GetVersions(ctx, tt.gomodulePath)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(BeEquivalentTo(tt.want))
		})
	}
}

func Test_GetGoproxyHost(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second

	tests := []struct {
		name       string
		envvar     string
		wantScheme string
		wantHost   string
		wantErr    bool
	}{
		{
			name:       "defaulting",
			envvar:     "",
			wantScheme: "https",
			wantHost:   "proxy.golang.org",
			wantErr:    false,
		},
		{
			name:       "direct falls back to empty strings",
			envvar:     "direct",
			wantScheme: "",
			wantHost:   "",
			wantErr:    false,
		},
		{
			name:       "off falls back to empty strings",
			envvar:     "off",
			wantScheme: "",
			wantHost:   "",
			wantErr:    false,
		},
		{
			name:       "other goproxy",
			envvar:     "foo.bar.de",
			wantScheme: "https",
			wantHost:   "foo.bar.de",
			wantErr:    false,
		},
		{
			name:       "other goproxy comma separated, return first",
			envvar:     "foo.bar,foobar.barfoo",
			wantScheme: "https",
			wantHost:   "foo.bar",
			wantErr:    false,
		},
		{
			name:       "other goproxy including https scheme",
			envvar:     "https://foo.bar",
			wantScheme: "https",
			wantHost:   "foo.bar",
			wantErr:    false,
		},
		{
			name:       "other goproxy including http scheme",
			envvar:     "http://foo.bar",
			wantScheme: "http",
			wantHost:   "foo.bar",
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			gotScheme, gotHost, err := GetSchemeAndHost(tt.envvar)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotScheme).To(Equal(tt.wantScheme))
			g.Expect(gotHost).To(Equal(tt.wantHost))
		})
	}
}
