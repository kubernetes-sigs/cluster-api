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

package google

import (
	"fmt"
	"os/exec"
)

const (
	MachineControllerServiceAccount = "k8s-machine-controller"
	MachineControllerSecret         = "machine-controller-credential"
)

// Creates a GCP service account for the machine controller, granted the
// permissions to manage compute instances, and stores its credentials as a
// Kubernetes secret.
func CreateMachineControllerServiceAccount(projects []string) error {
	// TODO: use real go bindings

	// The service account needs to be created in a single project, so just
	// use the first one, but grant permission to all projects in the list.
	project := projects[0]

	err := run("gcloud", "--project", project, "iam", "service-accounts", "create", "--display-name=k8s machines controller", MachineControllerServiceAccount)
	if err != nil {
		return fmt.Errorf("couldn't create service account: %v", err)
	}

	email := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", MachineControllerServiceAccount, project)
	localFile := MachineControllerServiceAccount + "-key.json"

	for _, project := range projects {
		err = run("gcloud", "projects", "add-iam-policy-binding", project, "--member=serviceAccount:"+email, "--role=roles/compute.instanceAdmin.v1")
		if err != nil {
			return fmt.Errorf("couldn't grant permissions to service account: %v", err)
		}
	}

	err = run("gcloud", "--project", project, "iam", "service-accounts", "keys", "create", localFile, "--iam-account", email)
	if err != nil {
		return fmt.Errorf("couldn't create service account key: %v", err)
	}

	err = run("kubectl", "create", "secret", "generic", "-n", "kube-system", MachineControllerSecret, "--from-file=service-account.json="+localFile)
	if err != nil {
		return fmt.Errorf("couldn't import service account key as credential: %v", err)
	}

	return nil
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	return nil
}
