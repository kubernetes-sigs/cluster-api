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

// Package flags implements the webhook server TLS options utilities.
package flags

import (
	"net/http"
	"net/http/pprof"

	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/server/routes"
	"k8s.io/component-base/logs"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// DiagnosticsOptions has the options to configure diagnostics.
type DiagnosticsOptions struct {
	// MetricsBindAddr
	//
	// Deprecated: This field will be removed in an upcoming release.
	MetricsBindAddr     string
	DiagnosticsAddress  string
	InsecureDiagnostics bool
}

// AddDiagnosticsOptions adds the diagnostics flags to the flag set.
func AddDiagnosticsOptions(fs *pflag.FlagSet, options *DiagnosticsOptions) {
	fs.StringVar(&options.MetricsBindAddr, "metrics-bind-addr", "",
		"The address the metrics endpoint binds to.")
	_ = fs.MarkDeprecated("metrics-bind-addr", "Please use --diagnostics-address instead. To continue to serve"+
		"metrics via http and without authentication/authorization set --insecure-diagnostics as well.")

	fs.StringVar(&options.DiagnosticsAddress, "diagnostics-address", ":8443",
		"The address the diagnostics endpoint binds to. Per default metrics are served via https and with"+
			"authentication/authorization. To serve via http and without authentication/authorization set --insecure-diagnostics."+
			"If --insecure-diagnostics is not set the diagnostics endpoint also serves pprof endpoints and an endpoint to change the log level.")

	fs.BoolVar(&options.InsecureDiagnostics, "insecure-diagnostics", false,
		"Enable insecure diagnostics serving. For more details see the description of --diagnostics-address.")
}

// GetDiagnosticsOptions returns metrics options which can be used to configure a Manager.
func GetDiagnosticsOptions(options DiagnosticsOptions) metricsserver.Options {
	// If the deprecated "--metrics-bind-addr" flag is set, continue to serve metrics via http
	// and without authentication/authorization.
	if options.MetricsBindAddr != "" {
		return metricsserver.Options{
			BindAddress: options.MetricsBindAddr,
		}
	}

	// If "--insecure-diagnostics" is set, serve metrics via http
	// and without authentication/authorization.
	if options.InsecureDiagnostics {
		return metricsserver.Options{
			BindAddress:   options.DiagnosticsAddress,
			SecureServing: false,
		}
	}

	// If "--insecure-diagnostics" is not set, serve metrics via https
	// and with authentication/authorization. As the endpoint is protected,
	// we also serve pprof endpoints and an endpoint to change the log level.
	return metricsserver.Options{
		BindAddress:    options.DiagnosticsAddress,
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
	}
}
