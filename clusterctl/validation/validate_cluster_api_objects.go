/*
Copyright 2018 The Kubernetes Authors.

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

package validation

import (
	"fmt"
	"io"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	"sigs.k8s.io/cluster-api/pkg/controller/noderefutil"
)

func ValidateClusterAPIObjects(w io.Writer, clusterApiClient *clientset.Clientset, k8sClient kubernetes.Interface, clusterName string, namespace string) error {
	fmt.Fprintf(w, "Validating Cluster API objects in namespace %q\n", namespace)

	cluster, err := getClusterObject(clusterApiClient, clusterName, namespace)
	if err != nil {
		return err
	}
	if err := validateClusterObject(w, cluster); err != nil {
		return err
	}

	machines, err := clusterApiClient.ClusterV1alpha1().Machines(namespace).List(meta_v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to get the machines from the apiserver in namespace %q: %v", namespace, err)
	}

	return validateMachineObjects(w, machines, k8sClient)
}

func getClusterObject(clusterApiClient *clientset.Clientset, clusterName string, namespace string) (*v1alpha1.Cluster, error) {
	if clusterName != "" {
		cluster, err := clusterApiClient.ClusterV1alpha1().Clusters(namespace).Get(clusterName, meta_v1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get the cluster %q from the apiserver in namespace %q: %v", clusterName, namespace, err)
		}
		return cluster, nil
	}

	clusters, err := clusterApiClient.ClusterV1alpha1().Clusters(namespace).List(meta_v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get the clusters from the apiserver in namespace %q: %v", namespace, err)
	}
	if numOfClusters := len(clusters.Items); numOfClusters == 0 {
		return nil, fmt.Errorf("fail: No cluster exists in namespace %q.", namespace)
	} else if numOfClusters > 1 {
		return nil, fmt.Errorf("fail: There is more than one cluster in namespace %q. Please specify --cluster-name.", namespace)
	}
	return &clusters.Items[0], nil
}

func validateClusterObject(w io.Writer, cluster *v1alpha1.Cluster) error {
	fmt.Fprintf(w, "Checking cluster object %q... ", cluster.Name)
	if cluster.Status.ErrorReason != "" || cluster.Status.ErrorMessage != "" {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\t[%v]: %s\n", cluster.Status.ErrorReason, cluster.Status.ErrorMessage)
		return fmt.Errorf("Cluster %q failed the validation.", cluster.Name)
	}
	fmt.Fprintf(w, "PASS\n")
	return nil
}

func validateMachineObjects(w io.Writer, machines *v1alpha1.MachineList, k8sClient kubernetes.Interface) error {
	pass := true
	for _, machine := range machines.Items {
		if (!validateMachineObject(w, machine, k8sClient)) {
			pass = false
		}
	}
	if !pass {
		return fmt.Errorf("Machine objects failed the validation.")
	}
	return nil
}

func validateMachineObject(w io.Writer, machine v1alpha1.Machine, k8sClient kubernetes.Interface) bool {
	fmt.Fprintf(w, "Checking machine object %q... ", machine.Name)
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		var reason common.MachineStatusError = ""
		if machine.Status.ErrorReason != nil {
			reason = *machine.Status.ErrorReason
		}
		var message string = ""
		if machine.Status.ErrorMessage != nil {
			message = *machine.Status.ErrorMessage
		}
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\t[%v]: %s\n", reason, message)
		return false
	}
	if machine.Status.NodeRef == nil {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\tThe corresponding node is missing.\n")
		return false
	}
	err := validateReferredNode(w, machine.Status.NodeRef.Name, k8sClient)
	if err != nil {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\t%v\n", err)
		return false
	}
	fmt.Fprintf(w, "PASS\n")
	return true
}

func validateReferredNode(w io.Writer, nodeName string, k8sClient kubernetes.Interface) error {
	node, err := k8sClient.CoreV1().Nodes().Get(nodeName, meta_v1.GetOptions{})
	if err != nil {
		return fmt.Errorf("The corresponding node %q is not found: %v", nodeName, err)
	}
	if !noderefutil.IsNodeReady(node) {
		return fmt.Errorf("The corresponding node %q is not ready.", nodeName)
	}
	return nil
}
