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

package controllers

import (
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	operatorv1 "sigs.k8s.io/cluster-api/exp/operator/api/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/operator/internal/envtest"
	// +kubebuilder:scaffold:imports
)

const (
	timeout = time.Second * 30
)

var (
	env *envtest.Environment
	ctx = ctrl.SetupSignalHandler()
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Operator Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func TestMain(m *testing.M) {
	fmt.Println("Creating new test environment")
	utilruntime.Must(flag.CommandLine.Set("v", "5"))

	certificate := &unstructured.Unstructured{}
	certificate.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "Certificate",
	})
	certificateRequest := &unstructured.Unstructured{}
	certificateRequest.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "CertificateRequest",
	})
	issuer := &unstructured.Unstructured{}
	issuer.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "Issuer",
	})

	env = envtest.New(
		&corev1.ConfigMap{},
		&corev1.Secret{},
		certificate,
		certificateRequest,
		issuer,
	)
	certManagerInstaller := NewSingletonInstaller()

	if err := (&GenericProviderReconciler{
		Provider:             &operatorv1.CoreProvider{},
		ProviderList:         &operatorv1.CoreProviderList{},
		Client:               env,
		Config:               env.Config,
		CertManagerInstaller: certManagerInstaller,
	}).SetupWithManager(env.Manager, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		panic(fmt.Sprintf("Failed to start CoreProviderReconciler: %v", err))
	}

	if err := (&GenericProviderReconciler{
		Provider:             &operatorv1.InfrastructureProvider{},
		ProviderList:         &operatorv1.InfrastructureProviderList{},
		Client:               env,
		Config:               env.Config,
		CertManagerInstaller: certManagerInstaller,
	}).SetupWithManager(env.Manager, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		panic(fmt.Sprintf("Failed to start InfrastructureProviderReconciler: %v", err))
	}

	if err := (&GenericProviderReconciler{
		Provider:             &operatorv1.BootstrapProvider{},
		ProviderList:         &operatorv1.BootstrapProviderList{},
		Client:               env,
		Config:               env.Config,
		CertManagerInstaller: certManagerInstaller,
	}).SetupWithManager(env.Manager, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		panic(fmt.Sprintf("Failed to start BootstrapProviderReconciler: %v", err))
	}

	if err := (&GenericProviderReconciler{
		Provider:             &operatorv1.ControlPlaneProvider{},
		ProviderList:         &operatorv1.ControlPlaneProviderList{},
		Client:               env,
		Config:               env.Config,
		CertManagerInstaller: certManagerInstaller,
	}).SetupWithManager(env.Manager, controller.Options{MaxConcurrentReconciles: 1}); err != nil {
		panic(fmt.Sprintf("Failed to start ControlPlaneProviderReconciler: %v", err))
	}

	go func() {
		if err := env.Start(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the envtest manager: %v", err))
		}
	}()
	<-env.Manager.Elected()

	// Run tests
	code := m.Run()
	// Tearing down the test environment
	if err := env.Stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the envtest: %v", err))
	}

	// Report exit code
	os.Exit(code)
}
