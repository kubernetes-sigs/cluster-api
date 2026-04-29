/*
Copyright 2026 The Kubernetes Authors.

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

package internal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetTransformedPod gets a corev1.Pod and returns a transformed Pod with only the subset of fields that we use.
func GetTransformedPod(ctx context.Context, c client.Client, podKey client.ObjectKey) (*Pod, error) {
	pod := &corev1.Pod{}
	if err := c.Get(ctx, podKey, pod); err != nil {
		return nil, err
	}

	return TransformPod(pod), nil
}

// GetTransformedNode gets a corev1.Node and returns a transformed Node with only the subset of fields that we use.
func GetTransformedNode(ctx context.Context, c client.Client, nodeKey client.ObjectKey) (*Node, error) {
	node := &corev1.Node{}
	if err := c.Get(ctx, nodeKey, node); err != nil {
		return nil, err
	}

	return TransformNode(node), nil
}

// ListTransformedNodes lists corev1.Nodes and returns transformed Nodes with only the subset of fields that we use.
func ListTransformedNodes(ctx context.Context, c client.Client, opts ...client.ListOption) ([]*Node, error) {
	nodeList := &corev1.NodeList{}
	if err := c.List(ctx, nodeList, opts...); err != nil {
		return nil, err
	}

	nodes := make([]*Node, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		nodes = append(nodes, TransformNode(&node))
	}
	return nodes, nil
}

// ObjectMeta is the subset of metav1.ObjectMeta that we use.
type ObjectMeta struct {
	Name              string
	Namespace         string
	CreationTimestamp metav1.Time
	Labels            map[string]string
	Annotations       map[string]string
}

// Pod is the subset of corev1.Pod that we use.
type Pod struct {
	ObjectMeta
	Status corev1.PodStatus
}

// TransformPod transforms a corev1.Pod to a Pod with only the subset of fields that we use
// Note: This func *must* be aligned to our cache configuration in: controlplane/kubeadm/internal/setup/setup.go.
func TransformPod(pod *corev1.Pod) *Pod {
	return &Pod{
		ObjectMeta: ObjectMeta{
			Name:              pod.Name,
			Namespace:         pod.Namespace,
			CreationTimestamp: pod.CreationTimestamp,
			Labels:            pod.Labels,
			Annotations:       pod.Annotations,
		},
		Status: pod.Status,
	}
}

// Node is the subset of corev1.Node that we use.
type Node struct {
	ObjectMeta
	Spec   NodeSpec
	Status NodeStatus
}

// NodeSpec is the subset of corev1.NodeSpec that we use.
type NodeSpec struct {
	ProviderID string
	Taints     []corev1.Taint
}

// NodeStatus is the subset of corev1.NodeStatus that we use.
type NodeStatus struct {
	Conditions []corev1.NodeCondition
}

// TransformNode transforms a corev1.Node to a Node with only the subset of fields that we use
// Note: This func *must* be aligned to our cache configuration in: controlplane/kubeadm/internal/setup/setup.go.
func TransformNode(node *corev1.Node) *Node {
	return &Node{
		ObjectMeta: ObjectMeta{
			Name:              node.Name,
			Namespace:         node.Namespace,
			CreationTimestamp: node.CreationTimestamp,
			Labels:            node.Labels,
			Annotations:       node.Annotations,
		},
		Spec: NodeSpec{
			ProviderID: node.Spec.ProviderID,
			Taints:     node.Spec.Taints,
		},
		Status: NodeStatus{
			Conditions: node.Status.Conditions,
		},
	}
}
