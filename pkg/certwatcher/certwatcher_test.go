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

package certwatcher

import (
	"bytes"
	"context"
	"crypto/tls"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/testcerts"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNew(t *testing.T) {
	secretKey := types.NamespacedName{Namespace: "test", Name: "webhook-cert"}

	t.Run("loads the initial certificate", func(t *testing.T) {
		g := NewWithT(t)
		client := fake.NewSimpleClientset(newTLSSecret(secretKey, testcerts.ServerCert, testcerts.ServerKey))

		watcher, err := newWithClient(t.Context(), client, secretKey)
		g.Expect(err).ToNot(HaveOccurred())

		certificate, err := watcher.GetCertificate(nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(certificate.Certificate).ToNot(BeEmpty())

		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		watcher.TLSConfig(tlsConfig)
		g.Expect(tlsConfig.GetCertificate).ToNot(BeNil())
		certificate, err = tlsConfig.GetCertificate(nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(certificate.Certificate).ToNot(BeEmpty())
	})

	for name, key := range map[string]types.NamespacedName{
		"requires a Secret name":      {Namespace: "test"},
		"requires a Secret namespace": {Name: "webhook-cert"},
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := newWithClient(t.Context(), fake.NewSimpleClientset(), key)
			g.Expect(err).To(MatchError("webhook certificate Secret namespace and name must be set"))
		})
	}

	t.Run("fails if the Secret does not exist", func(t *testing.T) {
		g := NewWithT(t)
		_, err := newWithClient(t.Context(), fake.NewSimpleClientset(), secretKey)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails if the initial certificate is invalid", func(t *testing.T) {
		g := NewWithT(t)
		client := fake.NewSimpleClientset(newTLSSecret(secretKey, []byte("invalid"), []byte("invalid")))
		_, err := newWithClient(t.Context(), client, secretKey)
		g.Expect(err).To(HaveOccurred())
	})

	for name, secret := range map[string]*corev1.Secret{
		"fails if the Secret has no certificate": {
			ObjectMeta: metav1.ObjectMeta{Namespace: secretKey.Namespace, Name: secretKey.Name},
			Data:       map[string][]byte{corev1.TLSPrivateKeyKey: testcerts.ServerKey},
		},
		"fails if the Secret has no private key": {
			ObjectMeta: metav1.ObjectMeta{Namespace: secretKey.Namespace, Name: secretKey.Name},
			Data:       map[string][]byte{corev1.TLSCertKey: testcerts.ServerCert},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			_, err := newWithClient(t.Context(), fake.NewSimpleClientset(secret), secretKey)
			g.Expect(err).To(HaveOccurred())
		})
	}
}

func TestWatcher(t *testing.T) {
	g := NewWithT(t)
	secretKey := types.NamespacedName{Namespace: "test", Name: "webhook-cert"}
	client := fake.NewSimpleClientset(newTLSSecret(secretKey, testcerts.ServerCert, testcerts.ServerKey))
	watcher, err := newWithClient(t.Context(), client, secretKey)
	g.Expect(err).ToNot(HaveOccurred())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = watcher.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		g.Eventually(done, 5*time.Second).Should(BeClosed())
	})

	initialCertificate, err := watcher.GetCertificate(nil)
	g.Expect(err).ToNot(HaveOccurred())

	updatedSecret := newTLSSecret(secretKey, testcerts.ClientCert, testcerts.ClientKey)
	_, err = client.CoreV1().Secrets(secretKey.Namespace).Update(t.Context(), updatedSecret, metav1.UpdateOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Eventually(func() bool {
		certificate, err := watcher.GetCertificate(nil)
		return err == nil && !bytes.Equal(certificate.Certificate[0], initialCertificate.Certificate[0])
	}, 5*time.Second).Should(BeTrue())

	validCertificate, err := watcher.GetCertificate(nil)
	g.Expect(err).ToNot(HaveOccurred())
	invalidSecret := newTLSSecret(secretKey, []byte("invalid"), []byte("invalid"))
	_, err = client.CoreV1().Secrets(secretKey.Namespace).Update(t.Context(), invalidSecret, metav1.UpdateOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Consistently(func() bool {
		certificate, err := watcher.GetCertificate(nil)
		return err == nil && bytes.Equal(certificate.Certificate[0], validCertificate.Certificate[0])
	}, time.Second).Should(BeTrue())

	restoredSecret := newTLSSecret(secretKey, testcerts.ServerCert, testcerts.ServerKey)
	_, err = client.CoreV1().Secrets(secretKey.Namespace).Update(t.Context(), restoredSecret, metav1.UpdateOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Eventually(func() bool {
		certificate, err := watcher.GetCertificate(nil)
		return err == nil && bytes.Equal(certificate.Certificate[0], initialCertificate.Certificate[0])
	}, 5*time.Second).Should(BeTrue())

	restoredCertificate, err := watcher.GetCertificate(nil)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(client.CoreV1().Secrets(secretKey.Namespace).Delete(t.Context(), secretKey.Name, metav1.DeleteOptions{})).To(Succeed())
	g.Consistently(func() bool {
		certificate, err := watcher.GetCertificate(nil)
		return err == nil && bytes.Equal(certificate.Certificate[0], restoredCertificate.Certificate[0])
	}, time.Second).Should(BeTrue())
}

func newTLSSecret(secretKey types.NamespacedName, cert, key []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretKey.Namespace,
			Name:      secretKey.Name,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       cert,
			corev1.TLSPrivateKeyKey: key,
		},
	}
}

var _ func(*tls.ClientHelloInfo) (*tls.Certificate, error) = (&Watcher{}).GetCertificate
