/*
Copyright 2026 The Kubernetes Authors.

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

package etcd

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestEvaluateDefragRule(t *testing.T) {
	// Shared metrics used across most cases:
	//   dbSize      = 1_500_000_000  (1.5 GiB)
	//   dbSizeInUse =   900_000_000  (0.9 GiB)
	//   dbQuota     = 2_000_000_000  (2.0 GiB)
	// Derived values the CEL expression may also reference:
	//   dbSizeFree   = 600_000_000   (dbSize - dbSizeInUse)
	//   dbQuotaUsage = 0.75          (dbSize / dbQuota)
	const (
		dbSize      = 1_500_000_000
		dbSizeInUse = 900_000_000
		dbQuota     = 2_000_000_000
	)

	tests := []struct {
		name        string
		rule        string
		dbSize      int64
		dbSizeInUse int64
		dbQuota     int64
		wantResult  bool
		wantErr     bool
	}{
		{
			name:        "dbSize threshold satisfied",
			rule:        "dbSize > 1000000000",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  true,
		},
		{
			name:        "dbSize threshold not satisfied",
			rule:        "dbSize > 2000000000",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  false,
		},
		{
			name:        "dbSizeInUse threshold satisfied",
			rule:        "dbSizeInUse > 500000000",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  true,
		},
		{
			name:        "dbSizeFree (derived) threshold satisfied",
			rule:        "dbSizeFree > 500000000",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  true,
		},
		{
			name:        "dbSizeFree (derived) threshold not satisfied",
			rule:        "dbSizeFree > 700000000",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  false,
		},
		{
			name:        "dbQuotaUsage (derived) threshold satisfied",
			rule:        "dbQuotaUsage > 0.7",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  true,
		},
		{
			name:        "dbQuotaUsage (derived) threshold not satisfied",
			rule:        "dbQuotaUsage > 0.8",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  false,
		},
		{
			name:        "dbSize relative to dbQuota satisfied",
			rule:        "dbSize * 10 > dbQuota * 7",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  true,
		},
		{
			name:        "dbSize relative to dbQuota not satisfied",
			rule:        "dbSize * 10 > dbQuota * 8",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  false,
		},
		{
			name:        "compound expression satisfied",
			rule:        "dbSizeFree > 500000000 && dbQuotaUsage > 0.7",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  true,
		},
		{
			name:        "compound expression not satisfied",
			rule:        "dbSizeFree > 500000000 && dbQuotaUsage > 0.8",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantResult:  false,
		},
		{
			name:        "invalid CEL syntax returns error",
			rule:        "dbSize >",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantErr:     true,
		},
		{
			name:        "non-boolean expression returns error",
			rule:        "dbSize + 1",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantErr:     true,
		},
		{
			name:        "unknown variable returns error",
			rule:        "unknownVar > 0",
			dbSize:      dbSize,
			dbSizeInUse: dbSizeInUse,
			dbQuota:     dbQuota,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result, err := EvaluateDefragRule(tt.rule, tt.dbSize, tt.dbSizeInUse, tt.dbQuota)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result).To(Equal(tt.wantResult))
		})
	}
}
