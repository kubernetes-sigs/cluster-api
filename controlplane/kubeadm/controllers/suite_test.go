/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"os"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/internal/envtest"
	ctrl "sigs.k8s.io/controller-runtime"
	// +kubebuilder:scaffold:imports
)

var (
	env *envtest.Environment
	ctx = ctrl.SetupSignalHandler()
	// TODO(sbueringer): move under internal/testtypes (or refactor it in a way that we don't need it anymore).
	fakeGenericMachineTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "genericmachinetemplate.generic.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "generic.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "GenericMachineTemplate",
			},
		},
	}
)

func TestMain(m *testing.M) {
	os.Exit(envtest.Run(ctx, envtest.RunInput{
		M:        m,
		SetupEnv: func(e *envtest.Environment) { env = e },
	}))
}
