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

package google

import (
	"fmt"
	"os/exec"

	"github.com/golang/glog"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	gceconfigv1 "sigs.k8s.io/cluster-api/cloud/google/gceproviderconfig/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/util"
)

const (
	MasterNodeServiceAccountPrefix        = "k8s-master"
	WorkerNodeServiceAccountPrefix        = "k8s-worker"
	IngressControllerServiceAccountPrefix = "k8s-ingress-controller"
	MachineControllerServiceAccountPrefix = "k8s-machine-controller"

	IngressControllerSecret = "glbc-gcp-key"
	MachineControllerSecret = "machine-controller-credential"

	ClusterAnnotationPrefix = "gce.clusterapi.k8s.io/service-account-"
)

var (
	MasterNodeRoles = []string{
		"compute.instanceAdmin",
		"compute.networkAdmin",
		"compute.securityAdmin",
		"compute.viewer",
		"iam.serviceAccountUser",
		"storage.admin",
		"storage.objectViewer",
	}
	WorkerNodeRoles        = []string{}
	IngressControllerRoles = []string{
		"compute.instanceAdmin.v1",
		"compute.networkAdmin",
		"compute.securityAdmin",
		"iam.serviceAccountActor",
	}
	MachineControllerRoles = []string{
		"compute.instanceAdmin.v1",
		"compute.securityAdmin",
		"iam.serviceAccountActor",
	}
)

type ServiceAccountService struct {
	gceProviderConfigCodec   *gceconfigv1.GCEProviderConfigCodec
}

func NewServiceAccountService(codec *gceconfigv1.GCEProviderConfigCodec) *ServiceAccountService {
	return &ServiceAccountService{
		gceProviderConfigCodec: codec,
	}
}

// Returns the email address of the service account that should be used
// as the default service account for this machine
func (sas *ServiceAccountService) GetDefaultServiceAccountForMachine(cluster *clusterv1.Cluster, machine *clusterv1.Machine) string {
	if util.IsMaster(machine) {
		return cluster.ObjectMeta.Annotations[ClusterAnnotationPrefix+MasterNodeServiceAccountPrefix]
	} else {
		return cluster.ObjectMeta.Annotations[ClusterAnnotationPrefix+WorkerNodeServiceAccountPrefix]
	}
}

// Creates a GCP service account for the master node, granted permissions
// that allow the control plane to provision disks and networking resources
func (sas *ServiceAccountService) CreateMasterNodeServiceAccount(cluster *clusterv1.Cluster) error {
	_, _, err := sas.createServiceAccount(MasterNodeServiceAccountPrefix, MasterNodeRoles, cluster)

	return err
}

// Creates a GCP service account for the worker node
func (sas *ServiceAccountService) CreateWorkerNodeServiceAccount(cluster *clusterv1.Cluster) error {
	_, _, err := sas.createServiceAccount(WorkerNodeServiceAccountPrefix, WorkerNodeRoles, cluster)

	return err
}

// Creates a GCP service account for the ingress controller
func (sas *ServiceAccountService) CreateIngressControllerServiceAccount(cluster *clusterv1.Cluster) error {
	accountId, project, err := sas.createServiceAccount(IngressControllerServiceAccountPrefix, IngressControllerRoles, cluster)
	if err != nil {
		return err
	}

	return sas.createSecretForServiceAccountKey(accountId, project, IngressControllerSecret, "kube-system")
}

// Creates a GCP service account for the machine controller, granted the
// permissions to manage compute instances, and stores its credentials as a
// Kubernetes secret.
func (sas *ServiceAccountService) CreateMachineControllerServiceAccount(cluster *clusterv1.Cluster) error {
	accountId, project, err := sas.createServiceAccount(MachineControllerServiceAccountPrefix, MachineControllerRoles, cluster)
	if err != nil {
		return err
	}

	return sas.createSecretForServiceAccountKey(accountId, project, MachineControllerSecret, "default")
}

func (sas *ServiceAccountService) createSecretForServiceAccountKey(accountId string, project string, secretName string, namespace string) error {
	email := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountId, project)

	localFile := accountId + "-key.json"
	err := run("gcloud", "--project", project, "iam", "service-accounts", "keys", "create", localFile, "--iam-account", email)
	if err != nil {
		return fmt.Errorf("couldn't create service account key: %v", err)
	}

	err = run("kubectl", "create", "secret", "generic", secretName, "--from-file=service-account.json="+localFile, "--namespace="+namespace)
	if err != nil {
		return fmt.Errorf("couldn't import service account key as credential: %v", err)
	}

	if err := run("rm", localFile); err != nil {
		glog.Error(err)
	}

	return nil
}

// creates a service account with the roles specifed. Returns the account id
// of the created account and the project it belongs to.
func (sas *ServiceAccountService) createServiceAccount(serviceAccountPrefix string, roles []string, cluster *clusterv1.Cluster) (string, string, error) {

	config, err := sas.gceProviderConfigCodec.ClusterProviderFromProviderConfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return "", "", err
	}

	accountId := serviceAccountPrefix + "-" + util.RandomString(5)

	err = run("gcloud", "--project", config.Project, "iam", "service-accounts", "create", "--display-name="+serviceAccountPrefix+" service account", accountId)
	if err != nil {
		return "", "", fmt.Errorf("couldn't create service account: %v", err)
	}

	email := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountId, config.Project)

	for _, role := range roles {
		err = run("gcloud", "projects", "add-iam-policy-binding", config.Project, "--member=serviceAccount:"+email, "--role=roles/"+role)
		if err != nil {
			return "", "", fmt.Errorf("couldn't grant permissions to service account: %v", err)
		}
	}

	if cluster.ObjectMeta.Annotations == nil {
		cluster.ObjectMeta.Annotations = make(map[string]string)
	}
	cluster.ObjectMeta.Annotations[ClusterAnnotationPrefix+serviceAccountPrefix] = email

	return accountId, config.Project, nil
}

func (sas *ServiceAccountService) DeleteMasterNodeServiceAccount(cluster *clusterv1.Cluster) error {
	return sas.deleteServiceAccount(MasterNodeServiceAccountPrefix, MasterNodeRoles, cluster)
}

func (sas *ServiceAccountService) DeleteWorkerNodeServiceAccount(cluster *clusterv1.Cluster) error {
	return sas.deleteServiceAccount(WorkerNodeServiceAccountPrefix, WorkerNodeRoles, cluster)
}

func (sas *ServiceAccountService) DeleteIngressControllerServiceAccount(cluster *clusterv1.Cluster) error {
	return sas.deleteServiceAccount(IngressControllerServiceAccountPrefix, IngressControllerRoles, cluster)
}

func (sas *ServiceAccountService) DeleteMachineControllerServiceAccount(cluster *clusterv1.Cluster) error {
	return sas.deleteServiceAccount(MachineControllerServiceAccountPrefix, MachineControllerRoles, cluster)
}

func (sas *ServiceAccountService) deleteServiceAccount(serviceAccountPrefix string, roles []string, cluster *clusterv1.Cluster) error {

	config, err := sas.gceProviderConfigCodec.ClusterProviderFromProviderConfig(cluster.Spec.ProviderConfig)
	if err != nil {
		glog.Info("cannot parse cluster providerConfig field")
		return nil
	}

	var email string
	if cluster.ObjectMeta.Annotations != nil {
		email = cluster.ObjectMeta.Annotations[ClusterAnnotationPrefix+serviceAccountPrefix]
	}

	if email == "" {
		glog.Info("No service a/c found in cluster.")
		return nil
	}

	for _, role := range roles {
		err = run("gcloud", "projects", "remove-iam-policy-binding", config.Project, "--member=serviceAccount:"+email, "--role=roles/"+role)
	}

	if err != nil {
		return fmt.Errorf("couldn't remove permissions to service account: %v", err)
	}

	err = run("gcloud", "--project", config.Project, "iam", "service-accounts", "delete", email)
	if err != nil {
		return fmt.Errorf("couldn't delete service account: %v", err)
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
