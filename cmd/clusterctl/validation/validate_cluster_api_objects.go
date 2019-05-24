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
	"context"
	"fmt"
	"io"

	"github.com/openshift/cluster-api/pkg/apis/cluster/common"
	"github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1 "github.com/openshift/cluster-api/pkg/apis/cluster/v1alpha1"
	"github.com/openshift/cluster-api/pkg/controller/noderefutil"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ValidateClusterAPIObjects(ctx context.Context, w io.Writer, c client.Client, clusterName string, namespace string) error {
	fmt.Fprintf(w, "Validating Cluster API objects in namespace %q\n", namespace)

	cluster, err := getClusterObject(ctx, c, clusterName, namespace)
	if err != nil {
		return err
	}
	if err := validateClusterObject(w, cluster); err != nil {
		return err
	}

	machines := &clusterv1alpha1.MachineList{}
	if err := c.List(ctx, machines, client.InNamespace(namespace)); err != nil {
		return errors.Wrapf(err, "failed to get the machines from the apiserver in namespace %q", namespace)
	}

	return validateMachineObjects(ctx, w, machines, c)
}

func getClusterObject(ctx context.Context, c client.Reader, clusterName string, namespace string) (*v1alpha1.Cluster, error) {
	if clusterName != "" {
		cluster := &clusterv1alpha1.Cluster{}
		err := c.Get(ctx, types.NamespacedName{Name: clusterName, Namespace: namespace}, cluster)
		return cluster, err
	}

	clusters := &clusterv1alpha1.ClusterList{}
	if err := c.List(ctx, clusters, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "failed to get the clusters from the apiserver in namespace %q", namespace)
	}

	if numOfClusters := len(clusters.Items); numOfClusters == 0 {
		return nil, errors.Errorf("fail: No cluster exists in namespace %q", namespace)
	} else if numOfClusters > 1 {
		return nil, errors.Errorf("fail: There is more than one cluster in namespace %q. Please specify --cluster-name", namespace)
	}
	return &clusters.Items[0], nil
}

func validateClusterObject(w io.Writer, cluster *v1alpha1.Cluster) error {
	fmt.Fprintf(w, "Checking cluster object %q... ", cluster.Name)
	if cluster.Status.ErrorReason != "" || cluster.Status.ErrorMessage != "" {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\t[%v]: %s\n", cluster.Status.ErrorReason, cluster.Status.ErrorMessage)
		return errors.Errorf("cluster %q failed the validation", cluster.Name)
	}
	fmt.Fprintf(w, "PASS\n")
	return nil
}

func validateMachineObjects(ctx context.Context, w io.Writer, machines *v1alpha1.MachineList, client client.Client) error {
	pass := true
	for _, machine := range machines.Items {
		if !validateMachineObject(ctx, w, machine, client) {
			pass = false
		}
	}
	if !pass {
		return errors.Errorf("machine objects failed the validation")
	}
	return nil
}

func validateMachineObject(ctx context.Context, w io.Writer, machine v1alpha1.Machine, client client.Client) bool {
	fmt.Fprintf(w, "Checking machine object %q... ", machine.Name)
	if machine.Status.ErrorReason != nil || machine.Status.ErrorMessage != nil {
		var reason common.MachineStatusError
		if machine.Status.ErrorReason != nil {
			reason = *machine.Status.ErrorReason
		}
		var message string
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
	if err := validateReferredNode(ctx, machine.Status.NodeRef.Name, client); err != nil {
		fmt.Fprintf(w, "FAIL\n")
		fmt.Fprintf(w, "\t%v\n", err)
		return false
	}
	fmt.Fprintf(w, "PASS\n")
	return true
}

func validateReferredNode(ctx context.Context, nodeName string, client client.Client) error {
	node := &corev1.Node{}
	if err := client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		return errors.Wrapf(err, "the corresponding node %q is not found", nodeName)
	}
	if !noderefutil.IsNodeReady(node) {
		return errors.Errorf("the corresponding node %q is not ready", nodeName)
	}
	return nil
}
