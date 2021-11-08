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

package config

import (
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func TestCertManagerGet(t *testing.T) {
	type fields struct {
		reader Reader
	}
	tests := []struct {
		name    string
		fields  fields
		want    CertManager
		wantErr bool
	}{
		{
			name: "return default url if no custom config is provided",
			fields: fields{
				reader: test.NewFakeReader(),
			},
			want:    NewCertManager(CertManagerDefaultURL, CertManagerDefaultVersion, CertManagerDefaultTimeout.String()),
			wantErr: false,
		},
		{
			name: "return custom url if defined",
			fields: fields{
				reader: test.NewFakeReader().WithCertManager("foo-url", "vX.Y.Z", ""),
			},
			want:    NewCertManager("foo-url", "vX.Y.Z", CertManagerDefaultTimeout.String()),
			wantErr: false,
		},
		{
			name: "return timeout if defined",
			fields: fields{
				reader: test.NewFakeReader().WithCertManager("", "", "5m"),
			},
			want:    NewCertManager(CertManagerDefaultURL, CertManagerDefaultVersion, "5m"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &certManagerClient{
				reader: tt.fields.reader,
			}
			got, err := p.Get()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
