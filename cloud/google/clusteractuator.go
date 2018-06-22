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
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/cluster-api/cloud/google/clients"
	"sigs.k8s.io/cluster-api/cloud/google/clients/errors"
	gceconfigv1 "sigs.k8s.io/cluster-api/cloud/google/gceproviderconfig/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
)

const (
	firewallRuleAnnotationPrefix = "gce.clusterapi.k8s.io/firewall"
	firewallRuleInternalSuffix   = "-allow-cluster-internal"
	firewallRuleApiSuffix        = "-allow-api-public"

	createClusterEventAction = "Create"
	deleteClusterEventAction = "Delete"
)

type GCEClusterClient struct {
	computeService         GCEClientComputeService
	clusterClient          client.ClusterInterface
	gceProviderConfigCodec *gceconfigv1.GCEProviderConfigCodec
	eventRecorder          record.EventRecorder
}

type ClusterActuatorParams struct {
	ComputeService GCEClientComputeService
	ClusterClient  client.ClusterInterface
	EventRecorder  record.EventRecorder
}

func NewClusterActuator(params ClusterActuatorParams) (*GCEClusterClient, error) {
	computeService, err := getOrNewComputeServiceForCluster(params)
	if err != nil {
		return nil, err
	}
	codec, err := gceconfigv1.NewCodec()
	if err != nil {
		return nil, err
	}

	return &GCEClusterClient{
		computeService:         computeService,
		clusterClient:          params.ClusterClient,
		gceProviderConfigCodec: codec,
		eventRecorder:          params.EventRecorder,
	}, nil
}

func (gce *GCEClusterClient) Reconcile(cluster *clusterv1.Cluster) error {
	glog.Infof("Reconciling cluster %v.", cluster.Name)
	created := gce.createClusterIfNeeded(cluster)
	if created {
		gce.eventRecorder.Eventf(cluster, corev1.EventTypeNormal, "Created", "Created Cluster %v", cluster.Name)
	}
	return nil
}

func (gce *GCEClusterClient) Delete(cluster *clusterv1.Cluster) error {
	err := gce.deleteFirewallRule(cluster, cluster.Name+firewallRuleInternalSuffix)
	if err != nil {
		gce.fireErrorEvent(cluster, err, deleteClusterEventAction)
		return gce.handleError(cluster, apierrors.DeleteCluster("Error deleting firewall rule for internal cluster traffic: %v", err), deleteClusterEventAction)
	}
	if err := gce.deleteFirewallRule(cluster, cluster.Name+firewallRuleApiSuffix); err != nil {
		gce.fireErrorEvent(cluster, err, deleteClusterEventAction)
		return gce.handleError(cluster, apierrors.DeleteCluster("Error deleting firewall rule for core api server traffic: %v", err), deleteClusterEventAction)
	}

	gce.eventRecorder.Eventf(cluster, corev1.EventTypeNormal, "Deleted", "Deleted Cluster %v", cluster.Name)

	return nil
}

// Creates cluster resources if they don't already exist. Returns true if any
// resources were successfully created for this cluster.
func (gce *GCEClusterClient) createClusterIfNeeded(cluster *clusterv1.Cluster) bool {
	createdInternalRule, err := gce.createFirewallRuleIfNotExists(cluster, &compute.Firewall{
		Name:    cluster.Name + firewallRuleInternalSuffix,
		Network: "global/networks/default",
		Allowed: []*compute.FirewallAllowed{
			{
				IPProtocol: "tcp",
			},
		},
		TargetTags: []string{cluster.Name + "-worker"},
		SourceTags: []string{cluster.Name + "-worker"},
	})
	if err != nil {
		gce.fireErrorEvent(cluster, err, createClusterEventAction)
		glog.Warningf("Error creating firewall rule for internal cluster traffic: %v", err)
	}
	createdApiRule, err := gce.createFirewallRuleIfNotExists(cluster, &compute.Firewall{
		Name:    cluster.Name + firewallRuleApiSuffix,
		Network: "global/networks/default",
		Allowed: []*compute.FirewallAllowed{
			{
				IPProtocol: "tcp",
				Ports:      []string{"443"},
			},
		},
		TargetTags:   []string{"https-server"},
		SourceRanges: []string{"0.0.0.0/0"},
	})
	if err != nil {
		gce.fireErrorEvent(cluster, err, createClusterEventAction)
		glog.Warningf("Error creating firewall rule for core api server traffic: %v", err)
	}

	return createdInternalRule || createdApiRule
}

func getOrNewComputeServiceForCluster(params ClusterActuatorParams) (GCEClientComputeService, error) {
	if params.ComputeService != nil {
		return params.ComputeService, nil
	}
	client, err := google.DefaultClient(context.TODO(), compute.ComputeScope)
	if err != nil {
		return nil, err
	}
	computeService, err := clients.NewComputeService(client)
	if err != nil {
		return nil, err
	}
	return computeService, nil
}

func (gce *GCEClusterClient) createFirewallRuleIfNotExists(cluster *clusterv1.Cluster, firewallRule *compute.Firewall) (bool, error) {
	ruleExistsAnnotation, ok := cluster.ObjectMeta.Annotations[firewallRuleAnnotationPrefix+firewallRule.Name]
	if ok && ruleExistsAnnotation == "true" {
		// The firewall rule was already created.
		return false, nil
	}
	clusterConfig, err := gce.clusterproviderconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return false, gce.handleError(cluster, apierrors.InvalidClusterConfiguration("Error parsing cluster provider config: %v", err), createClusterEventAction)
	}
	firewallRules, err := gce.computeService.FirewallsGet(clusterConfig.Project)
	if err != nil {
		return false, gce.handleError(cluster, apierrors.CreateCluster("Error getting firewall rules: %v", err), createClusterEventAction)
	}

	gceContainsRule := gce.containsFirewallRule(firewallRules, firewallRule.Name)
	if !gceContainsRule {
		op, err := gce.computeService.FirewallsInsert(clusterConfig.Project, firewallRule)
		if err != nil {
			return false, gce.handleError(cluster, apierrors.CreateCluster("Error creating firewall rule: %v", err), createClusterEventAction)
		}
		err = gce.computeService.WaitForOperation(clusterConfig.Project, op)
		if err != nil {
			return false, gce.handleError(cluster, apierrors.CreateCluster("Error waiting for firewall rule creation: %v", err), createClusterEventAction)
		}
	}
	// TODO (mkjelland) move this to a GCEClusterProviderStatus #347
	if cluster.ObjectMeta.Annotations == nil {
		cluster.ObjectMeta.Annotations = make(map[string]string)
	}
	cluster.ObjectMeta.Annotations[firewallRuleAnnotationPrefix+firewallRule.Name] = "true"
	_, err = gce.clusterClient.Update(cluster)
	if err != nil {
		fmt.Errorf("error updating cluster annotations %v", err)
	}
	return !gceContainsRule, nil
}

func (gce *GCEClusterClient) containsFirewallRule(firewallRules *compute.FirewallList, ruleName string) bool {
	for _, rule := range firewallRules.Items {
		if ruleName == rule.Name {
			return true
		}
	}
	return false
}

func (gce *GCEClusterClient) deleteFirewallRule(cluster *clusterv1.Cluster, ruleName string) error {
	clusterConfig, err := gce.clusterproviderconfig(cluster.Spec.ProviderConfig)
	if err != nil {
		return fmt.Errorf("error parsing cluster provider config: %v", err)
	}
	op, err := gce.computeService.FirewallsDelete(clusterConfig.Project, ruleName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("error deleting firewall rule: %v", err)
	}
	return gce.computeService.WaitForOperation(clusterConfig.Project, op)
}

func (gce *GCEClusterClient) handleError(cluster *clusterv1.Cluster, err *apierrors.ClusterError, eventAction string) error {
	if gce.clusterClient != nil {
		cluster.Status.ErrorReason = err.Reason
		cluster.Status.ErrorMessage = err.Message
		if _, clusterError := gce.clusterClient.UpdateStatus(cluster); clusterError != nil {
			glog.Warningf("Error updating Cluster Status ErrorReason and Error Message: %v", clusterError)
		}
	}

	glog.Errorf("Cluster error: %v", err.Message)
	return err
}

func (gce *GCEClusterClient) fireErrorEvent(cluster *clusterv1.Cluster, err error, eventAction string) {
	gce.eventRecorder.Eventf(cluster, corev1.EventTypeWarning, "Failed"+eventAction, "%v", err)
}

func (gce *GCEClusterClient) clusterproviderconfig(providerConfig clusterv1.ProviderConfig) (*gceconfigv1.GCEClusterProviderConfig, error) {
	var config gceconfigv1.GCEClusterProviderConfig
	err := gce.gceProviderConfigCodec.DecodeFromProviderConfig(providerConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}
