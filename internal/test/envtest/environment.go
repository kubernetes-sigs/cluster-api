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

package envtest

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/pkg/errors"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	bootstrapwebhooks "sigs.k8s.io/cluster-api/bootstrap/kubeadm/webhooks"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	controlplanewebhooks "sigs.k8s.io/cluster-api/controlplane/kubeadm/webhooks"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	addonswebhooks "sigs.k8s.io/cluster-api/exp/addons/webhooks"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1beta1"
	expipamwebhooks "sigs.k8s.io/cluster-api/exp/ipam/webhooks"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	internalwebhooks "sigs.k8s.io/cluster-api/internal/webhooks"
	runtimewebhooks "sigs.k8s.io/cluster-api/internal/webhooks/runtime"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/version"
	"sigs.k8s.io/cluster-api/webhooks"
)

func init() {
	logger := klog.Background()
	// Use klog as the internal logger for this envtest environment.
	log.SetLogger(logger)
	// Additionally force all controllers to use the Ginkgo logger.
	ctrl.SetLogger(logger)
	// Add logger for ginkgo.
	klog.SetOutput(ginkgo.GinkgoWriter)

	// Calculate the scheme.
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(admissionv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(bootstrapv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(expv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(addonsv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(controlplanev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(admissionv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(runtimev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(ipamv1.AddToScheme(scheme.Scheme))
}

// RunInput is the input for Run.
type RunInput struct {
	M                   *testing.M
	ManagerUncachedObjs []client.Object
	SetupIndexes        func(ctx context.Context, mgr ctrl.Manager)
	SetupReconcilers    func(ctx context.Context, mgr ctrl.Manager)
	SetupEnv            func(e *Environment)
	MinK8sVersion       string
}

// Run executes the tests of the given testing.M in a test environment.
// Note: The environment will be created in this func and should not be created before. This func takes a *Environment
//
//	because our tests require access to the *Environment. We use this field to make the created Environment available
//	to the consumer.
//
// Note: Test environment creation can be skipped by setting the environment variable `CAPI_DISABLE_TEST_ENV`
// to a non-empty value. This only makes sense when executing tests which don't require the test environment,
// e.g. tests using only the fake client.
// Note: It's possible to write a kubeconfig for the test environment to a file by setting `CAPI_TEST_ENV_KUBECONFIG`.
// Note: It's possible to skip stopping the test env after the tests have been run by setting `CAPI_TEST_ENV_SKIP_STOP`
// to a non-empty value.
func Run(ctx context.Context, input RunInput) int {
	if os.Getenv("CAPI_DISABLE_TEST_ENV") != "" {
		klog.Info("Skipping test env start as CAPI_DISABLE_TEST_ENV is set")
		return input.M.Run()
	}

	// Bootstrapping test environment
	env := newEnvironment(input.ManagerUncachedObjs...)

	if input.SetupIndexes != nil {
		input.SetupIndexes(ctx, env.Manager)
	}
	if input.SetupReconcilers != nil {
		input.SetupReconcilers(ctx, env.Manager)
	}

	// Start the environment.
	env.start(ctx)

	if kubeconfigPath := os.Getenv("CAPI_TEST_ENV_KUBECONFIG"); kubeconfigPath != "" {
		klog.Infof("Writing test env kubeconfig to %q", kubeconfigPath)
		config := kubeconfig.FromEnvTestConfig(env.Config, &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		})
		if err := os.WriteFile(kubeconfigPath, config, 0600); err != nil {
			panic(errors.Wrapf(err, "failed to write the test env kubeconfig"))
		}
	}

	if input.MinK8sVersion != "" {
		if err := version.CheckKubernetesVersion(env.Config, input.MinK8sVersion); err != nil {
			fmt.Printf("[ERROR] Cannot run tests after failing version check: %v\n", err)
			if err := env.stop(); err != nil {
				fmt.Println("[WARNING] Failed to stop the test environment")
			}
			return 1
		}
	}

	// Expose the environment.
	input.SetupEnv(env)

	// Run tests
	code := input.M.Run()

	if skipStop := os.Getenv("CAPI_TEST_ENV_SKIP_STOP"); skipStop != "" {
		klog.Info("Skipping test env stop as CAPI_TEST_ENV_SKIP_STOP is set")
		return code
	}

	// Tearing down the test environment
	if err := env.stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the test environment: %v", err))
	}
	return code
}

var (
	cacheSyncBackoff = wait.Backoff{
		Duration: 100 * time.Millisecond,
		Factor:   1.5,
		Steps:    8,
		Jitter:   0.4,
	}
)

// Environment encapsulates a Kubernetes local test environment.
type Environment struct {
	manager.Manager
	client.Client
	Config *rest.Config

	env           *envtest.Environment
	cancelManager context.CancelFunc
}

// newEnvironment creates a new environment spinning up a local api-server.
//
// This function should be called only once for each package you're running tests within,
// usually the environment is initialized in a suite_test.go file within a `BeforeSuite` ginkgo block.
func newEnvironment(uncachedObjs ...client.Object) *Environment {
	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled
	root := path.Join(path.Dir(filename), "..", "..", "..")

	// Create the test environment.
	env := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join(root, "config", "crd", "bases"),
			filepath.Join(root, "controlplane", "kubeadm", "config", "crd", "bases"),
			filepath.Join(root, "bootstrap", "kubeadm", "config", "crd", "bases"),
		},
		CRDs: []*apiextensionsv1.CustomResourceDefinition{
			builder.GenericBootstrapConfigCRD.DeepCopy(),
			builder.GenericBootstrapConfigTemplateCRD.DeepCopy(),
			builder.GenericControlPlaneCRD.DeepCopy(),
			builder.GenericControlPlaneTemplateCRD.DeepCopy(),
			builder.GenericInfrastructureMachineCRD.DeepCopy(),
			builder.GenericInfrastructureMachineTemplateCRD.DeepCopy(),
			builder.GenericInfrastructureMachinePoolCRD.DeepCopy(),
			builder.GenericInfrastructureMachinePoolTemplateCRD.DeepCopy(),
			builder.GenericInfrastructureClusterCRD.DeepCopy(),
			builder.GenericInfrastructureClusterTemplateCRD.DeepCopy(),
			builder.GenericRemediationCRD.DeepCopy(),
			builder.GenericRemediationTemplateCRD.DeepCopy(),
			builder.TestInfrastructureClusterTemplateCRD.DeepCopy(),
			builder.TestInfrastructureClusterCRD.DeepCopy(),
			builder.TestInfrastructureMachineTemplateCRD.DeepCopy(),
			builder.TestInfrastructureMachinePoolCRD.DeepCopy(),
			builder.TestInfrastructureMachinePoolTemplateCRD.DeepCopy(),
			builder.TestInfrastructureMachineCRD.DeepCopy(),
			builder.TestBootstrapConfigTemplateCRD.DeepCopy(),
			builder.TestBootstrapConfigCRD.DeepCopy(),
			builder.TestControlPlaneTemplateCRD.DeepCopy(),
			builder.TestControlPlaneCRD.DeepCopy(),
		},
		// initialize webhook here to be able to test the envtest install via webhookOptions
		// This should set LocalServingCertDir and LocalServingPort that are used below.
		WebhookInstallOptions: initWebhookInstallOptions(),
	}

	if _, err := env.Start(); err != nil {
		err = kerrors.NewAggregate([]error{err, env.Stop()})
		panic(err)
	}

	// Localhost is used on MacOS to avoid Firewall warning popups.
	host := "localhost"
	if strings.EqualFold(os.Getenv("USE_EXISTING_CLUSTER"), "true") {
		// 0.0.0.0 is required on Linux when using kind because otherwise the kube-apiserver running in kind
		// is unable to reach the webhook, because the webhook would be only listening on 127.0.0.1.
		// Somehow that's not an issue on MacOS.
		if goruntime.GOOS == "linux" {
			host = "0.0.0.0"
		}
	}

	options := manager.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: uncachedObjs,
				// Use the cache for all Unstructured get/list calls.
				Unstructured: true,
			},
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:    env.WebhookInstallOptions.LocalServingPort,
				CertDir: env.WebhookInstallOptions.LocalServingCertDir,
				Host:    host,
			},
		),
	}

	mgr, err := ctrl.NewManager(env.Config, options)
	if err != nil {
		klog.Fatalf("Failed to start testenv manager: %v", err)
	}

	// Set minNodeStartupTimeout for Test, so it does not need to be at least 30s
	internalwebhooks.SetMinNodeStartupTimeout(metav1.Duration{Duration: 1 * time.Millisecond})

	if err := (&webhooks.Cluster{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&webhooks.ClusterClass{Client: mgr.GetClient()}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&webhooks.Machine{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&webhooks.MachineHealthCheck{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&webhooks.MachineSet{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&webhooks.MachineDeployment{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&bootstrapwebhooks.KubeadmConfig{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&bootstrapwebhooks.KubeadmConfigTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&controlplanewebhooks.KubeadmControlPlaneTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&controlplanewebhooks.KubeadmControlPlane{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook: %+v", err)
	}
	if err := (&addonswebhooks.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for crs: %+v", err)
	}
	if err := (&addonswebhooks.ClusterResourceSetBinding{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for ClusterResourceSetBinding: %+v", err)
	}
	if err := (&webhooks.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for machinepool: %+v", err)
	}
	if err := (&runtimewebhooks.ExtensionConfig{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for extensionconfig: %+v", err)
	}
	if err := (&expipamwebhooks.IPAddress{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for ipaddress: %v", err)
	}
	if err := (&expipamwebhooks.IPAddressClaim{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for ipaddressclaim: %v", err)
	}

	return &Environment{
		Manager: mgr,
		Client:  mgr.GetClient(),
		Config:  mgr.GetConfig(),
		env:     env,
	}
}

// start starts the manager.
func (e *Environment) start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	e.cancelManager = cancel

	go func() {
		fmt.Println("Starting the test environment manager")
		if err := e.Manager.Start(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
		}
	}()
	<-e.Manager.Elected()
	e.waitForWebhooks()
}

// stop stops the test environment.
func (e *Environment) stop() error {
	fmt.Println("Stopping the test environment")
	e.cancelManager()
	return e.env.Stop()
}

// waitForWebhooks waits for the webhook server to be available.
func (e *Environment) waitForWebhooks() {
	port := e.env.WebhookInstallOptions.LocalServingPort

	klog.V(2).Infof("Waiting for webhook port %d to be open prior to running tests", port)
	timeout := 1 * time.Second
	for {
		time.Sleep(1 * time.Second)
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
		if err != nil {
			klog.V(2).Infof("Webhook port is not ready, will retry in %v: %s", timeout, err)
			continue
		}
		if err := conn.Close(); err != nil {
			klog.V(2).Infof("Closing connection when testing if webhook port is ready failed: %v", err)
		}
		klog.V(2).Info("Webhook port is now open. Continuing with tests...")
		return
	}
}

// CreateKubeconfigSecret generates a new Kubeconfig secret from the envtest config.
func (e *Environment) CreateKubeconfigSecret(ctx context.Context, cluster *clusterv1.Cluster) error {
	return e.Create(ctx, kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(e.Config, cluster)))
}

// Cleanup deletes all the given objects.
func (e *Environment) Cleanup(ctx context.Context, objs ...client.Object) error {
	errs := []error{}
	for _, o := range objs {
		err := e.Client.Delete(ctx, o)
		if apierrors.IsNotFound(err) {
			continue
		}
		errs = append(errs, err)
	}
	return kerrors.NewAggregate(errs)
}

// CleanupAndWait deletes all the given objects and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) CleanupAndWait(ctx context.Context, objs ...client.Object) error {
	if err := e.Cleanup(ctx, objs...); err != nil {
		return err
	}

	// Makes sure the cache is updated with the deleted object
	errs := []error{}
	for _, o := range objs {
		// Ignoring namespaces because in testenv the namespace cleaner is not running.
		if o.GetObjectKind().GroupVersionKind().GroupKind() == corev1.SchemeGroupVersion.WithKind("Namespace").GroupKind() {
			continue
		}

		oCopy := o.DeepCopyObject().(client.Object)
		key := client.ObjectKeyFromObject(o)
		err := wait.ExponentialBackoff(
			cacheSyncBackoff,
			func() (done bool, err error) {
				if err := e.Get(ctx, key, oCopy); err != nil {
					if apierrors.IsNotFound(err) {
						return true, nil
					}
					return false, err
				}
				return false, nil
			})
		errs = append(errs, errors.Wrapf(err, "key %s, %s is not being deleted from the testenv client cache", o.GetObjectKind().GroupVersionKind().String(), key))
	}
	return kerrors.NewAggregate(errs)
}

// CreateAndWait creates the given object and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) CreateAndWait(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := e.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}

	// Makes sure the cache is updated with the new object
	objCopy := obj.DeepCopyObject().(client.Object)
	key := client.ObjectKeyFromObject(obj)
	if err := wait.ExponentialBackoff(
		cacheSyncBackoff,
		func() (done bool, err error) {
			if err := e.Get(ctx, key, objCopy); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		}); err != nil {
		return errors.Wrapf(err, "object %s, %s is not being added to the testenv client cache", obj.GetObjectKind().GroupVersionKind().String(), key)
	}
	return nil
}

// PatchAndWait creates or updates the given object using server-side apply and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) PatchAndWait(ctx context.Context, obj client.Object, opts ...client.PatchOption) error {
	key := client.ObjectKeyFromObject(obj)
	objCopy := obj.DeepCopyObject().(client.Object)
	if err := e.GetAPIReader().Get(ctx, key, objCopy); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Store old resource version, empty string if not found.
	oldResourceVersion := objCopy.GetResourceVersion()

	if err := e.Client.Patch(ctx, obj, client.Apply, opts...); err != nil {
		return err
	}

	// Makes sure the cache is updated with the new object
	if err := wait.ExponentialBackoff(
		cacheSyncBackoff,
		func() (done bool, err error) {
			if err := e.Get(ctx, key, objCopy); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			if objCopy.GetResourceVersion() == oldResourceVersion {
				return false, nil
			}
			return true, nil
		}); err != nil {
		return errors.Wrapf(err, "object %s, %s is not being added to or did not get updated in the testenv client cache", obj.GetObjectKind().GroupVersionKind().String(), key)
	}
	return nil
}

// CreateNamespace creates a new namespace with a generated name.
func (e *Environment) CreateNamespace(ctx context.Context, generateName string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", generateName),
			Labels: map[string]string{
				"testenv/original-name": generateName,
			},
		},
	}
	if err := e.Client.Create(ctx, ns); err != nil {
		return nil, err
	}

	return ns, nil
}
