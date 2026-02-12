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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1beta1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	bootstrapwebhooks "sigs.k8s.io/cluster-api/bootstrap/kubeadm/webhooks"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	controlplanewebhooks "sigs.k8s.io/cluster-api/controlplane/kubeadm/webhooks"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	internalwebhooks "sigs.k8s.io/cluster-api/internal/webhooks"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/test/builder"
	"sigs.k8s.io/cluster-api/version"
	"sigs.k8s.io/cluster-api/webhooks"
)

func init() {
	// This has to be done so klog.Background() uses a proper logger.
	// Otherwise it would fall back and log to os.Stderr.
	// This would lead to race conditions because input.M.Run() writes os.Stderr
	// while some go routines in controller-runtime use os.Stderr to write logs.
	logOptions := logs.NewOptions()
	logOptions.Verbosity = logsv1.VerbosityLevel(2)
	if logLevel := os.Getenv("CAPI_TEST_ENV_LOG_LEVEL"); logLevel != "" {
		logLevelInt, err := strconv.Atoi(logLevel)
		if err != nil {
			klog.ErrorS(err, "Unable to convert value of CAPI_TEST_ENV_LOG_LEVEL environment variable to integer")
			os.Exit(1)
		}
		logOptions.Verbosity = logsv1.VerbosityLevel(logLevelInt)
	}
	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		klog.ErrorS(err, "Unable to validate and apply log options")
		os.Exit(1)
	}

	logger := klog.Background()
	// Use klog as the internal logger for this envtest environment.
	log.SetLogger(logger)
	// Additionally force all controllers to use the Ginkgo logger.
	ctrl.SetLogger(logger)
	// Add logger for ginkgo.
	klog.SetOutput(ginkgo.GinkgoWriter)

	// Calculate the global scheme used by fakeclients.
	registerSchemes(scheme.Scheme)
}

func registerSchemes(s *runtime.Scheme) {
	utilruntime.Must(admissionv1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))

	utilruntime.Must(addonsv1.AddToScheme(s))
	utilruntime.Must(bootstrapv1.AddToScheme(s))
	utilruntime.Must(clusterv1.AddToScheme(s))
	utilruntime.Must(controlplanev1.AddToScheme(s))
	utilruntime.Must(controlplanev1beta1.AddToScheme(s))
	utilruntime.Must(ipamv1.AddToScheme(s))
	utilruntime.Must(runtimev1.AddToScheme(s))
}

// RunInput is the input for Run.
type RunInput struct {
	M                           *testing.M
	ManagerUncachedObjs         []client.Object
	ManagerCacheOptions         cache.Options
	SetupIndexes                func(ctx context.Context, mgr ctrl.Manager)
	SetupReconcilers            func(ctx context.Context, mgr ctrl.Manager)
	SetupEnv                    func(e *Environment)
	MinK8sVersion               string
	AdditionalSchemeBuilder     runtime.SchemeBuilder
	AdditionalCRDDirectoryPaths []string
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

	// Calculate the scheme.
	scheme := runtime.NewScheme()
	registerSchemes(scheme)
	// Register additional schemes from k8s APIs.
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(rbacv1.AddToScheme(scheme))
	// Register additionally passed schemes.
	if input.AdditionalSchemeBuilder != nil {
		utilruntime.Must(input.AdditionalSchemeBuilder.AddToScheme(scheme))
	}

	// Bootstrapping test environment
	env := newEnvironment(ctx, scheme, input.AdditionalCRDDirectoryPaths, input.ManagerCacheOptions, input.ManagerUncachedObjs...)

	ctx, cancel := context.WithCancelCause(ctx)
	env.cancelManager = cancel

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
		if err := os.WriteFile(kubeconfigPath, config, 0o600); err != nil {
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

	var errs []error

	if err := verifyPanicMetrics(); err != nil {
		errs = append(errs, errors.Wrapf(err, "panics occurred during tests"))
	}

	// Tearing down the test environment
	if err := env.stop(); err != nil {
		errs = append(errs, errors.Wrapf(err, "failed to stop the test environment"))
	}

	if len(errs) > 0 {
		panic(kerrors.NewAggregate(errs))
	}

	return code
}

var cacheSyncBackoff = wait.Backoff{
	Duration: 100 * time.Millisecond,
	Factor:   1.5,
	Steps:    8,
	Jitter:   0.4,
}

// Environment encapsulates a Kubernetes local test environment.
type Environment struct {
	manager.Manager
	client.Client
	Config *rest.Config

	env           *envtest.Environment
	cancelManager context.CancelCauseFunc
}

// newEnvironment creates a new environment spinning up a local api-server.
//
// This function should be called only once for each package you're running tests within,
// usually the environment is initialized in a suite_test.go file within a `BeforeSuite` ginkgo block.
func newEnvironment(ctx context.Context, scheme *runtime.Scheme, additionalCRDDirectoryPaths []string, managerCacheOptions cache.Options, uncachedObjs ...client.Object) *Environment {
	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint:dogsled
	root := path.Join(path.Dir(filename), "..", "..", "..")

	crdDirectoryPaths := []string{
		filepath.Join(root, "config", "crd", "bases"),
		filepath.Join(root, "controlplane", "kubeadm", "config", "crd", "bases"),
		filepath.Join(root, "bootstrap", "kubeadm", "config", "crd", "bases"),
	}
	for _, path := range additionalCRDDirectoryPaths {
		crdDirectoryPaths = append(crdDirectoryPaths, filepath.Join(root, path))
	}

	// Create the test environment.
	env := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     crdDirectoryPaths,
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

	// if ARTIFACTS is setup, configure apiserver audit logs to log to ARTIFACTS dir
	if os.Getenv("ARTIFACTS") != "" {
		_, packageFileName, _, _ := goruntime.Caller(2)
		relativePathPackageCallerFile, err := filepath.Rel(root, packageFileName)
		if err != nil {
			klog.Fatalf("unable to get relative path of calling package %+v", err)
		}

		relativePathPackageCallerDir := filepath.Dir(relativePathPackageCallerFile)
		auditLogsDir := filepath.Join(os.Getenv("ARTIFACTS"), relativePathPackageCallerDir)
		auditLogsFilePath := filepath.Join(auditLogsDir, "apiserver-audit-logs")

		if err = os.MkdirAll(auditLogsDir, 0750); err != nil {
			klog.Fatalf("failed to create audit logs dir: %+v", err)
		}

		auditPolicyPath, err := writeAuditPolicy(auditLogsDir)
		if err != nil {
			klog.Fatalf("failed to write audit logs policy file: %+v", err)
		}

		env.ControlPlane = envtest.ControlPlane{}
		env.ControlPlane.APIServer = &envtest.APIServer{}
		env.ControlPlane.APIServer.Configure().Set("audit-log-path", auditLogsFilePath)
		env.ControlPlane.APIServer.Configure().Set("audit-log-format", "json")
		env.ControlPlane.APIServer.Configure().Set("audit-policy-file", auditPolicyPath)
		env.ControlPlane.APIServer.Configure().Set("audit-log-maxage", "0")
		env.ControlPlane.APIServer.Configure().Set("audit-log-maxbackup", "0")
		env.ControlPlane.APIServer.Configure().Set("audit-log-maxsize", "0")
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
		Controller: config.Controller{
			UsePriorityQueue: ptr.To[bool](feature.Gates.Enabled(feature.PriorityQueue)),
		},
		Scheme: scheme,
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
		Cache: managerCacheOptions,
	}

	mgr, err := ctrl.NewManager(env.Config, options)
	if err != nil {
		klog.Fatalf("Failed to start testenv manager: %v", err)
	}

	// Set minNodeStartupTimeout for Test, so it does not need to be at least 30s
	internalwebhooks.SetMinNodeStartupTimeoutSeconds(0)

	// Setup the func to retrieve apiVersion for a GroupKind for conversion webhooks.
	apiVersionGetter := func(gk schema.GroupKind) (string, error) {
		return contract.GetAPIVersion(ctx, mgr.GetClient(), gk)
	}
	controlplanev1beta1.SetAPIVersionGetter(apiVersionGetter)

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
	if err := (&webhooks.MachineDrainRule{}).SetupWebhookWithManager(mgr); err != nil {
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
	if err := (&webhooks.ClusterResourceSet{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for crs: %+v", err)
	}
	if err := (&webhooks.ClusterResourceSetBinding{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for ClusterResourceSetBinding: %+v", err)
	}
	if err := (&webhooks.MachinePool{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for machinepool: %+v", err)
	}
	if err := (&webhooks.ExtensionConfig{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for extensionconfig: %+v", err)
	}
	if err := (&webhooks.IPAddress{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for ipaddress: %v", err)
	}
	if err := (&webhooks.IPAddressClaim{}).SetupWebhookWithManager(mgr); err != nil {
		klog.Fatalf("unable to create webhook for ipaddressclaim: %v", err)
	}

	return &Environment{
		Manager: mgr,
		Client:  mgr.GetClient(),
		Config:  mgr.GetConfig(),
		env:     env,
	}
}

func writeAuditPolicy(dir string) (string, error) {
	policyFile := filepath.Join(dir, "audit-policy.yaml")

	policyYAML := []byte(`
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  - level: RequestResponse
    resources:
      - group: ""
      - group: "cluster.x-k8s.io"
      - group: "infrastructure.cluster.x-k8s.io"
      - group: "controlplane.cluster.x-k8s.io"
      - group: "addons.cluster.x-k8s.io"
      - group: "bootstrap.cluster.x-k8s.io"
      - group: "runtime.cluster.x-k8s.io"
`)

	if err := os.WriteFile(policyFile, policyYAML, 0600); err != nil {
		return "", err
	}
	return policyFile, nil
}

// start starts the manager.
func (e *Environment) start(ctx context.Context) {
	go func() {
		fmt.Println("Starting the test environment manager")
		if err := e.Start(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
		}
	}()
	<-e.Elected()
	e.waitForWebhooks(ctx)
}

// stop stops the test environment.
func (e *Environment) stop() error {
	fmt.Println("Stopping the test environment")
	e.cancelManager(errors.New("test environment stopped"))
	return e.env.Stop()
}

// waitForWebhooks waits for the webhook server to be available.
func (e *Environment) waitForWebhooks(ctx context.Context) {
	port := e.env.WebhookInstallOptions.LocalServingPort

	klog.V(2).Infof("Waiting for webhook port %d to be open prior to running tests", port)
	timeout := 1 * time.Second
	for {
		time.Sleep(1 * time.Second)
		dialer := &net.Dialer{
			Timeout: timeout,
		}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
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
	return e.CreateAndWait(ctx, kubeconfig.GenerateSecret(cluster, kubeconfig.FromEnvTestConfig(e.Config, cluster)))
}

// Cleanup deletes all the given objects.
func (e *Environment) Cleanup(ctx context.Context, objs ...client.Object) error {
	errs := []error{}
	for _, o := range objs {
		err := e.Delete(ctx, o)
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
	if err := e.Create(ctx, obj, opts...); err != nil {
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

// DeleteAndWait deletes the given object and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) DeleteAndWait(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := e.Delete(ctx, obj, opts...); err != nil {
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
					// if not found possible no finalizer and delete just removed the object.
					return true, nil
				}
				return false, err
			}
			if !objCopy.GetDeletionTimestamp().IsZero() {
				return true, nil
			}
			return false, nil
		}); err != nil {
		return errors.Wrapf(err, "object %s, %s is not being added to the testenv client cache", obj.GetObjectKind().GroupVersionKind().String(), key)
	}
	return nil
}

// PatchAndWait creates or updates the given object using server-side apply and waits for the cache to be updated accordingly.
//
// NOTE: Waiting for the cache to be updated helps in preventing test flakes due to the cache sync delays.
func (e *Environment) PatchAndWait(ctx context.Context, obj client.Object, opts ...client.ApplyOption) error {
	objGVK, err := apiutil.GVKForObject(obj, e.Scheme())
	if err != nil {
		return errors.Wrapf(err, "failed to get GVK to set GVK on object")
	}
	// Ensure that GVK is explicitly set because e.Patch below uses json.Marshal
	// to serialize the object and the apiserver would complain if GVK is not sent.
	obj.GetObjectKind().SetGroupVersionKind(objGVK)

	key := client.ObjectKeyFromObject(obj)
	objCopy := obj.DeepCopyObject().(client.Object)
	if err := e.GetAPIReader().Get(ctx, key, objCopy); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	}
	// Store old resource version, empty string if not found.
	oldResourceVersion := objCopy.GetResourceVersion()

	// Convert to Unstructured because Apply only works with ApplyConfigurations and Unstructured.
	objUnstructured := &unstructured.Unstructured{}
	switch obj.(type) {
	case *unstructured.Unstructured:
		objUnstructured = obj.DeepCopyObject().(*unstructured.Unstructured)
	default:
		if err := e.Scheme().Convert(obj, objUnstructured, nil); err != nil {
			return errors.Wrap(err, "failed to convert object to Unstructured")
		}
	}

	if err := e.Apply(ctx, client.ApplyConfigurationFromUnstructured(objUnstructured), opts...); err != nil {
		return err
	}

	// Write back the modified object so callers can access the patched object.
	if err := e.Scheme().Convert(objUnstructured, obj, ctx); err != nil {
		return errors.Wrapf(err, "failed to write object")
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
	if err := e.Create(ctx, ns); err != nil {
		return nil, err
	}

	return ns, nil
}

func verifyPanicMetrics() error {
	metricFamilies, err := metrics.Registry.Gather()
	if err != nil {
		return err
	}

	var errs []error
	for _, metricFamily := range metricFamilies {
		if metricFamily.GetName() == "controller_runtime_reconcile_panics_total" {
			for _, controllerPanicMetric := range metricFamily.Metric {
				if controllerPanicMetric.Counter != nil && controllerPanicMetric.Counter.Value != nil && *controllerPanicMetric.Counter.Value > 0 {
					controllerName := "unknown"
					for _, label := range controllerPanicMetric.Label {
						if *label.Name == "controller" {
							controllerName = *label.Value
						}
					}
					errs = append(errs, fmt.Errorf("%.0f panics occurred in %q controller (check logs for more details)", *controllerPanicMetric.Counter.Value, controllerName))
				}
			}
		}

		if metricFamily.GetName() == "controller_runtime_webhook_panics_total" {
			for _, webhookPanicMetric := range metricFamily.Metric {
				if webhookPanicMetric.Counter != nil && webhookPanicMetric.Counter.Value != nil && *webhookPanicMetric.Counter.Value > 0 {
					errs = append(errs, fmt.Errorf("%.0f panics occurred in webhooks (check logs for more details)", *webhookPanicMetric.Counter.Value))
				}
			}
		}

		if metricFamily.GetName() == "controller_runtime_conversion_webhook_panics_total" {
			for _, webhookPanicMetric := range metricFamily.Metric {
				if webhookPanicMetric.Counter != nil && webhookPanicMetric.Counter.Value != nil && *webhookPanicMetric.Counter.Value > 0 {
					errs = append(errs, fmt.Errorf("%.0f panics occurred in conversion webhooks (check logs for more details)", *webhookPanicMetric.Counter.Value))
				}
			}
		}
	}

	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}

	return nil
}

// ApplyCRDs allows you to add or replace CRDs after test env has been started.
func (e *Environment) ApplyCRDs(ctx context.Context, crdPath string) error {
	installOpts := envtest.CRDInstallOptions{
		Scheme:       e.GetScheme(),
		MaxTime:      10 * time.Second,
		PollInterval: 100 * time.Millisecond,
		Paths: []string{
			crdPath,
		},
		ErrorIfPathMissing: true,
	}

	// Read the CRD YAMLs into options.CRDs.
	if err := envtest.ReadCRDFiles(&installOpts); err != nil {
		return fmt.Errorf("unable to read CRD files: %w", err)
	}

	// Apply the CRDs.
	if err := applyCRDs(ctx, e.GetClient(), installOpts.CRDs); err != nil {
		return fmt.Errorf("unable to create CRD instances: %w", err)
	}

	// Wait for the CRDs to appear in discovery.
	if err := envtest.WaitForCRDs(e.GetConfig(), installOpts.CRDs, installOpts); err != nil {
		return fmt.Errorf("something went wrong waiting for CRDs to appear as API resources: %w", err)
	}

	return nil
}

func applyCRDs(ctx context.Context, c client.Client, crds []*apiextensionsv1.CustomResourceDefinition) error {
	for _, crd := range crds {
		existingCrd := crd.DeepCopy()
		err := c.Get(ctx, client.ObjectKey{Name: crd.GetName()}, existingCrd)
		switch {
		case apierrors.IsNotFound(err):
			if err := c.Create(ctx, crd); err != nil {
				return fmt.Errorf("unable to create CRD %s: %w", crd.GetName(), err)
			}
		case err != nil:
			return fmt.Errorf("unable to get CRD %s to check if it exists: %w", crd.GetName(), err)
		default:
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := c.Get(ctx, client.ObjectKey{Name: crd.GetName()}, existingCrd); err != nil {
					return err
				}
				// Note: Intentionally only overwriting spec and thus preserving metadata labels, annotations, etc.
				existingCrd.Spec = crd.Spec
				return c.Update(ctx, existingCrd)
			}); err != nil {
				return fmt.Errorf("unable to update CRD %s: %w", crd.GetName(), err)
			}
		}
	}
	return nil
}
