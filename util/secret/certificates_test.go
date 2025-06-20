/*
Copyright 2019 The Kubernetes Authors.

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

package secret_test

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/util/certs"
	"sigs.k8s.io/cluster-api/util/secret"
)

func TestNewControlPlaneJoinCertsStacked(t *testing.T) {
	g := NewWithT(t)

	config := &bootstrapv1.ClusterConfiguration{}
	certs := secret.NewControlPlaneJoinCerts(config)
	g.Expect(certs.GetByPurpose(secret.EtcdCA).KeyFile).NotTo(BeEmpty())
}

func TestNewControlPlaneJoinCertsExternal(t *testing.T) {
	g := NewWithT(t)

	config := &bootstrapv1.ClusterConfiguration{
		Etcd: bootstrapv1.Etcd{
			External: &bootstrapv1.ExternalEtcd{},
		},
	}

	certs := secret.NewControlPlaneJoinCerts(config)
	g.Expect(certs.GetByPurpose(secret.EtcdCA).KeyFile).To(BeEmpty())
}

func TestNewControlPlaneJoinCertsAsFilesNotPanicsWhenEmpty(t *testing.T) {
	g := NewWithT(t)

	config := &bootstrapv1.ClusterConfiguration{}
	certs := secret.NewControlPlaneJoinCerts(config)
	g.Expect(certs.AsFiles()).To(BeEmpty())
}

func TestNewCertificatesForInitialControlPlane(t *testing.T) {
	tests := []struct {
		name                            string
		expectedExpiry                  time.Time
		caCertificateValidityPeriodDays *int32
	}{
		{
			name:           "should return default expiry if caCertificateValidityPeriodDays not set",
			expectedExpiry: time.Now().UTC().Add(time.Hour * 24 * 365 * 10), // 10 years.
		},
		{
			name:                            "should return expiry if caCertificateValidityPeriodDays set",
			expectedExpiry:                  time.Now().UTC().Add(time.Hour * 24 * 10), // 10 days.
			caCertificateValidityPeriodDays: ptr.To[int32](10),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)

			clusterCerts := secret.NewCertificatesForInitialControlPlane(&bootstrapv1.ClusterConfiguration{
				CACertificateValidityPeriodDays: test.caCertificateValidityPeriodDays,
			})
			g.Expect(clusterCerts.Generate()).To(Succeed())
			caCert := clusterCerts.GetByPurpose(secret.ClusterCA)

			g.Expect(caCert.KeyPair).NotTo(BeNil())
			g.Expect(caCert.KeyPair.Cert).NotTo(BeNil())

			// Decode the cert
			decodedCert, err := certs.DecodeCertPEM(caCert.KeyPair.Cert)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(decodedCert.NotAfter).Should(BeTemporally("~", test.expectedExpiry, 1*time.Minute))
		})
	}
}
