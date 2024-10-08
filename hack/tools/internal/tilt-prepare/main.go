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

// main is the main package for Tilt Prepare.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
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
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kustomize/api/types"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"
)

/*
Example call for tilt up:
	--tools kustomize,envsubst
*/

const (
	kustomizePath = "./hack/tools/bin/kustomize"
	envsubstPath  = "./hack/tools/bin/envsubst"
)

var (
	// Defines the default version to be used for the provider CR if no version is specified in the tilt-provider.yaml|json file.
	defaultProviderVersion = "v1.9.99"

	// This data struct mirrors a subset of info from the providers struct in the tilt file
	// which is containing "hard-coded" tilt-provider.yaml files for the providers managed in the Cluster API repository.
	providers = map[string]tiltProviderConfig{
		"core": {
			Context:           ptr.To("."),
			hardCodedProvider: true,
		},
		"kubeadm-bootstrap": {
			Context:           ptr.To("bootstrap/kubeadm"),
			hardCodedProvider: true,
		},
		"kubeadm-control-plane": {
			Context:           ptr.To("controlplane/kubeadm"),
			hardCodedProvider: true,
		},
		"docker": {
			Context:           ptr.To("test/infrastructure/docker"),
			hardCodedProvider: true,
		},
		"in-memory": {
			Context:           ptr.To("test/infrastructure/inmemory"),
			hardCodedProvider: true,
		},
		"test-extension": {
			Context:           ptr.To("test/extension"),
			hardCodedProvider: true,
		},
	}

	rootPath             string
	tiltBuildPath        string
	tiltSettingsFileFlag = pflag.String("tilt-settings-file", "./tilt-settings.yaml", "Path to a tilt-settings.(json|yaml) file")
	toolsFlag            = pflag.StringSlice("tools", []string{}, "list of tools to be created; each value should correspond to a make target")
)

// Types used to de-serialize the tilt-settings.yaml/json file from the Cluster API repository.

type tiltSettings struct {
	Debug                    map[string]tiltSettingsDebugConfig `json:"debug,omitempty"`
	ExtraArgs                map[string]tiltSettingsExtraArgs   `json:"extra_args,omitempty"`
	DeployCertManager        *bool                              `json:"deploy_cert_manager,omitempty"`
	DeployObservability      []string                           `json:"deploy_observability,omitempty"`
	EnableProviders          []string                           `json:"enable_providers,omitempty"`
	AllowedContexts          []string                           `json:"allowed_contexts,omitempty"`
	ProviderRepos            []string                           `json:"provider_repos,omitempty"`
	AdditionalKustomizations map[string]string                  `json:"additional_kustomizations,omitempty"`
}

type tiltSettingsDebugConfig struct {
	Continue     *bool `json:"continue"`
	Port         *int  `json:"port"`
	ProfilerPort *int  `json:"profiler_port"`
}

type tiltSettingsExtraArgs []string

// Types used to de-serialize the tilt-providers.yaml/json file from the provider repositories.

type tiltProvider struct {
	Name   string              `json:"name,omitempty"`
	Config *tiltProviderConfig `json:"config,omitempty"`
}

type tiltProviderConfig struct {
	Context           *string  `json:"context,omitempty"`
	Version           *string  `json:"version,omitempty"`
	LiveReloadDeps    []string `json:"live_reload_deps,omitempty"`
	ApplyProviderYaml *bool    `json:"apply_provider_yaml,omitempty"`
	KustomizeFolder   *string  `json:"kustomize_folder,omitempty"`
	KustomizeOptions  []string `json:"kustomize_options,omitempty"`

	// hardCodedProvider is used for providers hardcoded in the Tiltfile to always set cmd for them to start.sh to allow restarts.
	hardCodedProvider bool
}

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
	// Set clusterctl logger with a log level of 5.
	// This makes it easier to see what clusterctl is doing and to debug it.
	logf.SetLogger(logf.NewLogger(logf.WithThreshold(ptr.To(5))))

	// Set controller-runtime logger as well.
	ctrl.SetLogger(klog.Background())

	klog.Infof("[main] started\n")
	start := time.Now()

	pflag.Parse()

	ctx := ctrl.SetupSignalHandler()

	ts, err := readTiltSettings(*tiltSettingsFileFlag)
	if err != nil {
		klog.Exit(fmt.Sprintf("[main] failed to read tilt settings: %v", err))
	}

	if err := allowK8sConfig(ts); err != nil {
		klog.Exit(fmt.Sprintf("[main] tilt-prepare can't start: %v", err))
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

	setTiltSettingsDefaults(ts)
	return ts, nil
}

// setTiltSettingsDefaults sets default values for tiltSettings info.
func setTiltSettingsDefaults(ts *tiltSettings) {
	if ts.DeployCertManager == nil {
		ts.DeployCertManager = ptr.To(true)
	}

	for k := range ts.Debug {
		p := ts.Debug[k]
		if p.Continue == nil {
			p.Continue = ptr.To(true)
		}
		if p.Port == nil {
			p.Port = ptr.To(0)
		}
		if p.ProfilerPort == nil {
			p.ProfilerPort = ptr.To(0)
		}

		ts.Debug[k] = p
	}
}

// allowK8sConfig mimics allow_k8s_contexts; only kind is enabled by default but more can be added.
func allowK8sConfig(ts *tiltSettings) error {
	config, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return errors.Wrap(err, "failed to load KubeConfig file")
	}

	if config.CurrentContext == "" {
		return errors.New("failed to get current context from the KubeConfig file")
	}
	context, ok := config.Contexts[config.CurrentContext]
	if !ok {
		return errors.Errorf("failed to get context %s from the KubeConfig file", config.CurrentContext)
	}
	if strings.HasPrefix(context.Cluster, "kind-") {
		return nil
	}

	allowed := sets.Set[string]{}.Insert(ts.AllowedContexts...)
	if !allowed.Has(config.CurrentContext) {
		return errors.Errorf("context %s from the KubeConfig file is not allowed", config.CurrentContext)
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

	// Note: we are creating clusterctl CRDs using kustomize (vs using clusterctl) because we want to create
	// a dependency between these resources and provider resources.
	tasks["clusterctl.crd"] = kustomizeTask("./cmd/clusterctl/config/crd/", "clusterctl.crd.yaml")

	// If required, all the task to install cert manager.
	// NOTE: strictly speaking cert-manager is not a resource, however it is a dependency for most of the actual resources
	// and running this is the same task group of the kustomize/provider tasks gives the maximum benefits in terms of reducing the total elapsed time.
	if ts.DeployCertManager == nil || *ts.DeployCertManager {
		cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
		if err != nil {
			return errors.Wrap(err, "failed to load KubeConfig file")
		}
		// The images can only be preloaded when the cluster is a kind cluster.
		// Note: Not repeating the validation on the config already done in allowK8sConfig here.
		if strings.HasPrefix(cfg.Contexts[cfg.CurrentContext].Cluster, "kind-") {
			tasks["cert-manager-cainjector"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-cainjector:%s", config.CertManagerDefaultVersion))
			tasks["cert-manager-webhook"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-webhook:%s", config.CertManagerDefaultVersion))
			tasks["cert-manager-controller"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-controller:%s", config.CertManagerDefaultVersion))
		}
		tasks["cert-manager"] = certManagerTask()
	}

	// Add a kustomize task for each tool configured via deploy_observability.
	for _, tool := range ts.DeployObservability {
		name := fmt.Sprintf("%s.observability", tool)
		path := fmt.Sprintf("./hack/observability/%s/", tool)
		tasks[name] = sequential(
			cleanupChartTask(path),
			kustomizeTask(path, fmt.Sprintf("%s.yaml", name)),
		)
	}

	for name, path := range ts.AdditionalKustomizations {
		name := fmt.Sprintf("%s.kustomization", name)
		tasks[name] = sequential(
			kustomizeTask(path, fmt.Sprintf("%s.yaml", name)),
		)
	}

	// Add read configurations from provider repos
	for _, p := range ts.ProviderRepos {
		tiltProviderConfigs, err := loadTiltProvider(p)
		if err != nil {
			return errors.Wrapf(err, "failed to load tilt-provider.yaml/json from %s", p)
		}

		for name, config := range tiltProviderConfigs {
			providers[name] = config
		}
	}

	enableCore := false
	for _, providerName := range ts.EnableProviders {
		if providerName == "core" {
			enableCore = true
		}
	}
	if !enableCore {
		ts.EnableProviders = append(ts.EnableProviders, "core")
	}

	// Add the provider task for each of the enabled provider
	for _, providerName := range ts.EnableProviders {
		config, ok := providers[providerName]
		if !ok {
			return errors.Errorf("failed to obtain config for the provider %s, please add the providers path to the provider_repos list in tilt-settings.yaml/json file", providerName)
		}
		if ptr.Deref(config.ApplyProviderYaml, true) {
			kustomizeFolder := path.Join(*config.Context, ptr.Deref(config.KustomizeFolder, "config/default"))
			kustomizeOptions := config.KustomizeOptions
			liveReloadDeps := config.LiveReloadDeps
			var debugConfig *tiltSettingsDebugConfig
			if d, ok := ts.Debug[providerName]; ok {
				debugConfig = &d
			}
			extraArgs := ts.ExtraArgs[providerName]
			tasks[providerName] = workloadTask(providerName, "provider", "manager", "manager", liveReloadDeps, debugConfig, extraArgs, config.hardCodedProvider, kustomizeFolder, kustomizeOptions, getProviderObj(config.Version))
		}
	}

	return runTaskGroup(ctx, "resources", tasks)
}

func loadTiltProvider(providerRepository string) (map[string]tiltProviderConfig, error) {
	tiltProviders, err := readTiltProvider(providerRepository)
	if err != nil {
		return nil, err
	}

	ret := make(map[string]tiltProviderConfig)
	for _, p := range tiltProviders {
		if p.Config == nil {
			return nil, errors.Errorf("tilt-provider.yaml/json file from %s does not contain a config section for provider %s", providerRepository, p.Name)
		}

		// Resolving context, that is a relative path to the repository where the tilt-provider is defined
		contextPath := filepath.Join(providerRepository, ptr.Deref(p.Config.Context, "."))

		ret[p.Name] = tiltProviderConfig{
			Context:           &contextPath,
			Version:           p.Config.Version,
			LiveReloadDeps:    p.Config.LiveReloadDeps,
			ApplyProviderYaml: p.Config.ApplyProviderYaml,
			KustomizeFolder:   p.Config.KustomizeFolder,
			KustomizeOptions:  p.Config.KustomizeOptions,
		}
	}
	return ret, nil
}

func readTiltProvider(path string) ([]tiltProvider, error) {
	path, err := addTiltProviderFile(path)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	ps := []tiltProvider{}
	// providerSettings file can be an array and this is done to detect arrays
	if strings.HasPrefix(string(content), "[") || strings.HasPrefix(string(content), "-") {
		if err := yaml.Unmarshal(content, &ps); err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to read provider path %s tilt file", path))
		}
	} else {
		p := tiltProvider{}
		if err := yaml.Unmarshal(content, &p); err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to read provider path %s tilt file", path))
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func addTiltProviderFile(path string) (string, error) {
	pathAndFile := filepath.Join(path, "tilt-provider.yaml")
	if _, err := os.Stat(pathAndFile); err == nil {
		return pathAndFile, nil
	}
	pathAndFile = filepath.Join(path, "tilt-provider.json")
	if _, err := os.Stat(pathAndFile); err == nil {
		return pathAndFile, nil
	}
	return "", errors.Errorf("unable to find a tilt-provider.yaml|json file under %s", path)
}

type taskFunction func(ctx context.Context, prefix string, errors chan error)

func sequential(tasks ...taskFunction) taskFunction {
	return func(ctx context.Context, prefix string, errors chan error) {
		for _, task := range tasks {
			task(ctx, prefix, errors)
		}
	}
}

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
	defer cancel()

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
		config, err := config.New(ctx, "")
		if err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed create clusterctl config", prefix)
			return
		}
		cluster := cluster.New(cluster.Kubeconfig{}, config)

		if err := cluster.CertManager().EnsureInstalled(ctx); err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed to install cert-manger", prefix)
		}
	}
}

// chartFile represents a Chart.yaml.
type chartFile struct {
	Version string `json:"version"`
}

// cleanupChartTask ensures we are using the required version of a chart by cleaning up
// outdated charts below the given path. This is necessary because kustomize just
// uses a local Chart if it exists, no matter if it matches the required version or not.
func cleanupChartTask(path string) taskFunction {
	return func(_ context.Context, prefix string, errCh chan error) {
		err := filepath.WalkDir(path, func(path string, _ fs.DirEntry, _ error) error {
			if !strings.HasSuffix(path, "kustomization.yaml") {
				return nil
			}

			// Read kustomization file.
			kustomizationFileBytes, err := os.ReadFile(path) //nolint:gosec
			if err != nil {
				return err
			}

			kustomizationFileContent := types.Kustomization{}
			if err := yaml.Unmarshal(kustomizationFileBytes, &kustomizationFileContent); err != nil {
				return err
			}

			baseDir := filepath.Dir(path)

			// Iterate through helmCharts in the kustomization.yaml and cleanup outdated charts.
			for _, helmChart := range kustomizationFileContent.HelmCharts {
				chartsDir := filepath.Join(baseDir, "charts")
				chartDir := filepath.Join(chartsDir, helmChart.Name)
				chartYAMLPath := filepath.Join(chartDir, "Chart.yaml")

				if _, err := os.Stat(chartYAMLPath); err != nil && errors.Is(err, os.ErrNotExist) {
					// Continue if there is no cached Chart.
					continue
				}

				helmChartVersion := helmChart.Version
				if helmChartVersion == "" {
					return errors.New("Helm Chart version must be set")
				}

				// Read Chart.yaml
				chartFileBytes, err := os.ReadFile(chartYAMLPath) //nolint:gosec
				if err != nil {
					return err
				}
				chartFileContent := chartFile{}
				if err := yaml.Unmarshal(chartFileBytes, &chartFileContent); err != nil {
					return err
				}

				// Remove the local cached Chart if it is outdated.
				if helmChartVersion != chartFileContent.Version {
					klog.Infof("[%s] local Helm Chart is outdated (%s != %s), deleting cached chart...\n", prefix, helmChartVersion, chartFileContent.Version)
					// Delete dir, e.g. hack/observability/visualizer/charts/cluster-api-visualizer
					if err := os.RemoveAll(chartDir); err != nil {
						return err
					}
					// Delete file, e.g. hack/observability/visualizer/charts/cluster-api-visualizer-1.0.0.tgz
					if err := os.Remove(filepath.Join(chartsDir, fmt.Sprintf("%s-%s.tgz", helmChart.Name, chartFileContent.Version))); err != nil {
						return err
					}
				} else {
					klog.Infof("[%s] local Helm Chart already has the right version (%s)\n", prefix, helmChartVersion)
				}
			}

			return nil
		})
		if err != nil {
			errCh <- errors.Wrapf(err, "failed to cleanup Chart dir %q", path)
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

// workloadTask generates a task for creating the component yaml for a workload and saving the output on a file.
// NOTE: This task has several sub steps including running kustomize, envsubst, fixing components for debugging,
// and adding the workload resource mimicking what clusterctl init does.
func workloadTask(name, workloadType, binaryName, containerName string, liveReloadDeps []string, debugConfig *tiltSettingsDebugConfig, extraArgs tiltSettingsExtraArgs, hardCodedProvider bool, path string, options []string, getAdditionalObject func(string, []unstructured.Unstructured) (*unstructured.Unstructured, error)) taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		args := []string{"build"}
		args = append(args, options...)
		args = append(args, path)
		kustomizeCmd := exec.CommandContext(ctx, kustomizePath, args...)
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

		if err := prepareWorkload(prefix, binaryName, containerName, objs, liveReloadDeps, debugConfig, extraArgs, hardCodedProvider); err != nil {
			errCh <- err
			return
		}
		if getAdditionalObject != nil {
			additionalObject, err := getAdditionalObject(prefix, objs)
			if err != nil {
				errCh <- err
				return
			}
			objs = append(objs, *additionalObject)
		}

		for _, o := range objs {
			labels := o.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[clusterctlv1.ClusterctlLabel] = ""
			o.SetLabels(labels)
		}

		yaml, err := utilyaml.FromUnstructured(objs)
		if err != nil {
			errCh <- errors.Wrapf(err, "[%s] failed convert unstructured objects to yaml", prefix)
			return
		}

		if err := writeIfChanged(prefix, filepath.Join(tiltBuildPath, "yaml", fmt.Sprintf("%s.%s.yaml", name, workloadType)), yaml); err != nil {
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

// prepareWorkload sets the Command and Args for the manager container according to the given tiltSettings.
// If there is a debug config given for the workload, we modify Command and Args to work nicely with the delve debugger.
// If there are extra_args given for the workload, we append those to the ones that already exist in the deployment.
// This has the affect that the appended ones will take precedence, as those are read last.
// Finally, we modify the deployment to enable prometheus metrics scraping.
func prepareWorkload(prefix, binaryName, containerName string, objs []unstructured.Unstructured, liveReloadDeps []string, debugConfig *tiltSettingsDebugConfig, extraArgs tiltSettingsExtraArgs, hardcodedProvider bool) error {
	// Update provider namespaces to have the pod security standard enforce label set to privileged.
	// This is required because we remove the SecurityContext from provider deployments below to make tilt work.
	updateNamespacePodSecurityStandard(objs)
	return updateDeployment(prefix, objs, func(deployment *appsv1.Deployment) {
		for j, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name != containerName {
				continue
			}

			cmd := []string{"/" + binaryName}
			if len(liveReloadDeps) > 0 || debugConfig != nil || hardcodedProvider {
				cmd = []string{"sh", "/start.sh", "/" + binaryName}
			}
			args := append(container.Args, []string(extraArgs)...)

			// remove securityContext for tilt live_update, see https://github.com/tilt-dev/tilt/issues/3060
			container.SecurityContext = nil
			// ensure it's also removed from the pod template matching this container
			// setting this outside the loop would means altering every deployments
			deployment.Spec.Template.Spec.SecurityContext = nil

			// alter deployment for working nicely with delve debugger;
			// most specifically, configuring delve, starting the manager with profiling enabled, dropping liveness and
			// readiness probes and disabling leader election.
			if debugConfig != nil {
				cmd = []string{"sh", "/start.sh", "/dlv", "--accept-multiclient", "--api-version=2", "--headless=true", "exec"}

				if debugConfig.Port != nil && *debugConfig.Port > 0 {
					cmd = append(cmd, "--listen=:30000")
				}
				if debugConfig.Continue != nil && *debugConfig.Continue {
					cmd = append(cmd, "--continue")
				}

				cmd = append(cmd, []string{"--", "/" + binaryName}...)

				if debugConfig.ProfilerPort != nil && *debugConfig.ProfilerPort > 0 {
					args = append(args, []string{"--profiler-address=:6060"}...)
				}

				debugArgs := make([]string, 0, len(args))
				for _, a := range args {
					if a == "--leader-elect" || a == "--leader-elect=true" {
						a = "--leader-elect=false"
					}
					debugArgs = append(debugArgs, a)
				}
				args = debugArgs

				// Set annotation to point parca to the profiler port.
				if deployment.Spec.Template.Annotations == nil {
					deployment.Spec.Template.Annotations = map[string]string{}
				}
				deployment.Spec.Template.Annotations["parca.dev/port"] = "8443"

				container.LivenessProbe = nil
				container.ReadinessProbe = nil

				container.Resources = corev1.ResourceRequirements{}
			}

			container.Command = cmd
			container.Args = args
			deployment.Spec.Template.Spec.Containers[j] = container
			deployment.Spec.Replicas = ptr.To[int32](1)
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
		// Ignore Deployments that are not part of the provider, eg. ASO in CAPZ.
		if _, exists := obj.GetLabels()[clusterv1.ProviderNameLabel]; !exists {
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

func getProviderObj(version *string) func(prefix string, objs []unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return func(prefix string, objs []unstructured.Unstructured) (*unstructured.Unstructured, error) {
		namespace := ""
		manifestLabel := ""
		for i := range objs {
			if objs[i].GetKind() != "Namespace" {
				continue
			}

			namespace = objs[i].GetName()
			manifestLabel = objs[i].GetLabels()[clusterv1.ProviderNameLabel]
			break
		}

		if manifestLabel == "" {
			return nil, errors.Errorf(
				"Could not find any Namespace object with label %s and therefore failed to deduce provider name and type",
				clusterv1.ProviderNameLabel)
		}

		providerType := string(clusterctlv1.CoreProviderType)
		providerName := manifestLabel
		if strings.HasPrefix(manifestLabel, "infrastructure-") {
			providerType = string(clusterctlv1.InfrastructureProviderType)
			providerName = manifestLabel[len("infrastructure-"):]
		}
		if strings.HasPrefix(manifestLabel, "bootstrap-") {
			providerType = string(clusterctlv1.BootstrapProviderType)
			providerName = manifestLabel[len("bootstrap-"):]
		}
		if strings.HasPrefix(manifestLabel, "control-plane-") {
			providerType = string(clusterctlv1.ControlPlaneProviderType)
			providerName = manifestLabel[len("control-plane-"):]
		}
		if strings.HasPrefix(manifestLabel, "ipam-") {
			providerType = string(clusterctlv1.IPAMProviderType)
			providerName = manifestLabel[len("ipam-"):]
		}
		if strings.HasPrefix(manifestLabel, "runtime-extension-") {
			providerType = string(clusterctlv1.RuntimeExtensionProviderType)
			providerName = manifestLabel[len("runtime-extension-"):]
		}
		if strings.HasPrefix(manifestLabel, "addon-") {
			providerType = string(clusterctlv1.AddonProviderType)
			providerName = manifestLabel[len("addon-"):]
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
					clusterv1.ProviderNameLabel:      manifestLabel,
					clusterctlv1.ClusterctlLabel:     "",
					clusterctlv1.ClusterctlCoreLabel: clusterctlv1.ClusterctlCoreLabelInventoryValue,
				},
			},
			ProviderName: providerName,
			Type:         providerType,
			Version:      ptr.Deref(version, defaultProviderVersion),
		}

		providerObj := &unstructured.Unstructured{}
		if err := scheme.Scheme.Convert(provider, providerObj, nil); err != nil {
			return nil, errors.Wrapf(err, "[%s] failed to convert Provider to unstructured", prefix)
		}
		return providerObj, nil
	}
}

func updateNamespacePodSecurityStandard(objs []unstructured.Unstructured) {
	for i, obj := range objs {
		if obj.GetKind() != "Namespace" {
			continue
		}
		// Ignore Deployments that are not part of the provider, eg. ASO in CAPZ.
		if _, exists := obj.GetLabels()[clusterv1.ProviderNameLabel]; !exists {
			continue
		}
		labels := obj.GetLabels()
		labels["pod-security.kubernetes.io/enforce"] = "privileged"
		obj.SetLabels(labels)
		objs[i] = obj
	}
}
