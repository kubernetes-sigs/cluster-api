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

package framework

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util"
)

// CreateNamespaceInput is the input type for CreateNamespace.
type CreateNamespaceInput struct {
	Creator Creator
	Name    string
}

// CreateNamespace is used to create a namespace object.
// If name is empty, a "test-" + util.RandomString(6) name will be generated.
func CreateNamespace(ctx context.Context, input CreateNamespaceInput, intervals ...interface{}) *corev1.Namespace {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeleteNamespace")
	Expect(input.Creator).NotTo(BeNil(), "input.Creator is required for CreateNamespace")
	if input.Name == "" {
		input.Name = fmt.Sprintf("test-%s", util.RandomString(6))
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: input.Name,
		},
	}
	log.Logf("Creating namespace %s", input.Name)
	Eventually(func() error {
		return input.Creator.Create(ctx, ns)
	}, intervals...).Should(Succeed(), "Failed to create namespace %s", input.Name)

	return ns
}

// EnsureNamespace verifies if a namespaces exists. If it doesn't it will
// create the namespace.
func EnsureNamespace(ctx context.Context, mgmt client.Client, namespace string) {
	ns := &corev1.Namespace{}
	err := mgmt.Get(ctx, client.ObjectKey{Name: namespace}, ns)
	if err != nil && apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Eventually(func() error {
			return mgmt.Create(ctx, ns)
		}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to create namespace %q", namespace)
	}
}

// DeleteNamespaceInput is the input type for DeleteNamespace.
type DeleteNamespaceInput struct {
	Deleter Deleter
	Name    string
}

// DeleteNamespace is used to delete namespace object.
func DeleteNamespace(ctx context.Context, input DeleteNamespaceInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeleteNamespace")
	Expect(input.Deleter).NotTo(BeNil(), "input.Deleter is required for DeleteNamespace")
	Expect(input.Name).NotTo(BeEmpty(), "input.Name is required for DeleteNamespace")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: input.Name,
		},
	}
	log.Logf("Deleting namespace %s", input.Name)
	Eventually(func() error {
		return input.Deleter.Delete(ctx, ns)
	}, intervals...).Should(Succeed(), "Failed to delete namespace %s", input.Name)
}

// WatchNamespaceEventsInput is the input type for WatchNamespaceEvents.
type WatchNamespaceEventsInput struct {
	ClientSet *kubernetes.Clientset
	Name      string
	LogFolder string
}

// WatchNamespaceEvents creates a watcher that streams namespace events into a file.
// Example usage:
//
//	ctx, cancelWatches := context.WithCancel(context.Background())
//	go func() {
//		defer GinkgoRecover()
//		framework.WatchNamespaceEvents(ctx, framework.WatchNamespaceEventsInput{
//			ClientSet: clientSet,
//			Name: namespace.Name,
//			LogFolder:   logFolder,
//		})
//	}()
//	defer cancelWatches()
func WatchNamespaceEvents(ctx context.Context, input WatchNamespaceEventsInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WatchNamespaceEvents")
	Expect(input.ClientSet).NotTo(BeNil(), "input.ClientSet is required for WatchNamespaceEvents")
	Expect(input.Name).NotTo(BeEmpty(), "input.Name is required for WatchNamespaceEvents")

	logFile := filepath.Clean(path.Join(input.LogFolder, "resources", input.Name, "events.log"))
	Expect(os.MkdirAll(filepath.Dir(logFile), 0750)).To(Succeed())

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	Expect(err).NotTo(HaveOccurred())
	defer f.Close()

	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		input.ClientSet,
		10*time.Minute,
		informers.WithNamespace(input.Name),
	)
	eventInformer := informerFactory.Core().V1().Events().Informer()
	_, err = eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			e := obj.(*corev1.Event)
			_, _ = f.WriteString(fmt.Sprintf("[New Event] %s\n\tresource: %s/%s/%s\n\treason: %s\n\tmessage: %s\n\tfull: %#v\n",
				klog.KObj(e), e.InvolvedObject.APIVersion, e.InvolvedObject.Kind, e.InvolvedObject.Name, e.Reason, e.Message, e))
		},
		UpdateFunc: func(_, obj interface{}) {
			e := obj.(*corev1.Event)
			_, _ = f.WriteString(fmt.Sprintf("[Updated Event] %s\n\tresource: %s/%s/%s\n\treason: %s\n\tmessage: %s\n\tfull: %#v\n",
				klog.KObj(e), e.InvolvedObject.APIVersion, e.InvolvedObject.Kind, e.InvolvedObject.Name, e.Reason, e.Message, e))
		},
		DeleteFunc: func(obj interface{}) {},
	})
	Expect(err).NotTo(HaveOccurred())

	stopInformer := make(chan struct{})
	defer close(stopInformer)
	informerFactory.Start(stopInformer)
	<-ctx.Done()
	stopInformer <- struct{}{}
}

// CreateNamespaceAndWatchEventsInput is the input type for CreateNamespaceAndWatchEvents.
type CreateNamespaceAndWatchEventsInput struct {
	Creator   Creator
	ClientSet *kubernetes.Clientset
	Name      string
	LogFolder string
}

// CreateNamespaceAndWatchEvents creates a namespace and setups a watch for the namespace events.
func CreateNamespaceAndWatchEvents(ctx context.Context, input CreateNamespaceAndWatchEventsInput) (*corev1.Namespace, context.CancelFunc) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for CreateNamespaceAndWatchEvents")
	Expect(input.Creator).ToNot(BeNil(), "Invalid argument. input.Creator can't be nil when calling CreateNamespaceAndWatchEvents")
	Expect(input.ClientSet).ToNot(BeNil(), "Invalid argument. input.ClientSet can't be nil when calling ClientSet")
	Expect(input.Name).ToNot(BeEmpty(), "Invalid argument. input.Name can't be empty when calling ClientSet")
	Expect(os.MkdirAll(input.LogFolder, 0750)).To(Succeed(), "Invalid argument. input.LogFolder can't be created in CreateNamespaceAndWatchEvents")

	namespace := CreateNamespace(ctx, CreateNamespaceInput{Creator: input.Creator, Name: input.Name}, "40s", "10s")
	Expect(namespace).ToNot(BeNil(), "Failed to create namespace %q", input.Name)

	log.Logf("Creating event watcher for namespace %q", input.Name)
	watchesCtx, cancelWatches := context.WithCancel(ctx)
	go func() {
		defer GinkgoRecover()
		WatchNamespaceEvents(watchesCtx, WatchNamespaceEventsInput{
			ClientSet: input.ClientSet,
			Name:      namespace.Name,
			LogFolder: input.LogFolder,
		})
	}()
	return namespace, cancelWatches
}
