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
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/test/framework/internal/log"
	"sigs.k8s.io/cluster-api/util/patch"
)

// ApplyAutoscalerToWorkloadClusterInput is the input for ApplyAutoscalerToWorkloadCluster.
type ApplyAutoscalerToWorkloadClusterInput struct {
	ClusterctlConfigPath string

	ArtifactFolder                    string
	InfrastructureMachineTemplateKind string
	// WorkloadYamlPath should point the yaml that will be applied on the workload cluster.
	// The YAML file should:
	//  - Be creating the autoscaler deployment in the workload cluster
	//    - must deploy objects in the cluster-autoscaler-system namespace
	//    - must create a deployment named "cluster-autoscaler"
	//    - must create a ServiceAccount, ClusterRole, ClusterRoleBinding needed by the autoscaler deployment
	//    - must run the autoscaler with --cloud-provider=clusterapi,
	//      --node-group-auto-discovery=clusterapi:namespace=${CLUSTER_NAMESPACE},clusterName=${CLUSTER_NAME}
	//      and --cloud-config pointing to a kubeconfig to connect to the management cluster
	//      using a token.
	//    - could use following vars to build the management cluster kubeconfig:
	//      - $MANAGEMENT_CLUSTER_TOKEN
	//      - $MANAGEMENT_CLUSTER_ADDRESS
	//      - $MANAGEMENT_CLUSTER_CA
	//      - $AUTOSCALER_VERSION
	//      - $CLUSTER_NAMESPACE
	WorkloadYamlPath  string
	AutoscalerVersion string

	ManagementClusterProxy ClusterProxy
	Cluster                *clusterv1.Cluster
	WorkloadClusterProxy   ClusterProxy
}

// ApplyAutoscalerToWorkloadCluster installs autoscaler on the workload cluster.
// Installs autoscaler by doing the following:
// - Create a token on the management cluster with the correct RBAC needed by autoscaler
// - Applies the workload YAML after processing the yaml (template processing) to the workload cluster.
// The autoscaler deployment on the workload cluster uses a kubeconfig based on the token created in the above step to
// access the management cluster.
func ApplyAutoscalerToWorkloadCluster(ctx context.Context, input ApplyAutoscalerToWorkloadClusterInput, intervals ...interface{}) {
	By("Creating the autoscaler deployment in the workload cluster")

	workloadYamlTemplate, err := os.ReadFile(input.WorkloadYamlPath)
	Expect(err).ToNot(HaveOccurred(), "failed to load %s", workloadYamlTemplate)

	// Get a server address for the Management Cluster.
	// This address should be accessible from the workload cluster.
	serverAddr, mgtClusterCA := getServerAddrAndCA(ctx, input.ManagementClusterProxy)
	// Generate a token with the required permission that can be used by the autoscaler.
	token := getAuthenticationTokenForAutoscaler(ctx, input.ManagementClusterProxy, input.Cluster.Namespace, input.Cluster.Name, input.InfrastructureMachineTemplateKind)

	workloadYaml, err := ProcessYAML(&ProcessYAMLInput{
		Template:             workloadYamlTemplate,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Env: map[string]string{
			"CLUSTER_NAMESPACE":          input.Cluster.Namespace,
			"CLUSTER_NAME":               input.Cluster.Name,
			"MANAGEMENT_CLUSTER_TOKEN":   token,
			"MANAGEMENT_CLUSTER_ADDRESS": serverAddr,
			"MANAGEMENT_CLUSTER_CA":      base64.StdEncoding.EncodeToString(mgtClusterCA),
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

	Expect(input.WorkloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed(), fmt.Sprintf("failed to get Deployment %s", klog.KObj(deployment)))
	WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
		Getter:     input.WorkloadClusterProxy.GetClient(),
		Deployment: deployment,
	}, intervals...)

	// Start streaming logs from the autoscaler deployment.
	WatchDeploymentLogsByName(ctx, WatchDeploymentLogsByNameInput{
		GetLister:  input.WorkloadClusterProxy.GetClient(),
		Cache:      input.WorkloadClusterProxy.GetCache(ctx),
		ClientSet:  input.WorkloadClusterProxy.GetClientSet(),
		Deployment: deployment,
		LogPath:    filepath.Join(input.ArtifactFolder, "clusters", input.WorkloadClusterProxy.GetName(), "logs", deployment.GetNamespace()),
	})
}

// AddScaleUpDeploymentAndWaitInput is the input for AddScaleUpDeploymentAndWait.
type AddScaleUpDeploymentAndWaitInput struct {
	ClusterProxy ClusterProxy
}

// AddScaleUpDeploymentAndWait create a deployment that will trigger the autoscaler to scale up and create a new machine.
func AddScaleUpDeploymentAndWait(ctx context.Context, input AddScaleUpDeploymentAndWaitInput, intervals ...interface{}) {
	By("Create a scale up deployment with resource requests to force scale up")

	// gets the node size
	nodes := &corev1.NodeList{}
	workers := 0
	Expect(input.ClusterProxy.GetClient().List(ctx, nodes)).To(Succeed(), "failed to list nodes")
	var memory *resource.Quantity
	for _, n := range nodes.Items {
		if _, ok := n.Labels[nodeRoleControlPlane]; ok {
			continue
		}
		if _, ok := n.Labels[nodeRoleOldControlPlane]; ok {
			continue
		}
		memory = n.Status.Capacity.Memory() // Assume that all nodes have the same memory.
		workers++
	}
	Expect(memory).ToNot(BeNil(), "failed to get memory for the worker node")

	// creates a deployment requesting more memory than the worker has, thus triggering autoscaling
	// Each pod should requests memory resource of about 60% of the node capacity so that at most one pod
	// fits on each node. Setting a replicas of workers + 1 would ensure we have pods that cannot be scheduled.
	// This will force exactly one extra node to be spun up.
	replicas := workers + 1
	memoryRequired := int64(float64(memory.Value()) * 0.6)
	podMemory := resource.NewQuantity(memoryRequired, resource.BinarySI)

	scaleUpDeployment := &appsv1.Deployment{
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

	By("Create scale up deployment")
	Expect(input.ClusterProxy.GetClient().Create(ctx, scaleUpDeployment)).To(Succeed(), "failed to create the scale up pod")

	By("Wait for the scale up deployment to become ready (this implies machines to be created)")
	WaitForDeploymentsAvailable(ctx, WaitForDeploymentsAvailableInput{
		Getter:     input.ClusterProxy.GetClient(),
		Deployment: scaleUpDeployment,
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

type DisableAutoscalerForMachineDeploymentTopologyAndWaitInput struct {
	ClusterProxy                  ClusterProxy
	Cluster                       *clusterv1.Cluster
	WaitForAnnotationsToBeDropped []interface{}
}

// DisableAutoscalerForMachineDeploymentTopologyAndWait drop the autoscaler annotations from the MachineDeploymentTopology
// and waits till the annotations are dropped from the underlying MachineDeployment. It also verifies that the replicas
// fields of the MachineDeployments are not affected after the annotations are dropped.
func DisableAutoscalerForMachineDeploymentTopologyAndWait(ctx context.Context, input DisableAutoscalerForMachineDeploymentTopologyAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DisableAutoscalerForMachineDeploymentTopologyAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling DisableAutoscalerForMachineDeploymentTopologyAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	// Get the current replicas of the MachineDeployments.
	replicas := map[string]*int32{}
	mdList := GetMachineDeploymentsByCluster(ctx, GetMachineDeploymentsByClusterInput{
		Lister:      mgmtClient,
		ClusterName: input.Cluster.Name,
		Namespace:   input.Cluster.Namespace,
	})
	for _, md := range mdList {
		replicas[md.Name] = md.Spec.Replicas
	}

	log.Logf("Dropping the %s and %s annotations from the MachineDeployments in ClusterTopology", clusterv1.AutoscalerMinSizeAnnotation, clusterv1.AutoscalerMaxSizeAnnotation)
	patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create patch helper for Cluster %s", klog.KObj(input.Cluster)))
	for i := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
		md := input.Cluster.Spec.Topology.Workers.MachineDeployments[i]
		delete(md.Metadata.Annotations, clusterv1.AutoscalerMinSizeAnnotation)
		delete(md.Metadata.Annotations, clusterv1.AutoscalerMaxSizeAnnotation)
		input.Cluster.Spec.Topology.Workers.MachineDeployments[i] = md
	}
	Eventually(func(g Gomega) {
		g.Expect(patchHelper.Patch(ctx, input.Cluster)).Should(Succeed())
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch Cluster topology to drop autoscaler annotations")

	log.Logf("Wait for the annotations to be dropped from the MachineDeployments")
	Eventually(func(g Gomega) {
		mdList := GetMachineDeploymentsByCluster(ctx, GetMachineDeploymentsByClusterInput{
			Lister:      mgmtClient,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		for i := range mdList {
			md := mdList[i]
			g.Expect(md.Annotations).ToNot(HaveKey(clusterv1.AutoscalerMinSizeAnnotation), fmt.Sprintf("MachineDeployment %s should not have %s annotation", klog.KObj(md), clusterv1.AutoscalerMinSizeAnnotation))
			g.Expect(md.Annotations).ToNot(HaveKey(clusterv1.AutoscalerMaxSizeAnnotation), fmt.Sprintf("MachineDeployment %s should not have %s annotation", klog.KObj(md), clusterv1.AutoscalerMaxSizeAnnotation))
			// Verify that disabling auto scaler does not change the current MachineDeployment replicas.
			g.Expect(md.Spec.Replicas).To(Equal(replicas[md.Name]), fmt.Sprintf("MachineDeployment %s replicas should not change after disabling autoscaler", klog.KObj(md)))
		}
	}, input.WaitForAnnotationsToBeDropped...).Should(Succeed(), "Auto scaler annotations are not dropped or replicas changed for the MachineDeployments")
}

type EnableAutoscalerForMachineDeploymentTopologyAndWaitInput struct {
	ClusterProxy                ClusterProxy
	Cluster                     *clusterv1.Cluster
	NodeGroupMinSize            string
	NodeGroupMaxSize            string
	WaitForAnnotationsToBeAdded []interface{}
}

func EnableAutoscalerForMachineDeploymentTopologyAndWait(ctx context.Context, input EnableAutoscalerForMachineDeploymentTopologyAndWaitInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for EnableAutoscalerForMachineDeploymentTopologyAndWait")
	Expect(input.ClusterProxy).ToNot(BeNil(), "Invalid argument. input.ClusterProxy can't be nil when calling EnableAutoscalerForMachineDeploymentTopologyAndWait")

	mgmtClient := input.ClusterProxy.GetClient()

	log.Logf("Add the %s and %s annotations to the MachineDeployments in ClusterTopology", clusterv1.AutoscalerMinSizeAnnotation, clusterv1.AutoscalerMaxSizeAnnotation)
	patchHelper, err := patch.NewHelper(input.Cluster, mgmtClient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create patch helper for Cluster %s", klog.KObj(input.Cluster)))
	for i := range input.Cluster.Spec.Topology.Workers.MachineDeployments {
		md := input.Cluster.Spec.Topology.Workers.MachineDeployments[i]
		if md.Metadata.Annotations == nil {
			md.Metadata.Annotations = map[string]string{}
		}
		// Add the autoscaler annotation
		md.Metadata.Annotations[clusterv1.AutoscalerMinSizeAnnotation] = input.NodeGroupMinSize
		md.Metadata.Annotations[clusterv1.AutoscalerMaxSizeAnnotation] = input.NodeGroupMaxSize
		// Drop the replicas from MachineDeploymentTopology, or else the topology controller and autoscaler with fight over control.
		md.Replicas = nil
		input.Cluster.Spec.Topology.Workers.MachineDeployments[i] = md
	}
	Eventually(func(g Gomega) {
		g.Expect(patchHelper.Patch(ctx, input.Cluster)).Should(Succeed())
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to patch Cluster topology to add autoscaler annotations")

	log.Logf("Wait for the annotations to applied on the MachineDeployments")
	Eventually(func(g Gomega) {
		mdList := GetMachineDeploymentsByCluster(ctx, GetMachineDeploymentsByClusterInput{
			Lister:      mgmtClient,
			ClusterName: input.Cluster.Name,
			Namespace:   input.Cluster.Namespace,
		})
		for i := range mdList {
			md := mdList[i]
			g.Expect(md.Annotations).To(HaveKey(clusterv1.AutoscalerMinSizeAnnotation), fmt.Sprintf("MachineDeployment %s should have %s annotation", klog.KObj(md), clusterv1.AutoscalerMinSizeAnnotation))
			g.Expect(md.Annotations).To(HaveKey(clusterv1.AutoscalerMaxSizeAnnotation), fmt.Sprintf("MachineDeployment %s should have %s annotation", klog.KObj(md), clusterv1.AutoscalerMaxSizeAnnotation))
		}
	}, input.WaitForAnnotationsToBeAdded...).Should(Succeed(), "Auto scaler annotations are missing from the MachineDeployments")
}

// getAuthenticationTokenForAutoscaler returns a bearer authenticationToken with minimal RBAC permissions that will be used
// by the autoscaler running on the workload cluster to access the management cluster.
func getAuthenticationTokenForAutoscaler(ctx context.Context, managementClusterProxy ClusterProxy, namespace string, cluster string, infraMachineTemplateKind string) string {
	name := fmt.Sprintf("cluster-%s", cluster)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	Expect(managementClusterProxy.GetClient().Create(ctx, sa)).To(Succeed(), fmt.Sprintf("failed to create %s service account", name))

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"get", "list", "update", "watch"},
				APIGroups: []string{"cluster.x-k8s.io"},
				Resources: []string{"machinedeployments", "machinedeployments/scale", "machinepools", "machinepools/scale", "machines", "machinesets"},
			},
			{
				Verbs:     []string{"get", "list"},
				APIGroups: []string{"infrastructure.cluster.x-k8s.io"},
				Resources: []string{infraMachineTemplateKind},
			},
		},
	}
	Expect(managementClusterProxy.GetClient().Create(ctx, role)).To(Succeed(), fmt.Sprintf("failed to create %s role", name))

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				APIGroup:  "",
				Name:      name,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     name,
		},
	}
	Expect(managementClusterProxy.GetClient().Create(ctx, roleBinding)).To(Succeed(), fmt.Sprintf("failed to create %s role binding", name))

	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: pointer.Int64(2 * 60 * 60), // 2 hours.
		},
	}
	tokenRequest, err := managementClusterProxy.GetClientSet().CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, tokenRequest, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "failed to create token for Autoscaler")

	return tokenRequest.Status.Token
}

// getServerAddrAndCA returns the address to be used for accessing the management cluster from a workload cluster.
func getServerAddrAndCA(ctx context.Context, clusterProxy ClusterProxy) (string, []byte) {
	// With CAPD, we can't just access the bootstrap cluster via 127.0.0.1:<port> from the
	// workload cluster. Instead we retrieve the server name from the cluster-info ConfigMap in the bootstrap
	// cluster (e.g. "https://test-z45p9k-control-plane:6443")
	// Note: This has been tested with MacOS,Linux and Prow.
	clusterInfoCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-info",
			Namespace: metav1.NamespacePublic,
		},
	}
	Expect(clusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(clusterInfoCM), clusterInfoCM)).To(Succeed(), fmt.Sprintf("failed to get ConfigMap %s", klog.KObj(clusterInfoCM)))
	Expect(clusterInfoCM.Data).To(HaveKey("kubeconfig"), fmt.Sprintf("ConfigMap %s data is missing \"kubeconfig\"", klog.KObj(clusterInfoCM)))

	kubeConfigString := clusterInfoCM.Data["kubeconfig"]

	kubeConfig, err := clientcmd.Load([]byte(kubeConfigString))
	Expect(err).ToNot(HaveOccurred(), "failed to parse KubeConfig information")

	return kubeConfig.Clusters[""].Server, kubeConfig.Clusters[""].CertificateAuthorityData
}
