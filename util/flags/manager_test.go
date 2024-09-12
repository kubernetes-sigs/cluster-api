/*
Copyright 2024 The Kubernetes Authors.

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

package flags

import (
	"crypto/tls"
	"net/http"
	"net/http/pprof"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apiserver/pkg/server/routes"
	"k8s.io/component-base/logs"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func TestGetManagerOptions(t *testing.T) {
	tests := []struct {
		name                        string
		managerOptions              ManagerOptions
		wantTLSConfig               *tls.Config
		wantMetricsOptions          *metricsserver.Options
		wantMetricsOptionsTLSConfig *tls.Config
		wantErr                     bool
	}{
		{
			name: "invalid tls min version",
			managerOptions: ManagerOptions{
				TLSMinVersion: "VersionTLS_DOES_NOT_EXIST",
			},
			wantErr: true,
		},
		{
			name: "invalid tls cipher suites",
			managerOptions: ManagerOptions{
				TLSCipherSuites: []string{"TLS_DOES_NOT_EXIST"},
			},
			wantErr: true,
		},
		{
			name: "valid tls + insecure diagnostics + diagnostics address",
			managerOptions: ManagerOptions{
				TLSMinVersion:       "VersionTLS13",
				TLSCipherSuites:     []string{"TLS_AES_256_GCM_SHA384"},
				InsecureDiagnostics: true,
				DiagnosticsAddress:  ":8443",
			},
			wantTLSConfig: &tls.Config{
				MinVersion:   tls.VersionTLS13,
				CipherSuites: []uint16{tls.TLS_AES_256_GCM_SHA384},
			},
			wantMetricsOptions: &metricsserver.Options{
				BindAddress: ":8443",
			},
			wantErr: false,
		},
		{
			name: "valid tls + diagnostics address",
			managerOptions: ManagerOptions{
				TLSMinVersion:      "VersionTLS13",
				TLSCipherSuites:    []string{"TLS_AES_256_GCM_SHA384"},
				DiagnosticsAddress: ":8443",
			},
			wantTLSConfig: &tls.Config{
				MinVersion:   tls.VersionTLS13,
				CipherSuites: []uint16{tls.TLS_AES_256_GCM_SHA384},
			},
			wantMetricsOptions: &metricsserver.Options{
				BindAddress:    ":8443",
				SecureServing:  true,
				FilterProvider: filters.WithAuthenticationAndAuthorization,
				ExtraHandlers: map[string]http.Handler{
					// Add handler to dynamically change log level.
					"/debug/flags/v": routes.StringFlagPutHandler(logs.GlogSetter),
					// Add pprof handler.
					"/debug/pprof/":        http.HandlerFunc(pprof.Index),
					"/debug/pprof/cmdline": http.HandlerFunc(pprof.Cmdline),
					"/debug/pprof/profile": http.HandlerFunc(pprof.Profile),
					"/debug/pprof/symbol":  http.HandlerFunc(pprof.Symbol),
					"/debug/pprof/trace":   http.HandlerFunc(pprof.Trace),
				},
			},
			wantMetricsOptionsTLSConfig: &tls.Config{
				MinVersion:   tls.VersionTLS13,
				CipherSuites: []uint16{tls.TLS_AES_256_GCM_SHA384},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			tlsOpts, metricsOptions, err := GetManagerOptions(tt.managerOptions)

			tlsConfig := &tls.Config{} //nolint:gosec // it's fine to not set a TLS min version here.
			for _, opt := range tlsOpts {
				opt(tlsConfig)
			}
			var metricsOptionsTLSConfig *tls.Config
			if metricsOptions != nil && metricsOptions.TLSOpts != nil {
				metricsOptionsTLSConfig = &tls.Config{} //nolint:gosec // it's fine to not set a TLS min version here.
				for _, opt := range metricsOptions.TLSOpts {
					opt(metricsOptionsTLSConfig)
				}
			}

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			// When we don't get an error we expect that metricsOptions is never nil.
			g.Expect(metricsOptions).ToNot(BeNil())

			g.Expect(tlsConfig).To(Equal(tt.wantTLSConfig))
			// There is no way we can compare the tlsOpts functions with equal
			metricsOptions.TLSOpts = nil
			// There is no way we can compare the FilterProvider functions with equal
			g.Expect(metricsOptions.FilterProvider == nil).To(Equal(tt.wantMetricsOptions.FilterProvider == nil))
			metricsOptions.FilterProvider = nil
			tt.wantMetricsOptions.FilterProvider = nil
			// There is no way we can compare the handler funcs with equal
			for k := range metricsOptions.ExtraHandlers {
				metricsOptions.ExtraHandlers[k] = nil
			}
			for k := range tt.wantMetricsOptions.ExtraHandlers {
				tt.wantMetricsOptions.ExtraHandlers[k] = nil
			}
			g.Expect(metricsOptions).To(Equal(tt.wantMetricsOptions))
			g.Expect(metricsOptionsTLSConfig).To(Equal(tt.wantMetricsOptionsTLSConfig))
		})
	}
}
