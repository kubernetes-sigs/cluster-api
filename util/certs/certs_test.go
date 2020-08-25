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

package certs

import (
	"testing"

	. "github.com/onsi/gomega"
)

type decodeTest struct {
	name        string
	key         []byte
	expectError bool
}

func TestDecodePrivateKeyPEM(t *testing.T) {

	cases := []decodeTest{
		{
			name: "successfully processes PKCS1 private key",
			key: []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIICXAIBAAKBgQCgcTrC6rTj6KV5GeUyEODguAY+RMxX0ZzskOZBUFuUn1ADj7qK
vdfF9WHetcvvnnZ+XuCWrHcoRRIiO5Ikpnz0H54J9Zdy5UAIqkGCOIEdhAVDvLBe
oJ7G2x11Lyz/us7EekqNeguZ9xJ+efjWsuPwYxo8iWluR3jcIA3NK5QCLQIDAQAB
AoGBAIr1xwkvM4D57OfYb9RPHhZEDNQ9ziZ5nEqgrW0AZnFxEmIjSFQGXS5Ne3jj
SEC/pK2LC0Y1FfdA65XOtqMbt7hx3QqjBYIu01AyQGYnrSsiSPdLf4RZviEmZ19n
kuZKKI6TjLXG9LfZO9/x3bYJeHa+rgZoSYK/JEUznIn768/BAkEAzKtZhwLH3zcI
mFyOYjIk2pFauz5tt/9pdXOFHRFS3KKsIrbI2NZd5C5dVp5mnRZ27H4g9HZGurxy
3zWfcrRQ1QJBAMiuUH5iIcWdoRJsgUgCmCYsaynzZgLecEF7VOlRWHiJ60bwNZTG
p0TkEewdmPogbCmaAEtovsBFuQ4JCIxVV/kCQFFn+iUUOxGSny2S6uMt1LDGzdLa
IuPjiDu6JgEIye+OGG96SmrM4O2Ib4GrYV8r90Nba5owjTNrDzmu52vFQr0CQDE9
3JB2YdUMraZIq5xQzqanRZBgogpYLHFU4uvxQuUo6mtYq70a1ZZo5CDszkmpxQCc
QjA+vneNZDAWdVuB4XkCQHjO1CcHKWlihm/xmXDVQKK4oWrNrs6MddLwJ6vAZBAw
I8eun6k9HNyEieJTVaB9AVnykoZ78UbCQaipm9W7i4Q=
-----END RSA PRIVATE KEY-----
			`),
		},
		{
			name: "successfully processes PKCS8 private key",
			key: []byte(`
-----BEGIN PRIVATE KEY-----
MIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAKBxOsLqtOPopXkZ
5TIQ4OC4Bj5EzFfRnOyQ5kFQW5SfUAOPuoq918X1Yd61y++edn5e4JasdyhFEiI7
kiSmfPQfngn1l3LlQAiqQYI4gR2EBUO8sF6gnsbbHXUvLP+6zsR6So16C5n3En55
+Nay4/BjGjyJaW5HeNwgDc0rlAItAgMBAAECgYEAivXHCS8zgPns59hv1E8eFkQM
1D3OJnmcSqCtbQBmcXESYiNIVAZdLk17eONIQL+krYsLRjUV90Drlc62oxu3uHHd
CqMFgi7TUDJAZietKyJI90t/hFm+ISZnX2eS5koojpOMtcb0t9k73/Hdtgl4dr6u
BmhJgr8kRTOcifvrz8ECQQDMq1mHAsffNwiYXI5iMiTakVq7Pm23/2l1c4UdEVLc
oqwitsjY1l3kLl1WnmadFnbsfiD0dka6vHLfNZ9ytFDVAkEAyK5QfmIhxZ2hEmyB
SAKYJixrKfNmAt5wQXtU6VFYeInrRvA1lManROQR7B2Y+iBsKZoAS2i+wEW5DgkI
jFVX+QJAUWf6JRQ7EZKfLZLq4y3UsMbN0toi4+OIO7omAQjJ744Yb3pKaszg7Yhv
gathXyv3Q1trmjCNM2sPOa7na8VCvQJAMT3ckHZh1QytpkirnFDOpqdFkGCiClgs
cVTi6/FC5Sjqa1irvRrVlmjkIOzOSanFAJxCMD6+d41kMBZ1W4HheQJAeM7UJwcp
aWKGb/GZcNVAorihas2uzox10vAnq8BkEDAjx66fqT0c3ISJ4lNVoH0BWfKShnvx
RsJBqKmb1buLhA==
-----END PRIVATE KEY-----
			`),
		},
		{
			name: "successfully processes EC private key",
			key: []byte(`
-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIOsVFUX30MNP7e+MFRTbdknxaC3q3S8fYvmXtrM9tPJJoAoGCCqGSM49
AwEHoUQDQgAERhsfjOmIFAKxuniysAVbR2GJefo03OombXMr1SuuPyTtlcEbWh4b
X9ZN2FCDgn06wSq/cZvLOl2tGPRt5wSMug==
-----END EC PRIVATE KEY-----
			`),
		},
		{
			name: "return error for bad format private key",
			key: []byte(`
-----BEGIN RSA PRIVATE KEY-----
sxcvMIICXAIBAAKBgQCgcTrC6rTj6KV5GeUyEODguAY+RMxX0ZzskOZBUFuUn1ADj7qK
vdfF9WHetcvvnnZ+XuCWrHcoRRIiO5Ikpnz0H54J9Zdy5UAIqkGCOIEdhAVDvLBe
oJ7G2x11Lyz/us7EekqNeguZ9xJ+efjWsuPwYxo8iWluR3jcIA3NK5QCLQIDAQAB
AoGBAIr1xwkvM4D57OfYb9RPHhZEDNQ9ziZ5nEqgrW0AZnFxEmIjSFQGXS5Ne3jj
SEC/pK2LC0Y1FfdA65XOtqMbt7hx3QqjBYIu01AyQGYnrSsiSPdLf4RZviEmZ19n
kuZKKI6TjLXG9LfZO9/x3bYJeHa+rgZoSYK/JEUznIn768/BAkEAzKtZhwLH3zcI
mFyOYjIk2pFauz5tt/9pdXOFHRFS3KKsIrbI2NZd5C5dVp5mnRZ27H4g9HZGurxy
3zWfcrRQ1QJBAMiuUH5iIcWdoRJsgUgCmCYsaynzZgLecEF7VOlRWHiJ60bwNZTG
p0TkEewdmPogbCmaAEtovsBFuQ4JCIxVV/kCQFFn+iUUOxGSny2S6uMt1LDGzdLa
IuPjiDu6JgEIye+OGG96SmrM4O2Ib4GrYV8r90Nba5owjTNrDzmu52vFQr0CQDE9
3JB2YdUMraZIq5xQzqanRZBgogpYLHFU4uvxQuUo6mtYq70a1ZZo5CDszkmpxQCc
QjA+vneNZDAWdVuB4XkCQHjO1CcHKWlihm/xmXDVQKK4oWrNrs6MddLwJ6vAZBAw
I8eun6k9HNyEieJTVaB9AVnykoZ78UbCQaipm9W7i4Q=
-----END RSA PRIVATE KEY-----
			`),
			expectError: true,
		},
		{
			name:        "return error for un-decodeable key",
			key:         []byte("un-decodeable"),
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := DecodePrivateKeyPEM(tc.key)
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func TestDecodeCertPEM(t *testing.T) {
	cases := []decodeTest{
		{
			name:        "return error for un-decodeable cert",
			key:         []byte("un-decodeable"),
			expectError: true,
		},
	}

	for _, tc := range cases {
		g := NewWithT(t)
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeCertPEM(tc.key)
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}

}
