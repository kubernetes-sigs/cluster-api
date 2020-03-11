/*
Copyright 2020 The Kubernetes Authors.

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

func Test_imageMetaClient_AlterImage(t *testing.T) {
	g := NewWithT(t)

	type fields struct {
		reader Reader
	}
	type args struct {
		component string
		image     string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "no image config, image should not be changes",
			fields: fields{
				reader: test.NewFakeReader(),
			},
			args: args{
				component: "any",
				image:     "quay.io/jetstack/cert-manager-cainjector:v0.11.0",
			},
			want:    "quay.io/jetstack/cert-manager-cainjector:v0.11.0",
			wantErr: false,
		},
		{
			name: "image config for cert-manager, image for the cert-manager should be changed",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta("cert-manager", "foo-repository.io", "foo-tag"),
			},
			args: args{
				component: "cert-manager",
				image:     "quay.io/jetstack/cert-manager-cainjector:v0.11.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for all, image for the cert-manager should be changed",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta(allImageConfig, "foo-repository.io", "foo-tag"),
			},
			args: args{
				component: "cert-manager",
				image:     "quay.io/jetstack/cert-manager-cainjector:v0.11.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for all and image config for cert-manager, image for the cert-manager should be changed according to the most specific",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(allImageConfig, "foo-repository.io", "foo-tag").
					WithImageMeta("cert-manager", "bar-repository.io", "bar-tag"),
			},
			args: args{
				component: "cert-manager",
				image:     "quay.io/jetstack/cert-manager-cainjector:v0.11.0",
			},
			want:    "bar-repository.io/cert-manager-cainjector:bar-tag",
			wantErr: false,
		},
		{
			name: "image config for all and image config for cert-manager, image for the cert-manager should be changed according to the most specific (mixed case)",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(allImageConfig, "foo-repository.io", "").
					WithImageMeta("cert-manager", "", "bar-tag"),
			},
			args: args{
				component: "cert-manager",
				image:     "quay.io/jetstack/cert-manager-cainjector:v0.11.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:bar-tag",
			wantErr: false,
		},
		{
			name: "fails if wrong image config",
			fields: fields{
				reader: test.NewFakeReader().WithVar(imagesConfigKey, "invalid"),
			},
			args: args{
				component: "any",
				image:     "any",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "fails if wrong image name",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta(allImageConfig, "foo-Repository.io", ""),
			},
			args: args{
				component: "any",
				image:     "invalid:invalid:invalid",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newImageMetaClient(tt.fields.reader)

			got, err := p.AlterImage(tt.args.component, tt.args.image)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
