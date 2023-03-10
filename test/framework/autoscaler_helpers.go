/*
Copyright 2023 The Kubernetes Authors.

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
	"bytes"
	"context"
	b64 "encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

// ApplyAutoscalerToWorkloadClusterInput is the input for ApplyAutoscalerToWorkloadCluster.
type ApplyAutoscalerToWorkloadClusterInput struct {
	ClusterctlConfigPath string

	// info about where autoscaler yaml are
	// Note:
	//  - the file creating the service account to be used by the autoscaler when connecting to the management cluster
	//    - must be named "autoscaler-to-workload-management.yaml"
	//    - must deploy objects in the $CLUSTER_NAMESPACE
	//    - must create a service account with name "cluster-$CLUSTER_NAME" and the RBAC rules required to work.
	//    - must create a secret with name "cluster-$CLUSTER_NAME-token" and type "kubernetes.io/service-account-token".
	//  - the file creating the autoscaler deployment in the workload cluster
	//    - must be named "autoscaler-to-workload-workload.yaml"
	//    - must deploy objects in the cluster-autoscaler-system namespace
	//    - must create a deployment named "cluster-autoscaler"
	//    - must run the autoscaler with --cloud-provider=clusterapi,
	//      --node-group-auto-discovery=clusterapi:namespace=${CLUSTER_NAMESPACE},clusterName=${CLUSTER_NAME}
	//      and --cloud-config pointing to a kubeconfig to connect to the management cluster
	//      using the token above.
	//    - could use following vars to build the management cluster kubeconfig:
	//      $MANAGEMENT_CLUSTER_TOKEN, $MANAGEMENT_CLUSTER_CA, $MANAGEMENT_CLUSTER_ADDRESS
	ArtifactFolder         string
	InfrastructureProvider string
	LatestProviderVersion  string
	AutoscalerVersion      string

	ManagementClusterProxy ClusterProxy
	Cluster                *clusterv1.Cluster
	WorkloadClusterProxy   ClusterProxy
}

// ApplyAutoscalerToWorkloadCluster apply the autoscaler to the workload cluster.
// Please note that it also create a service account in the workload cluster for the autoscaler to use.
func ApplyAutoscalerToWorkloadCluster(ctx context.Context, input ApplyAutoscalerToWorkloadClusterInput, intervals ...interface{}) {
	By("Creating the service account to be used by the autoscaler when connecting to the management cluster")

	managementYamlPath := filepath.Join(input.ArtifactFolder, "repository", fmt.Sprintf("infrastructure-%s", input.InfrastructureProvider), input.LatestProviderVersion, "autoscaler-to-workload-management.yaml")
	managementYamlTemplate, err := os.ReadFile(managementYamlPath) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "failed to load %s", managementYamlTemplate)

	managementYaml, err := ProcessYAML(&ProcessYAMLInput{
		Template:             managementYamlTemplate,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Env: map[string]string{
			"CLUSTER_NAMESPACE": input.Cluster.Namespace,
			"CLUSTER_NAME":      input.Cluster.Name,
		},
	})
	Expect(err).ToNot(HaveOccurred(), "failed to parse %s", managementYamlTemplate)
	Expect(input.ManagementClusterProxy.Apply(ctx, managementYaml)).To(Succeed(), "failed to apply %s", managementYamlTemplate)

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: input.Cluster.Namespace,
			Name:      fmt.Sprintf("cluster-%s-token", input.Cluster.Name),
		},
	}
	Eventually(func() bool {
		err := input.ManagementClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret)
		if err != nil {
			return false
		}
		if _, ok := tokenSecret.Data["token"]; !ok {
			return false
		}
		if _, ok := tokenSecret.Data["ca.crt"]; !ok {
			return false
		}
		return true
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "failed to get token for the autoscaler service account")

	By("Creating the autoscaler deployment in the workload cluster")

	workloadYamlPath := filepath.Join(input.ArtifactFolder, "repository", fmt.Sprintf("infrastructure-%s", input.InfrastructureProvider), input.LatestProviderVersion, "autoscaler-to-workload-workload.yaml")
	workloadYamlTemplate, err := os.ReadFile(workloadYamlPath) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "failed to load %s", workloadYamlTemplate)

	serverAddr := input.ManagementClusterProxy.GetRESTConfig().Host
	// On CAPD, if not running on Linux, we need to use Docker's proxy to connect back to the host
	// to the CAPD cluster. Moby on Linux doesn't use the host.docker.internal DNS name.
	if runtime.GOOS != "linux" {
		serverAddr = strings.ReplaceAll(serverAddr, "127.0.0.1", "host.docker.internal")
	}

	workloadYaml, err := ProcessYAML(&ProcessYAMLInput{
		Template:             workloadYamlTemplate,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Env: map[string]string{
			"CLUSTER_NAMESPACE":          input.Cluster.Namespace,
			"CLUSTER_NAME":               input.Cluster.Name,
			"MANAGEMENT_CLUSTER_TOKEN":   string(tokenSecret.Data["token"]),
			"MANAGEMENT_CLUSTER_CA":      b64.StdEncoding.EncodeToString(tokenSecret.Data["ca.crt"]),
			"MANAGEMENT_CLUSTER_ADDRESS": serverAddr,
			"AUTOSCALER_VERSION":         input.AutoscalerVersion,
		},
	})
	Expect(err).ToNot(HaveOccurred(), "failed to parse %s", workloadYamlTemplate)
	Expect(input.WorkloadClusterProxy.Apply(ctx, workloadYaml)).To(Succeed(), "failed to apply %s", workloadYamlTemplate)

	By("Wait for the autoscaler deployment and collect logs")
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-autoscaler",
			Namespace: "cluster-autoscaler-system",
		},
	}

	if err := input.WorkloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment); apierrors.IsNotFound(err) {
		WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
			Getter:     input.WorkloadClusterProxy.GetClient(),
			Deployment: deployment,
		}, intervals...)

		// Start streaming logs from all controller providers
		WatchDeploymentLogsByName(ctx, WatchDeploymentLogsByNameInput{
			GetLister:  input.WorkloadClusterProxy.GetClient(),
			ClientSet:  input.WorkloadClusterProxy.GetClientSet(),
			Deployment: deployment,
			LogPath:    filepath.Join(input.ArtifactFolder, "clusters", input.WorkloadClusterProxy.GetName(), "logs", deployment.GetNamespace()),
		})
	}
}

// AddScaleUpDeploymentAndWaitInput is the input for AddScaleUpDeploymentAndWait.
type AddScaleUpDeploymentAndWaitInput struct {
	ClusterProxy ClusterProxy
}

// AddScaleUpDeploymentAndWait create a deployment that will trigger the autoscaler to scale up and create a new machine.
func AddScaleUpDeploymentAndWait(ctx context.Context, input AddScaleUpDeploymentAndWaitInput, intervals ...interface{}) {
	By("Create a scale up deployment requiring 2G memory for each replica")

	// gets the node size
	nodes := &corev1.NodeList{}
	workers := 0
	Expect(input.ClusterProxy.GetClient().List(ctx, nodes)).To(Succeed(), "failed to list nodes")
	var memory *resource.Quantity
	for _, n := range nodes.Items {
		if _, ok := n.Labels[nodeRoleControlPlane]; ok {
			continue
		}
		memory = n.Status.Capacity.Memory()
		workers++
	}
	Expect(workers).To(Equal(1), "AddScaleUpDeploymentAndWait assumes only one worker node exist")
	Expect(memory).ToNot(BeNil(), "failed to get memory for the worker node")

	// creates a deployment requesting more memory than the worker has, thus triggering autoscaling
	var replicas int64 = 5
	podMemory := resource.NewQuantity(memory.Value()/(replicas-1), resource.BinarySI)

	scalelUpDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scale-up",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"app": "scale-up",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(int32(replicas)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "scale-up",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "scale-up",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "busybox",
							Image: "busybox",
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceMemory: *podMemory,
								},
							},
							Command: []string{"/bin/sh", "-c", "echo \"up\" & sleep infinity"},
						},
					},
				},
			},
		},
	}

	By("Scale up deployment created")
	Expect(input.ClusterProxy.GetClient().Create(ctx, scalelUpDeployment)).To(Succeed(), "failed to create the scale up pod")

	By("Wait for the scale up deployment to become ready (this implies machines to be created)")
	WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
		Getter:     input.ClusterProxy.GetClient(),
		Deployment: scalelUpDeployment,
	}, intervals...)
}

type ProcessYAMLInput struct {
	Template             []byte
	ClusterctlConfigPath string
	Env                  map[string]string
}

func ProcessYAML(input *ProcessYAMLInput) ([]byte, error) {
	for n, v := range input.Env {
		_ = os.Setenv(n, v)
	}

	c, err := clusterctlclient.New(input.ClusterctlConfigPath)
	if err != nil {
		return nil, err
	}
	options := clusterctlclient.ProcessYAMLOptions{
		ReaderSource: &clusterctlclient.ReaderSourceOptions{
			Reader: bytes.NewReader(input.Template),
		},
	}

	printer, err := c.ProcessYAML(options)
	if err != nil {
		return nil, err
	}

	out, err := printer.Yaml()
	if err != nil {
		return nil, err
	}

	return out, nil
}
