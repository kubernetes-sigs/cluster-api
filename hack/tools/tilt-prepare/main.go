//go:build tools
// +build tools

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

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

/*
Example call for tilt up:
	--tools kustomize,envsubst
	--cert-manager
	--kustomize-builds clusterctl.crd:./cmd/clusterctl/config/crd/
	--kustomize-builds observability.tools:./hack/observability/
	--providers core:.
	--providers kubeadm-bootstrap:./bootstrap/kubeadm
	--providers kubeadm-control-plane:./controlplane/kubeadm
	--providers docker:./test/infrastructure/docker
*/

var (
	rootPath      string
	tiltBuildPath string

	toolsFlag            = pflag.StringSlice("tools", []string{}, "list of tools to be created; each value should correspond to a make target")
	certManagerFlag      = pflag.Bool("cert-manager", false, "prepare cert-manager")
	kustomizeBuildsFlag  = pflag.StringSlice("kustomize-builds", []string{}, "list of kustomize build to be run; each value should be in the form name:path")
	providersBuildsFlag  = pflag.StringSlice("providers", []string{}, "list of providers to be installed; each value should be in the form name:path")
	allowK8SContextsFlag = pflag.StringSlice("allow-k8s-contexts", []string{}, "Specifies that Tilt is allowed to run against the specified k8s context name; Kind is automatically allowed")
	tiltSettingsFileFlag = pflag.String("tilt-settings-file", "./tilt-settings.yaml", "Path to a tilt-settings.(json|yaml) file")
)

type tiltSettings struct {
	Debug     map[string]debugConfig `json:"debug,omitempty"`
	ExtraArgs map[string]extraArgs   `json:"extra_args,omitempty"`
}

type debugConfig struct {
	Continue     *bool `json:"continue"`
	Port         *int  `json:"port"`
	ProfilerPort *int  `json:"profiler_port"`
	MetricsPort  *int  `json:"metrics_port"`
}

type extraArgs []string

const (
	kustomizePath = "./hack/tools/bin/kustomize"
	envsubstPath  = "./hack/tools/bin/envsubst"
)

func init() {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		klog.Fatalf("failed to get project root folder: %s: %v)", stderr.String(), err)
	}
	rootPath = strings.Trim(stdout.String(), "\n")
	tiltBuildPath = filepath.Join(rootPath, ".tiltbuild")

	_ = clusterctlv1.AddToScheme(scheme.Scheme)
}

// This tool aims to speed up tilt startup time by running in parallel a set of task
// preparing everything is required for tilt up.
func main() {
	klog.Infof("[main] started\n")
	start := time.Now()

	pflag.Parse()
	if err := allowK8sConfig(); err != nil {
		klog.Exit(fmt.Sprintf("[main] tilt-prepare can't start: %v", err))
	}

	ctx := ctrl.SetupSignalHandler()

	ts, err := readTiltSettings(*tiltSettingsFileFlag)
	if err != nil {
		klog.Exit(fmt.Sprintf("[main] failed to read tilt settings: %v", err))
	}

	// Execute a first group of tilt prepare tasks, building all the tools required in subsequent steps/by tilt.
	if err := tiltTools(ctx); err != nil {
		klog.Exit(fmt.Sprintf("[main] failed to prepare tilt tools: %v", err))
	}

	// execute a second group of tilt prepare tasks, building all the resources required by tilt.
	if err := tiltResources(ctx, ts); err != nil {
		klog.Exit(fmt.Sprintf("[main] failed to prepare tilt resources: %v", err))
	}

	klog.Infof("[main] completed, elapsed: %s\n", time.Since(start))
}

// readTiltSettings reads a tilt-settings.(json|yaml) file from the given path and sets debug defaults for certain
// fields that are not present.
func readTiltSettings(path string) (*tiltSettings, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to open tilt-settings file for path: %s", path))
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read tilt-settings content")
	}

	ts := &tiltSettings{}
	if err := yaml.Unmarshal(data, ts); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal tilt-settings content")
	}

	setDebugDefaults(ts)
	return ts, nil
}

// setDebugDefaults sets default values for debug related fields in tiltSettings.
func setDebugDefaults(ts *tiltSettings) {
	for k := range ts.Debug {
		p := ts.Debug[k]
		if p.Continue == nil {
			p.Continue = pointer.BoolPtr(true)
		}
		if p.Port == nil {
			p.Port = pointer.IntPtr(0)
		}
		if p.ProfilerPort == nil {
			p.ProfilerPort = pointer.IntPtr(0)
		}
		if p.MetricsPort == nil {
			p.MetricsPort = pointer.IntPtr(0)
		}

		ts.Debug[k] = p
	}
}

// allowK8sConfig mimics allow_k8s_contexts; only kind is enabled by default but more can be added.
func allowK8sConfig() error {
	config, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return errors.Wrap(err, "failed to load Kubeconfig")
	}

	if config.CurrentContext == "" {
		return errors.New("failed to get current context")
	}
	context, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return errors.Errorf("failed to get context %s", config.CurrentContext)
	}
	if strings.HasPrefix(context.Cluster, "kind-") {
		return nil
	}

	allowed := sets.NewString(*allowK8SContextsFlag...)
	if !allowed.Has(config.CurrentContext) {
		return errors.Errorf("context %s is not allowed", config.CurrentContext)
	}
	return nil
}

// tiltTools runs tasks required for building all the tools required in subsequent steps/by tilt.
func tiltTools(ctx context.Context) error {
	tasks := map[string]taskFunction{}

	// Create a `make task` for each tool specified defined using the --tools flag.
	for _, t := range *toolsFlag {
		tasks[t] = makeTask(t)
	}

	return runTaskGroup(ctx, "tools", tasks)
}

// tiltResources runs tasks required for building all the resources required by tilt.
func tiltResources(ctx context.Context, ts *tiltSettings) error {
	tasks := map[string]taskFunction{}

	// If required, all the task to install cert manager.
	// NOTE: strictly speaking cert-manager is not a resource, however it is a dependency for most of the actual resources
	// and running this is the same task group of the kustomize/provider tasks gives the maximum benefits in terms of reducing the total elapsed time.
	if *certManagerFlag {
		tasks["cert-manager-cainjector"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-cainjector:%s", config.CertManagerDefaultVersion))
		tasks["cert-manager-webhook"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-webhook:%s", config.CertManagerDefaultVersion))
		tasks["cert-manager-controller"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-controller:%s", config.CertManagerDefaultVersion))
		tasks["cert-manager"] = certManagerTask()
	}

	// Add a kustomize task for each name/path defined using the --kustomize-build flag.
	for _, k := range *kustomizeBuildsFlag {
		values := strings.Split(k, ":")
		if len(values) != 2 {
			return errors.Errorf("[resources] failed to parse --kustomize-build flag %s: value should be in the form of name:path", k)
		}
		name := values[0]
		path := values[1]
		tasks[name] = kustomizeTask(path, fmt.Sprintf("%s.yaml", name))
	}

	// Add a provider task for each name/path defined using the --provider flag.
	for _, p := range *providersBuildsFlag {
		pValues := strings.Split(p, ":")
		if len(pValues) != 2 {
			return errors.Errorf("[resources] failed to parse --provider flag %s: value should be in the form of name:path", p)
		}
		name := pValues[0]
		path := pValues[1]
		tasks[name] = providerTask(name, fmt.Sprintf("%s/config/default", path), ts)
	}

	return runTaskGroup(ctx, "resources", tasks)
}

type taskFunction func(ctx context.Context, prefix string, errors chan error)

// runTaskGroup executes a group of task in parallel handling an error channel.
func runTaskGroup(ctx context.Context, name string, tasks map[string]taskFunction) error {
	if len(tasks) == 0 {
		return nil
	}

	klog.Infof("[%s] task group started\n", name)
	defer func(start time.Time) {
		klog.Infof("[%s] task group completed, elapsed: %s\n", name, time.Since(start))
	}(time.Now())

	// Create a context to be used for canceling all the tasks when another fails.
	ctx, cancel := context.WithCancel(ctx)

	// Make channels to pass fatal errors in WaitGroup
	errors := make(chan error)
	wgDone := make(chan bool)

	wg := new(sync.WaitGroup)
	for taskName, taskFunction := range tasks {
		wg.Add(1)
		taskPrefix := fmt.Sprintf("%s/%s", name, taskName)
		go runTask(ctx, wg, taskPrefix, taskFunction, errors)
	}

	go func() {
		wg.Wait()
		close(wgDone)
	}()

	// Wait until either WaitGroup is done or an error is received through the error channel.
	select {
	case <-wgDone:
		break
	case err := <-errors:
		// cancel all the running tasks
		cancel()
		// consumes all the errors from the channel
		errList := []error{err}
	Loop:
		for {
			select {
			case err := <-errors:
				errList = append(errList, err)
			default:
				break Loop
			}
		}
		close(errors)
		return kerrors.NewAggregate(errList)
	}
	return nil
}

// runTask runs a taskFunction taking care of logging elapsed time and reporting Done to the wait group.
// NOTE: those actions are common to all the tasks, so they are centralized.
func runTask(ctx context.Context, wg *sync.WaitGroup, prefix string, f taskFunction, errCh chan error) {
	klog.Infof("[%s] task started\n", prefix)
	defer func(start time.Time) {
		klog.Infof("[%s] task completed, elapsed: %s\n", prefix, time.Since(start))
	}(time.Now())

	defer wg.Done()

	// This is a catch-all in case a task does not cancel properly, then trigger an error
	// when the error channel has been already closed.
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("[%s] Recovered from panic: %s", prefix, r)
		}
	}()

	// run the actual task.
	f(ctx, prefix, errCh)
}

// makeTask generates a task for invoking a make target.
func makeTask(name string) taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		cmd := exec.CommandContext(ctx, "make", name)

		var stderr bytes.Buffer
		cmd.Dir = rootPath
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to run %s: %s", prefix, cmd.Args, stderr.String())
		}
	}
}

// preLoadImageTask generates a task for pre-loading an image into kind.
func preLoadImageTask(image string) taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		docker, err := container.NewDockerClient()
		if err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to create docker client", prefix)
			return
		}

		if err := docker.PullContainerImageIfNotExists(ctx, image); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to pull %s", prefix, image)
			return
		}

		// get cluster name from env
		name, exists := os.LookupEnv("CAPI_KIND_CLUSTER_NAME")
		if !exists {
			name = "capi-test"
		}

		// set command to use capi cluster name
		namecmd := fmt.Sprintf("--name=%s", name)

		cmd := exec.CommandContext(ctx, //nolint:gosec
			"kind",
			"load",
			"docker-image",
			namecmd,
			image,
		)

		var stderr bytes.Buffer
		cmd.Dir = rootPath
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to run %s: %s", prefix, cmd.Args, stderr.String())
		}
	}
}

// certManagerTask generates a task for installing cert-manager if not already present.
func certManagerTask() taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		config, err := config.New("")
		if err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed create clusterctl config", prefix)
			return
		}
		cluster := cluster.New(cluster.Kubeconfig{}, config)

		if err := cluster.CertManager().EnsureInstalled(); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to install cert-manger", prefix)
		}
	}
}

// kustomizeTask generates a task for running kustomize build on a path and saving the output on a file.
func kustomizeTask(path, out string) taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		cmd := exec.CommandContext(ctx,
			kustomizePath,
			"build",
			path,
			// enable helm to enable helmChartInflationGenerator.
			"--enable-helm",
			// to allow picking up resource files from a different folder.
			"--load-restrictor=LoadRestrictionsNone",
		)

		var stdout, stderr bytes.Buffer
		cmd.Dir = rootPath
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to run %s: %s", prefix, cmd.Args, stderr.String())
			return
		}

		// TODO: consider if to preload images into kind, this will speed up components startup.

		if err := writeIfChanged(prefix, filepath.Join(tiltBuildPath, "yaml", out), stdout.Bytes()); err != nil {
			errCh <- err
		}
	}
}

// providerTask generates a task for creating the component yal for a provider and saving the output on a file.
// NOTE: This task has several sub steps including running kustomize, envsubst, fixing components for debugging,
// and adding the Provider resource mimicking what clusterctl init does.
func providerTask(name, path string, ts *tiltSettings) taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		kustomizeCmd := exec.CommandContext(ctx, kustomizePath, "build", path)
		var stdout1, stderr1 bytes.Buffer
		kustomizeCmd.Dir = rootPath
		kustomizeCmd.Stdout = &stdout1
		kustomizeCmd.Stderr = &stderr1
		if err := kustomizeCmd.Run(); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to run %s: %s", prefix, kustomizeCmd.Args, stderr1.String())
			return
		}

		envsubstCmd := exec.CommandContext(ctx, envsubstPath)
		var stdout2, stderr2 bytes.Buffer
		envsubstCmd.Dir = rootPath
		envsubstCmd.Stdin = bytes.NewReader(stdout1.Bytes())
		envsubstCmd.Stdout = &stdout2
		envsubstCmd.Stderr = &stderr2
		if err := envsubstCmd.Run(); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to run %s: %s", prefix, envsubstCmd.Args, stderr2.String())
			return
		}

		objs, err := utilyaml.ToUnstructured(stdout2.Bytes())
		if err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed parse components yaml", prefix)
			return
		}
		if err := prepareManagerDeployment(name, prefix, objs, ts); err != nil {
			errCh <- err
			return
		}

		providerObj, err := getProviderObj(prefix, objs)
		if err != nil {
			errCh <- err
			return
		}
		objs = append(objs, *providerObj)

		yaml, err := utilyaml.FromUnstructured(objs)
		if err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed convert unstructured objects to yaml", prefix)
			return
		}

		if err := writeIfChanged(prefix, filepath.Join(tiltBuildPath, "yaml", fmt.Sprintf("%s.provider.yaml", name)), yaml); err != nil {
			errCh <- err
		}
	}
}

// writeIfChanged writes yaml to a file if the file does not exist or if the content has changed.
// NOTE: Skipping write in case the content is not changed avoids unnecessary Tiltfile reload.
func writeIfChanged(prefix string, path string, yaml []byte) error {
	_, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrapf(err, "[%s] failed to check if %s exists", prefix, path)
	}

	if err == nil {
		previousYaml, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return errors.Wrapf(err, "[%s] failed to read existing %s", prefix, path)
		}
		if bytes.Equal(previousYaml, yaml) {
			klog.Infof("[%s] no changes in the generated yaml\n", prefix)
			return nil
		}
	}

	err = os.MkdirAll(filepath.Dir(path), 0750)
	if err != nil {
		return errors.Wrapf(err, "[%s] failed to create dir %s", prefix, filepath.Dir(path))
	}

	if err := os.WriteFile(path, yaml, 0600); err != nil {
		return errors.Wrapf(err, "[%s] failed to write %s", prefix, path)
	}
	return nil
}

// prepareManagerDeployment sets the Command and Args for the manager container according to the given tiltSettings.
// If there is a debug config given for the provider, we modify Command and Args to work nicely with the delve debugger.
// If there are extra_args given for the provider, we append those to the ones that already exist in the deployment.
// This has the affect that the appended ones will take precedence, as those are read last.
// Finally, we modify the deployment to enable prometheus metrics scraping.
func prepareManagerDeployment(name, prefix string, objs []unstructured.Unstructured, ts *tiltSettings) error {
	return updateDeployment(prefix, objs, func(d *appsv1.Deployment) {
		for j, container := range d.Spec.Template.Spec.Containers {
			if container.Name != "manager" {
				// as defined in clusterctl Provider Contract "Controllers & Watching namespace"
				continue
			}

			cmd := []string{"sh", "/start.sh", "/manager"}
			args := append(container.Args, []string(ts.ExtraArgs[name])...)

			// alter controller deployment for working nicely with delve debugger;
			// most specifically, configuring delve, starting the manager with profiling enabled, dropping liveness and
			// readiness probes and disabling leader election.
			if d, ok := ts.Debug[name]; ok {
				cmd = []string{"sh", "/start.sh", "/dlv", "--accept-multiclient", "--api-version=2", "--headless=true", "exec"}

				if d.Port != nil && *d.Port > 0 {
					cmd = append(cmd, "--listen=:30000")
				}
				if d.Continue != nil && *d.Continue {
					cmd = append(cmd, "--continue")
				}

				cmd = append(cmd, []string{"--", "/manager"}...)

				if d.ProfilerPort != nil && *d.ProfilerPort > 0 {
					args = append(args, []string{"--profiler-address=:6060"}...)
				}

				debugArgs := make([]string, 0, len(args))
				for _, a := range args {
					if a == "--leader-elect" || a == "--leader-elect=true" {
						continue
					}
					debugArgs = append(debugArgs, a)
				}
				args = debugArgs

				container.LivenessProbe = nil
				container.ReadinessProbe = nil
			}

			// alter the controller deployment for working nicely with prometheus metrics scraping. Specifically, the
			// metrics endpoint is set to listen on all interfaces instead of only localhost, and another port is added
			// to the container to expose the metrics endpoint.
			finalArgs := make([]string, 0, len(args))
			for _, a := range args {
				if strings.HasPrefix(a, "--metrics-bind-addr=") {
					finalArgs = append(finalArgs, "--metrics-bind-addr=0.0.0.0:8080")
					continue
				}
				finalArgs = append(finalArgs, a)
			}

			container.Ports = append(container.Ports, corev1.ContainerPort{
				Name:          "metrics",
				ContainerPort: 8080,
				Protocol:      "TCP",
			})
			container.Command = cmd
			container.Args = finalArgs

			d.Spec.Template.Spec.Containers[j] = container
		}
	})
}

type updateDeploymentFunction func(deployment *appsv1.Deployment)

// updateDeployment passes all (typed) deployments to the updateDeploymentFunction
// to modify as needed.
func updateDeployment(prefix string, objs []unstructured.Unstructured, f updateDeploymentFunction) error {
	for i := range objs {
		obj := objs[i]
		if obj.GetKind() != "Deployment" {
			continue
		}

		// Convert Unstructured into a typed object
		d := &appsv1.Deployment{}
		if err := scheme.Scheme.Convert(&obj, d, nil); err != nil {
			return errors.Wrapf(err, "[%s] failed to convert Deployment to typed object", prefix)
		}

		// Call updater function
		f(d)

		// Convert back to Unstructured
		if err := scheme.Scheme.Convert(d, &obj, nil); err != nil {
			return errors.Wrapf(err, "[%s] failed to convert Deployment to unstructured", prefix)
		}

		objs[i] = obj
	}
	return nil
}

func getProviderObj(prefix string, objs []unstructured.Unstructured) (*unstructured.Unstructured, error) {
	namespace := ""
	manifestLabel := ""
	for i := range objs {
		if objs[i].GetKind() != "Namespace" {
			continue
		}

		namespace = objs[i].GetName()
		manifestLabel = objs[i].GetLabels()[clusterv1.ProviderLabelName]
		break
	}

	providerType := "CoreProvider"
	providerName := manifestLabel
	if strings.HasPrefix(manifestLabel, "infrastructure-") {
		providerType = "InfrastructureProvider"
		providerName = manifestLabel[len("infrastructure-"):]
	}
	if strings.HasPrefix(manifestLabel, "bootstrap-") {
		providerType = "BootstrapProvider"
		providerName = manifestLabel[len("bootstrap-"):]
	}
	if strings.HasPrefix(manifestLabel, "control-plane-") {
		providerType = "ControlPlaneProvider"
		providerName = manifestLabel[len("control-plane-"):]
	}

	provider := &clusterctlv1.Provider{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Provider",
			APIVersion: clusterctlv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      manifestLabel,
			Namespace: namespace,
			Labels: map[string]string{
				clusterv1.ProviderLabelName:          manifestLabel,
				clusterctlv1.ClusterctlLabelName:     "",
				clusterctlv1.ClusterctlCoreLabelName: clusterctlv1.ClusterctlCoreLabelInventoryValue,
			},
		},
		ProviderName: providerName,
		Type:         providerType,
		Version:      "v1.2.99",
	}

	providerObj := &unstructured.Unstructured{}
	if err := scheme.Scheme.Convert(provider, providerObj, nil); err != nil {
		return nil, errors.Wrapf(err, "[%s] failed to convert Provider to unstructured", prefix)
	}
	return providerObj, nil
}
