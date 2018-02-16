/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"os"
	"os/user"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

func homeDirOrPanic() string {
	user, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("Couldn't get user home directory: %v", err.Error()))
	}
	return user.HomeDir
}

func machinesClient(kubeconfig string) (v1alpha1.MachineInterface, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	c, err := v1alpha1.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return c.Machines(v1.NamespaceDefault), nil
}

func die(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}
