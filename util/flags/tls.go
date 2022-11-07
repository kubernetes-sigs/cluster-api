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
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
)

// TLSOptions has the options to configure the TLS settings
// for a webhook server.
type TLSOptions struct {
	TLSMinVersion   string
	TLSCipherSuites []string
}

// AddTLSOptions adds the webhook server TLS configuration flags
// to the flag set.
func AddTLSOptions(fs *pflag.FlagSet, options *TLSOptions) {
	fs.StringVar(&options.TLSMinVersion, "tls-min-version", "VersionTLS12",
		"The minimum TLS version in use by the webhook server.\n"+
			fmt.Sprintf("Possible values are %s.", strings.Join(cliflag.TLSPossibleVersions(), ", ")),
	)

	tlsCipherPreferredValues := cliflag.PreferredTLSCipherNames()
	tlsCipherInsecureValues := cliflag.InsecureTLSCipherNames()
	fs.StringSliceVar(&options.TLSCipherSuites, "tls-cipher-suites", []string{},
		"Comma-separated list of cipher suites for the webhook server. "+
			"If omitted, the default Go cipher suites will be used. \n"+
			"Preferred values: "+strings.Join(tlsCipherPreferredValues, ", ")+". \n"+
			"Insecure values: "+strings.Join(tlsCipherInsecureValues, ", ")+".")
}

// GetTLSOptionOverrideFuncs returns a list of TLS configuration overrides to be used
// by the webhook server.
func GetTLSOptionOverrideFuncs(options TLSOptions) ([]func(*tls.Config), error) {
	var tlsOptions []func(config *tls.Config)
	tlsVersion, err := cliflag.TLSVersion(options.TLSMinVersion)
	if err != nil {
		return nil, err
	}
	tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
		cfg.MinVersion = tlsVersion
	})

	if len(options.TLSCipherSuites) != 0 {
		suites, err := cliflag.TLSCipherSuites(options.TLSCipherSuites)
		if err != nil {
			return nil, err
		}
		tlsOptions = append(tlsOptions, func(cfg *tls.Config) {
			cfg.CipherSuites = suites
		})
	}
	return tlsOptions, nil
}
