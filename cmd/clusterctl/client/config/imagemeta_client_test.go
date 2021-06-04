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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_imageMetaClient_AlterImage(t *testing.T) {
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
			name: "no image config: images should not be changes",
			fields: fields{
				reader: test.NewFakeReader(),
			},
			args: args{
				component: "any",
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector: image for the cert-manager/cert-manager-cainjector should be changed",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "foo-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector: image for the cert-manager/cert-manager-webhook should not be changed",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "foo-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-webhook:v1.1.0",
			},
			want:    "quay.io/jetstack/cert-manager-webhook:v1.1.0",
			wantErr: false,
		},
		{
			name: "image config for cert-manager: images for the cert-manager should be changed",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta(CertManagerImageComponent, "foo-repository.io", "foo-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector and for cert-manager: images for the cert-manager/cert-manager-cainjector should be changed according to the most specific",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "foo-tag").
					WithImageMeta(CertManagerImageComponent, "bar-repository.io", "bar-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector and for cert-manager: images for the cert-manager/cert-manager-cainjector should be changed according to the most specific (mixed case)",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "").
					WithImageMeta(CertManagerImageComponent, "", "bar-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:bar-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector and for cert-manager: images for the cert-manager/cert-manager-webhook should be changed according to the most generic",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "foo-tag").
					WithImageMeta(CertManagerImageComponent, "bar-repository.io", "bar-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-webhook:v1.1.0",
			},
			want:    "bar-repository.io/cert-manager-webhook:bar-tag",
			wantErr: false,
		},
		{
			name: "image config for all: images for the cert-manager should be changed",
			fields: fields{
				reader: test.NewFakeReader().WithImageMeta(allImageConfig, "foo-repository.io", "foo-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for all and for cert-manager: images for the cert-manager should be changed according to the most specific",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(allImageConfig, "foo-repository.io", "foo-tag").
					WithImageMeta(CertManagerImageComponent, "bar-repository.io", "bar-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "bar-repository.io/cert-manager-cainjector:bar-tag",
			wantErr: false,
		},
		{
			name: "image config for all and for cert-manager: images for the cert-manager should be changed according to the most specific (mixed case)",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(allImageConfig, "foo-repository.io", "").
					WithImageMeta(CertManagerImageComponent, "", "bar-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:bar-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector, for cert-manager and for all: images for the cert-manager/cert-manager-cainjector should be changed according to the most specific",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "foo-tag").
					WithImageMeta(CertManagerImageComponent, "bar-repository.io", "bar-tag").
					WithImageMeta(allImageConfig, "baz-repository.io", "baz-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:foo-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector, for cert-manager and for all: images for the cert-manager/cert-manager-cainjector should be changed according to the most specific (mixed case)",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "").
					WithImageMeta(CertManagerImageComponent, "", "bar-tag").
					WithImageMeta(allImageConfig, "baz-repository.io", "baz-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-cainjector:v1.1.0",
			},
			want:    "foo-repository.io/cert-manager-cainjector:bar-tag",
			wantErr: false,
		},
		{
			name: "image config for cert-manager/cert-manager-cainjector, for cert-manager and for all: images for the cert-manager/cert-manager-webhook should be changed according to the most generic",
			fields: fields{
				reader: test.NewFakeReader().
					WithImageMeta(fmt.Sprintf("%s/cert-manager-cainjector", CertManagerImageComponent), "foo-repository.io", "foo-tag").
					WithImageMeta(CertManagerImageComponent, "bar-repository.io", "").
					WithImageMeta(allImageConfig, "baz-repository.io", "baz-tag"),
			},
			args: args{
				component: CertManagerImageComponent,
				image:     "quay.io/jetstack/cert-manager-webhook:v1.1.0",
			},
			want:    "bar-repository.io/cert-manager-webhook:baz-tag",
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
			g := NewWithT(t)

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
