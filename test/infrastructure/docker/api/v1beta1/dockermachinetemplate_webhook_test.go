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

package v1beta1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDockerMachineTemplateInvalid(t *testing.T) {
	oldTemplate := DockerMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{},
		Spec: DockerMachineTemplateSpec{
			Template: DockerMachineTemplateResource{},
		},
	}

	newTemplate := oldTemplate.DeepCopy()
	newTemplate.Spec.Template.Spec.ExtraMounts = append(newTemplate.Spec.Template.Spec.ExtraMounts, []Mount{{ContainerPath: "/var/run/docker.sock", HostPath: "/var/run/docker.sock"}}...)

	tests := []struct {
		name        string
		newTemplate *DockerMachineTemplate
		oldTemplate *DockerMachineTemplate
		wantError   bool
	}{
		{
			name:        "return no error if no modification",
			newTemplate: newTemplate,
			oldTemplate: newTemplate,
			wantError:   false,
		},
		{
			name:        "don't allow modification",
			newTemplate: newTemplate,
			oldTemplate: &oldTemplate,
			wantError:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.newTemplate.ValidateUpdate(tt.oldTemplate)
			if (err != nil) != tt.wantError {
				t.Errorf("unexpected result - wanted %+v, got %+v", tt.wantError, err)
			}
		})
	}
}
