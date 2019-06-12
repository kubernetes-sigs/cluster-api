// Copyright 2019 VMware, Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gitlab.com/chuckh/cluster-api-provider-kind/kind/actions"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Here we go!")
	clusterName := "my-cluster"
	for {
		// read input
		text, _ := reader.ReadString('\n')
		cleanText := strings.TrimSpace(text)
		inputs := strings.Split(cleanText, " ")
		switch inputs[0] {
		case "new-cluster":
			fmt.Println("Creating load balancer")
			lb, err := actions.SetUpLoadBalancer(clusterName)
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			ip, err := lb.IP()
			if err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
			if _, err := actions.CreateControlPlane(clusterName, ip); err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
		case "add-worker":
			if _, err := actions.AddWorker(clusterName); err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
		case "delete-node":
			if len(inputs) < 2 {
				fmt.Println("usage: delete-node my-cluster-worker1")
				continue
			}
			fmt.Println("Warning: If you are deleting a control plane node your cluster may break.")
			if err := actions.DeleteNode(clusterName, inputs[1]); err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
		case "add-control-plane":
			if _, err := actions.AddControlPlane(clusterName); err != nil {
				panic(fmt.Sprintf("%+v", err))
			}
		case "set-cluster-name":
			fmt.Println("setting cluster name...")
			clusterName = inputs[1]
		default:
			fmt.Println("Unknown command")
		}
		fmt.Println("Done!")
	}
}

func getName(clusterName, role string) string {
	ns, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.ClusterLabelKey, clusterName),
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, role))
	if err != nil {
		panic(err)
	}
	count := len(ns)
	suffix := fmt.Sprintf("%d", count)
	if count == 0 {
		suffix = ""
	}
	return fmt.Sprintf("%s-%s%s", clusterName, role, suffix)
}
