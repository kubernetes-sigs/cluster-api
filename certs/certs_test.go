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

package certs

import (
	"crypto/x509"
	"testing"

	"github.com/pkg/errors"
)

func TestNewCertificates(t *testing.T) {
	c, err := NewCertificates()
	if err != nil {
		t.Fatalf("error should be nil but is %v", err)
	}
	if c == nil {
		t.Fatal("return value should not be nil")
	}
}

func TestNewCertificateAuthority(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func() error
	}{
		{
			name: "should generate conformant ca certificate",
			testFunc: func() error {
				cert, _, err := NewCertificateAuthority()
				if err != nil {
					return err
				}

				if !cert.MaxPathLenZero {
					return errors.Errorf("Unexpected value for MaxPathLenZero, Want: [true]; Got: [%t]", cert.MaxPathLenZero)
				}

				if cert.MaxPathLen != 0 {
					return errors.Errorf("Unexpected value for MaxPathLen, Want: [0]; Got: [%d]", cert.MaxPathLen)
				}

				if !cert.BasicConstraintsValid {
					return errors.Errorf("Unexpected value for BasicConstraintsValid, Want: [true]; Got: [%t]", cert.BasicConstraintsValid)
				}

				expectedUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
				actualUsage := cert.KeyUsage
				if expectedUsage != actualUsage {
					return errors.Errorf("Unexpected value for KeyUsage, Want: [%d]; Got: [%d]", expectedUsage, actualUsage)
				}

				expectedCommonName := "kubernetes"
				actualCommonName := cert.Subject.CommonName
				if expectedCommonName != actualCommonName {
					return errors.Errorf("Unexpected CommonName, Want: [%s]; Got: [%s]", expectedCommonName, actualCommonName)
				}

				if !cert.IsCA {
					return errors.Errorf("Unexpected value for IsCA, Want: [true]; Got: [%t]", cert.IsCA)
				}

				return nil
			},
		},
	}

	for _, tc := range testCases {
		if err := tc.testFunc(); err != nil {
			t.Fatalf("[%s] failed: %v", tc.name, err)
		}
	}
}
