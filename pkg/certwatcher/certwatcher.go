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

// Package certwatcher provides a Secret-backed TLS certificate watcher.
package certwatcher

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Watcher watches a single Secret and exposes its current TLS certificate.
// Invalid updates do not replace the last valid certificate.
type Watcher struct {
	secretKey   types.NamespacedName
	informer    toolscache.SharedIndexInformer
	currentCert atomic.Pointer[tls.Certificate]
}

// New creates a Watcher for a TLS Secret. It reads and validates the current
// certificate before returning so GetCertificate is ready before the webhook
// server starts.
func New(ctx context.Context, config *rest.Config, secretKey types.NamespacedName) (*Watcher, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for webhook certificate Secret: %w", err)
	}
	return newWithClient(ctx, client, secretKey)
}

func newWithClient(ctx context.Context, client kubernetes.Interface, secretKey types.NamespacedName) (*Watcher, error) {
	if secretKey.Namespace == "" || secretKey.Name == "" {
		return nil, fmt.Errorf("webhook certificate Secret namespace and name must be set")
	}

	secret, err := client.CoreV1().Secrets(secretKey.Namespace).Get(ctx, secretKey.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get webhook certificate Secret %s: %w", secretKey, err)
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		0,
		informers.WithNamespace(secretKey.Namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", secretKey.Name).String()
		}),
	)
	w := &Watcher{
		secretKey: secretKey,
		informer:  factory.Core().V1().Secrets().Informer(),
	}
	if err := w.updateCertificate(secret); err != nil {
		return nil, err
	}

	if _, err := w.informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			w.handleUpdate(obj)
		},
		UpdateFunc: func(_, newObj any) {
			w.handleUpdate(newObj)
		},
		DeleteFunc: func(obj any) {
			w.handleDelete(obj)
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to add event handler for webhook certificate Secret %s: %w", secretKey, err)
	}

	return w, nil
}

// GetCertificate returns the latest valid certificate loaded from the Secret.
func (w *Watcher) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	certificate := w.currentCert.Load()
	if certificate == nil {
		return nil, fmt.Errorf("webhook certificate Secret %s has not been loaded", w.secretKey)
	}
	return certificate, nil
}

// TLSConfig sets GetCertificate on a TLS config.
func (w *Watcher) TLSConfig(config *tls.Config) {
	config.GetCertificate = w.GetCertificate
}

// Start starts watching the Secret until the context is canceled.
func (w *Watcher) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx).WithValues("Secret", klog.KRef(w.secretKey.Namespace, w.secretKey.Name))
	log.Info("Starting webhook certificate Secret watcher")
	w.informer.RunWithContext(ctx)
	return nil
}

// NeedLeaderElection indicates that the Watcher should run on every replica.
func (*Watcher) NeedLeaderElection() bool {
	return false
}

func (w *Watcher) handleUpdate(obj any) {
	secret, ok := obj.(*corev1.Secret)
	if !ok || secret.Namespace != w.secretKey.Namespace || secret.Name != w.secretKey.Name {
		return
	}

	if err := w.updateCertificate(secret); err != nil {
		ctrl.Log.WithName("certwatcher").Error(err, "Failed to update webhook certificate; continuing to use the last valid certificate",
			"Secret", klog.KRef(w.secretKey.Namespace, w.secretKey.Name))
	}
}

func (w *Watcher) handleDelete(obj any) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		if tombstone, tombstoneOK := obj.(toolscache.DeletedFinalStateUnknown); tombstoneOK {
			secret, ok = tombstone.Obj.(*corev1.Secret)
		}
	}
	if !ok || secret.Namespace != w.secretKey.Namespace || secret.Name != w.secretKey.Name {
		return
	}

	ctrl.Log.WithName("certwatcher").Info("Webhook certificate Secret was deleted; continuing to use the last valid certificate",
		"Secret", klog.KRef(w.secretKey.Namespace, w.secretKey.Name))
}

func (w *Watcher) updateCertificate(secret *corev1.Secret) error {
	certPEM, ok := secret.Data[corev1.TLSCertKey]
	if !ok {
		return fmt.Errorf("webhook certificate Secret %s does not contain %q", w.secretKey, corev1.TLSCertKey)
	}
	keyPEM, ok := secret.Data[corev1.TLSPrivateKeyKey]
	if !ok {
		return fmt.Errorf("webhook certificate Secret %s does not contain %q", w.secretKey, corev1.TLSPrivateKeyKey)
	}

	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to load certificate from webhook certificate Secret %s: %w", w.secretKey, err)
	}
	w.currentCert.Store(&certificate)
	return nil
}

var _ manager.LeaderElectionRunnable = &Watcher{}
