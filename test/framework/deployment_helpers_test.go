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

package framework

import (
	"testing"

	. "github.com/onsi/gomega"
)

func Test_verifyMetrics(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name: "no panic metric exists",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
`),
		},
		{
			name: "no panic occurred",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 0
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 0
`),
		},
		{
			name: "panic occurred in controller",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 1
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 0
`),
			wantErr: "panic occurred in \"cluster\" controller",
		},
		{
			name: "panic occurred in webhook",
			data: []byte(`
# HELP controller_runtime_max_concurrent_reconciles Maximum number of concurrent reconciles per controller
# TYPE controller_runtime_max_concurrent_reconciles gauge
controller_runtime_max_concurrent_reconciles{controller="cluster"} 10
controller_runtime_max_concurrent_reconciles{controller="clusterclass"} 10
# HELP controller_runtime_reconcile_panics_total Total number of reconciliation panics per controller
# TYPE controller_runtime_reconcile_panics_total counter
controller_runtime_reconcile_panics_total{controller="cluster"} 0
controller_runtime_reconcile_panics_total{controller="clusterclass"} 0
# HELP controller_runtime_webhook_panics_total Total number of webhook panics
# TYPE controller_runtime_webhook_panics_total counter
controller_runtime_webhook_panics_total 1
`),
			wantErr: "panic occurred in webhook",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := verifyMetrics(tt.data)
			if tt.wantErr == "" {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErr))
			}
		})
	}
}
