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

// Package flags implements the manager options utilities.
package flags

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/server/routes"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// ManagerOptions provides command line flags for manager options.
type ManagerOptions struct {
	// TLS Options
	// These are used for the webhook and for the metrics server (the latter only if secure serving is used)

	// TLSMinVersion is the field that stores the value of the --tls-min-version flag.
	// For further details, please see the description of the flag.
	TLSMinVersion string
	// TLSCipherSuites is the field that stores the value of the --tls-cipher-suites flag.
	// For further details, please see the description of the flag.
	TLSCipherSuites []string

	// Metrics Options
	// These are used to configure the metrics server

	// DiagnosticsAddress is the field that stores the value of the --diagnostics-address flag.
	// For further details, please see the description of the flag.
	DiagnosticsAddress string
	// InsecureDiagnostics is the field that stores the value of the --insecure-diagnostics flag.
	// For further details, please see the description of the flag.
	InsecureDiagnostics bool
}

// AddManagerOptions adds the manager options flags to the flag set.
func AddManagerOptions(fs *pflag.FlagSet, options *ManagerOptions) {
	fs.StringVar(&options.TLSMinVersion, "tls-min-version", "VersionTLS12",
		"The minimum TLS version in use by the webhook server and metrics server (the latter only if --insecure-diagnostics is not set to true).\n"+
			fmt.Sprintf("Possible values are %s.", strings.Join(cliflag.TLSPossibleVersions(), ", ")),
	)

	tlsCipherPreferredValues := cliflag.PreferredTLSCipherNames()
	tlsCipherInsecureValues := cliflag.InsecureTLSCipherNames()
	fs.StringSliceVar(&options.TLSCipherSuites, "tls-cipher-suites", []string{},
		"Comma-separated list of cipher suites for the webhook server and metrics server (the latter only if --insecure-diagnostics is not set to true). "+
			"If omitted, the default Go cipher suites will be used. \n"+
			"Preferred values: "+strings.Join(tlsCipherPreferredValues, ", ")+". \n"+
			"Insecure values: "+strings.Join(tlsCipherInsecureValues, ", ")+".")

	fs.StringVar(&options.DiagnosticsAddress, "diagnostics-address", ":8443",
		"The address the diagnostics endpoint binds to. Per default metrics are served via https and with"+
			"authentication/authorization. To serve via http and without authentication/authorization set --insecure-diagnostics. "+
			"If --insecure-diagnostics is not set the diagnostics endpoint also serves pprof endpoints and an endpoint to change the log level.")

	fs.BoolVar(&options.InsecureDiagnostics, "insecure-diagnostics", false,
		"Enable insecure diagnostics serving. For more details see the description of --diagnostics-address.")
}

// GetManagerOptions returns options which can be used to configure a Manager.
// This function should be used with the corresponding AddManagerOptions func.
func GetManagerOptions(options ManagerOptions) ([]func(config *tls.Config), *metricsserver.Options, error) {
	var tlsOptions []func(config *tls.Config)
	var metricsOptions *metricsserver.Options

	tlsVersion, err := cliflag.TLSVersion(options.TLSMinVersion)
	if err != nil {
		return nil, nil, err
	}
	tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
		cfg.MinVersion = tlsVersion
	})

	if len(options.TLSCipherSuites) != 0 {
		suites, err := cliflag.TLSCipherSuites(options.TLSCipherSuites)
		if err != nil {
			return nil, nil, err
		}
		tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
			cfg.CipherSuites = suites
		})
	}

	// If "--insecure-diagnostics" is set, serve metrics via http
	// and without authentication/authorization.
	if options.InsecureDiagnostics {
		metricsOptions = &metricsserver.Options{
			BindAddress:   options.DiagnosticsAddress,
			SecureServing: false,
		}
	} else {
		// If "--insecure-diagnostics" is not set, serve metrics via https
		// and with authentication/authorization. As the endpoint is protected,
		// we also serve pprof endpoints and an endpoint to change the log level.
		metricsOptions = &metricsserver.Options{
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
			TLSOpts: tlsOptions,
		}
	}

	return tlsOptions, metricsOptions, nil
}
