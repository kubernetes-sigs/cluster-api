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

package repository

import (
	"testing"
	"time"
)

func Test_getGoproxyHost(t *testing.T) {
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
			gotScheme, gotHost, err := getGoproxyHost(tt.envvar)
			if (err != nil) != tt.wantErr {
				t.Errorf("getGoproxyHost() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotScheme != tt.wantScheme {
				t.Errorf("getGoproxyHost() = %v, wantScheme %v", gotScheme, tt.wantScheme)
			}
			if gotHost != tt.wantHost {
				t.Errorf("getGoproxyHost() = %v, wantHost %v", gotHost, tt.wantHost)
			}
		})
	}
}
