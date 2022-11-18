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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"helm.sh/helm/v3/pkg/repo"
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
	"sigs.k8s.io/kustomize/api/types"

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
*/

var (
	rootPath             string
	tiltBuildPath        string
	tiltSettingsFileFlag = pflag.String("tilt-settings-file", "./tilt-settings.yaml", "Path to a tilt-settings.(json|yaml) file")
	toolsFlag            = pflag.StringSlice("tools", []string{}, "list of tools to be created; each value should correspond to a make target")
)

type tiltSettings struct {
	Debug               map[string]debugConfig `json:"debug,omitempty"`
	ExtraArgs           map[string]extraArgs   `json:"extra_args,omitempty"`
	DeployCertManager   *bool                  `json:"deploy_cert_manager,omitempty"`
	DeployObservability []string               `json:"deploy_observability,omitempty"`
	EnableProviders     []string               `json:"enable_providers,omitempty"`
	AllowedContexts     []string               `json:"allowed_contexts,omitempty"`
	ProviderRepos       []string               `json:"provider_repos,omitempty"`
}

type providerSettings struct {
	Name   string          `json:"name,omitempty"`
	Config *providerConfig `json:"config,omitempty"`
}

type providerConfig struct {
	Context *string `json:"context,omitempty"`
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

	setDefaults(ts)
	return ts, nil
}

// setDefaults sets default values for debug related fields in tiltSettings.
func setDefaults(ts *tiltSettings) {
	if ts.DeployCertManager == nil {
		ts.DeployCertManager = pointer.Bool(true)
	}

	for k := range ts.Debug {
		p := ts.Debug[k]
		if p.Continue == nil {
			p.Continue = pointer.Bool(true)
		}
		if p.Port == nil {
			p.Port = pointer.Int(0)
		}
		if p.ProfilerPort == nil {
			p.ProfilerPort = pointer.Int(0)
		}
		if p.MetricsPort == nil {
			p.MetricsPort = pointer.Int(0)
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

	allowed := sets.NewString(ts.AllowedContexts...)
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
		tasks["cert-manager-cainjector"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-cainjector:%s", config.CertManagerDefaultVersion))
		tasks["cert-manager-webhook"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-webhook:%s", config.CertManagerDefaultVersion))
		tasks["cert-manager-controller"] = preLoadImageTask(fmt.Sprintf("quay.io/jetstack/cert-manager-controller:%s", config.CertManagerDefaultVersion))
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

	providerPaths := map[string]string{"core": ".",
		"kubeadm-bootstrap":     "bootstrap/kubeadm",
		"kubeadm-control-plane": "controlplane/kubeadm",
		"docker":                "test/infrastructure/docker",
		"test-extension":        "test/extension",
	}

	// Add all the provider paths to the providerpaths map
	for _, p := range ts.ProviderRepos {
		providerContexts, err := loadProviders(p)
		if err != nil {
			return err
		}

		for name, path := range providerContexts {
			providerPaths[name] = path
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
		path, ok := providerPaths[providerName]
		if !ok {
			return errors.Errorf("failed to obtain path for the provider %s", providerName)
		}
		tasks[providerName] = workloadTask(providerName, "provider", "manager", "manager", ts, fmt.Sprintf("%s/config/default", path), getProviderObj)
	}

	return runTaskGroup(ctx, "resources", tasks)
}

func loadProviders(r string) (map[string]string, error) {
	var contextPath string
	providerData, err := readProviderSettings(r)
	if err != nil {
		return nil, err
	}

	providerContexts := map[string]string{}
	for _, p := range providerData {
		if p.Config != nil && p.Config.Context != nil {
			contextPath = r + "/" + *p.Config.Context
		} else {
			contextPath = r
		}
		providerContexts[p.Name] = contextPath
	}
	return providerContexts, nil
}

func readProviderSettings(path string) ([]providerSettings, error) {
	path, err := checkWorkloadFileFormat(path, "provider")
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}

	ps := []providerSettings{}
	// providerSettings file can be an array and this is done to detect arrays
	if strings.HasPrefix(string(content), "[") || strings.HasPrefix(string(content), "-") {
		if err := yaml.Unmarshal(content, &ps); err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to read provider path %s tilt file", path))
		}
	} else {
		p := providerSettings{}
		if err := yaml.Unmarshal(content, &p); err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("failed to read provider path %s tilt file", path))
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func checkWorkloadFileFormat(path, workloadType string) (string, error) {
	if _, err := os.Stat(path + fmt.Sprintf("/tilt-%s.yaml", workloadType)); err == nil {
		return path + fmt.Sprintf("/tilt-%s.yaml", workloadType), nil
	}
	if _, err := os.Stat(path + fmt.Sprintf("/tilt-%s.json", workloadType)); err == nil {
		return path + fmt.Sprintf("/tilt-%s.json", workloadType), nil
	}
	return "", fmt.Errorf("unable to find a tilt %s file under %s", workloadType, path)
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

// chartFile represents a Chart.yaml.
type chartFile struct {
	Version string `json:"version"`
}

// cleanupChartTask ensures we are using the required version of a chart by cleaning up
// outdated charts below the given path. This is necessary because kustomize just
// uses a local Chart if it exists, no matter if it matches the required version or not.
func cleanupChartTask(path string) taskFunction {
	return func(ctx context.Context, prefix string, errCh chan error) {
		err := filepath.WalkDir(path, func(path string, d fs.DirEntry, _ error) error {
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
					// If helmChart.Version is not set lookup the latest version.
					helmChartVersion, err = resolveLatestHelmChartVersion(ctx, helmChart.Repo, helmChart.Name)
					if err != nil {
						return err
					}
					klog.Infof("[%s] resolved latest Helm Chart version to %q\n", prefix, helmChartVersion)
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

// resolveLatestHelmChartVersion resolves the latest version of a Helm Chart with name chartName
// from a given chartRepo.
func resolveLatestHelmChartVersion(ctx context.Context, chartRepo, chartName string) (string, error) {
	chartIndexURL := fmt.Sprintf(chartRepo + "/index.yaml")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, chartIndexURL, http.NoBody)
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve latest Helm Chart version for Chart %q: failed to create request for chart index from URL %q", chartName, chartIndexURL)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve latest Helm Chart version for Chart %q: failed to get chart index from URL %q", chartName, chartIndexURL)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve latest Helm Chart version for Chart %q: failed to parse chart index from URL %q", chartName, chartIndexURL)
	}

	indexFile := &repo.IndexFile{}
	if err := yaml.UnmarshalStrict(bodyBytes, indexFile); err != nil {
		return "", errors.Wrapf(err, "failed to resolve latest Helm Chart version for Chart %q: failed to unmarshal chart index from URL %q", chartName, chartIndexURL)
	}
	// Sort entries to ensure the latest version is on top.
	// This is required to get the latest version.
	indexFile.SortEntries()

	chartVersion, err := indexFile.Get(chartName, "")
	if err != nil {
		return "", errors.Wrapf(err, "failed to resolve latest Helm Chart version for Chart %q: failed to get latest version from chart index from URL %q", chartName, chartIndexURL)
	}
	return chartVersion.Version, nil
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
func workloadTask(name, workloadType, binaryName, containerName string, ts *tiltSettings, path string, getAdditionalObject func(string, []unstructured.Unstructured) (*unstructured.Unstructured, error)) taskFunction {
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

		if err := prepareWorkload(name, prefix, binaryName, containerName, objs, ts); err != nil {
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
			labels[clusterctlv1.ClusterctlLabelName] = ""
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
func prepareWorkload(name, prefix, binaryName, containerName string, objs []unstructured.Unstructured, ts *tiltSettings) error {
	return updateDeployment(prefix, objs, func(d *appsv1.Deployment) {
		for j, container := range d.Spec.Template.Spec.Containers {
			if container.Name != containerName {
				continue
			}
			cmd := []string{"sh", "/start.sh", "/" + binaryName}
			args := append(container.Args, []string(ts.ExtraArgs[name])...)

			// alter deployment for working nicely with delve debugger;
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

				cmd = append(cmd, []string{"--", "/" + binaryName}...)

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

	if manifestLabel == "" {
		return nil, errors.Errorf(
			"Could not find any Namespace object with label %s and therefore failed to deduce provider name and type",
			clusterv1.ProviderLabelName)
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
		Version:      "v1.3.99",
	}

	providerObj := &unstructured.Unstructured{}
	if err := scheme.Scheme.Convert(provider, providerObj, nil); err != nil {
		return nil, errors.Wrapf(err, "[%s] failed to convert Provider to unstructured", prefix)
	}
	return providerObj, nil
}
