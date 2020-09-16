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

package setup

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	. "github.com/onsi/gomega"
	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
	"sigs.k8s.io/cluster-api/test/framework/kubernetesversions"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"

	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// Test suite constants for e2e config variables
const (
	CNIPath      = "CNI"
	CNIResources = "CNI_RESOURCES"
)

func InitScheme() *runtime.Scheme {
	sc := runtime.NewScheme()
	framework.TryAddDefaultSchemes(sc)
	return sc
}

func LoadE2EConfig(configPath string) *clusterctl.E2EConfig {
	config := clusterctl.LoadE2EConfig(context.TODO(), clusterctl.LoadE2EConfigInput{ConfigPath: configPath})
	Expect(config).ToNot(BeNil(), "Failed to load E2E config from %s", configPath)

	// Read CNI file and set CNI_RESOURCES environmental variable
	Expect(config.Variables).To(HaveKey(CNIPath), "Missing %s variable in the config", CNIPath)
	clusterctl.SetCNIEnvVar(config.GetVariable(CNIPath), CNIResources)

	return config
}

func CreateClusterctlLocalRepository(config *clusterctl.E2EConfig, repositoryFolder string) string {
	clusterctlConfig := clusterctl.CreateRepository(context.TODO(), clusterctl.CreateRepositoryInput{
		E2EConfig:        config,
		RepositoryFolder: repositoryFolder,
	})
	Expect(clusterctlConfig).To(BeAnExistingFile(), "The clusterctl config file does not exists in the local repository %s", repositoryFolder)
	return clusterctlConfig
}

type CreateBootstrapClusterInput struct {
	E2EConfig          *clusterctl.E2EConfig
	Scheme             *runtime.Scheme
	UseExistingCluster bool
}

func CreateCIArtifactsTemplate(artifactFolder string, srcFolder string, e2eConfig *clusterctl.E2EConfig) string {
	template, err := ioutil.ReadFile(srcFolder + "/data/infrastructure-docker/cluster-template.yaml")
	Expect(err).NotTo(HaveOccurred())

	platformKustomization, err := ioutil.ReadFile(srcFolder + "/data/infrastructure-docker/platform-kustomization.yaml")
	Expect(err).NotTo(HaveOccurred())

	ciTemplate, err := kubernetesversions.GenerateCIArtifactsInjectedTemplateForDebian(kubernetesversions.GenerateCIArtifactsInjectedTemplateForDebianInput{
		ArtifactsDirectory:    artifactFolder,
		SourceTemplate:        template,
		PlatformKustomization: platformKustomization,
	})

	Expect(err).NotTo(HaveOccurred())
	for i, p := range e2eConfig.Providers {
		if p.Name != "docker" {
			continue
		}
		e2eConfig.Providers[i].Files = append(e2eConfig.Providers[i].Files, clusterctl.Files{
			SourcePath: ciTemplate,
			TargetName: "cluster-template-with-ci-artifacts.yaml",
		})
	}
	return ciTemplate
}

func CreateBootstrapCluster(input CreateBootstrapClusterInput) (bootstrap.ClusterProvider, framework.ClusterProxy) {
	var clusterProvider bootstrap.ClusterProvider
	kubeconfigPath := ""
	Expect(input.E2EConfig).ToNot(BeNil(), "E2EConfig must be provided")
	Expect(input.Scheme).ToNot(BeNil(), "Scheme must be provided")
	if !input.UseExistingCluster {
		clusterProvider = bootstrap.CreateKindBootstrapClusterAndLoadImages(context.TODO(), bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:               input.E2EConfig.ManagementClusterName,
			RequiresDockerSock: input.E2EConfig.HasDockerProvider(),
			Images:             input.E2EConfig.Images,
		})
		Expect(clusterProvider).ToNot(BeNil(), "Failed to create a bootstrap cluster")

		kubeconfigPath = clusterProvider.GetKubeconfigPath()
		Expect(kubeconfigPath).To(BeAnExistingFile(), "Failed to get the kubeconfig file for the bootstrap cluster")
	}

	clusterProxy := framework.NewClusterProxy("bootstrap", kubeconfigPath, input.Scheme)
	Expect(clusterProxy).ToNot(BeNil(), "Failed to get a bootstrap cluster proxy")

	return clusterProvider, clusterProxy
}

type InitBootstrapClusterInput struct {
	BootstrapClusterProxy framework.ClusterProxy
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfig      string
	ArtifactsDirectory    string
}

func InitBootstrapCluster(input InitBootstrapClusterInput) {
	input.ArtifactsDirectory = framework.ResolveArtifactsDirectory(input.ArtifactsDirectory)
	Expect(input.E2EConfig).ToNot(BeNil(), "E2EConfig must be provided")
	Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "BootstrapClusterProxy must be provided")

	clusterctl.InitManagementClusterAndWatchControllerLogs(context.TODO(), clusterctl.InitManagementClusterAndWatchControllerLogsInput{
		ClusterProxy:            input.BootstrapClusterProxy,
		ClusterctlConfigPath:    input.ClusterctlConfig,
		InfrastructureProviders: input.E2EConfig.InfrastructureProviders(),
		LogFolder:               filepath.Join(input.ArtifactsDirectory, "clusters", input.BootstrapClusterProxy.GetName()),
	}, input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-controllers")...)
}

func TearDown(bootstrapClusterProvider bootstrap.ClusterProvider, bootstrapClusterProxy framework.ClusterProxy) {
	if bootstrapClusterProxy != nil {
		bootstrapClusterProxy.Dispose(context.TODO())
	}
	if bootstrapClusterProvider != nil {
		bootstrapClusterProvider.Dispose(context.TODO())
	}
}

type CreateSpecNamespaceInput struct {
	SpecName           string
	ClusterProxy       framework.ClusterProxy
	ArtifactsDirectory string
}

func CreateSpecNamespace(ctx context.Context, input CreateSpecNamespaceInput) (*corev1.Namespace, context.CancelFunc) {
	name := fmt.Sprintf("%s-%s", input.SpecName, util.RandomString(6))
	log := Log.WithValues("spec-name", input.SpecName, "namespace", name)
	log.Info("Creating a namespace for hosting the %q test spec", input.SpecName)
	namespace, cancelWatches := framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
		Creator:   input.ClusterProxy.GetClient(),
		ClientSet: input.ClusterProxy.GetClientSet(),
		Name:      name,
		LogFolder: filepath.Join(framework.ResolveArtifactsDirectory(input.ArtifactsDirectory), "clusters", input.ClusterProxy.GetName()),
	})

	return namespace, cancelWatches
}

type DumpSpecResourcesAndCleanupInput struct {
	SpecName           string
	ClusterProxy       framework.ClusterProxy
	ArtifactsDirectory string
	Namespace          *corev1.Namespace
	CancelWatches      context.CancelFunc
	Cluster            *clusterv1.Cluster
	IntervalsGetter    func(spec, key string) []interface{}
	SkipCleanup        bool
}

func DumpSpecResourcesAndCleanup(ctx context.Context, input DumpSpecResourcesAndCleanupInput) {
	Expect(input.ClusterProxy).ToNot(BeNil(), "ClusterProxy must be provided")
	Expect(input.Namespace).ToNot(BeNil(), "Namespace must be provided")
	Expect(input.IntervalsGetter).ToNot(BeNil(), "IntervalsGetter must be provided")
	logPath := filepath.Join(framework.ResolveArtifactsDirectory(input.ArtifactsDirectory), "clusters", input.ClusterProxy.GetName(), "resources")
	log := Log.WithValues(
		"namespace", input.Namespace.Name,
		"cluster-namespace", input.Cluster.Namespace,
		"cluster-name", input.Cluster.Name,
		"spec-name", input.SpecName,
		"log-path", logPath,
	)

	// Dump all the logs from the workload cluster before deleting them.
	input.ClusterProxy.CollectWorkloadClusterLogs(ctx,
		input.Cluster.Namespace,
		input.Cluster.Name,
		filepath.Join(input.ArtifactsDirectory, "clusters", input.Cluster.Name, "machines"))

	log.Info("Dumping all the Cluster API resources in namespace")
	// Dump all Cluster API related resources to artifacts before deleting them.
	framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
		Lister:    input.ClusterProxy.GetClient(),
		Namespace: input.Namespace.Name,
		LogPath:   logPath,
	})

	if !input.SkipCleanup {
		log.Info("Deleting cluster")
		// While https://github.com/kubernetes-sigs/cluster-api/issues/2955 is addressed in future iterations, there is a chance
		// that cluster variable is not set even if the cluster exists, so we are calling DeleteAllClustersAndWait
		// instead of DeleteClusterAndWait
		framework.DeleteAllClustersAndWait(ctx, framework.DeleteAllClustersAndWaitInput{
			Client:    input.ClusterProxy.GetClient(),
			Namespace: input.Namespace.Name,
		}, input.IntervalsGetter(input.SpecName, "wait-delete-cluster")...)

		log.Info("Deleting namespace used for hosting the test spec")
		framework.DeleteNamespace(ctx, framework.DeleteNamespaceInput{
			Deleter: input.ClusterProxy.GetClient(),
			Name:    input.Namespace.Name,
		})
	}
	input.CancelWatches()
}
