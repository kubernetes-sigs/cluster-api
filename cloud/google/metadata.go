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
	"bytes"
	"fmt"
	"text/template"

	"sigs.k8s.io/cluster-api/cloud/google/machinesetup"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

type metadataParams struct {
	Token        string
	Cluster      *clusterv1.Cluster
	Machine      *clusterv1.Machine
	DockerImages []string
	Project      string
	Metadata     *machinesetup.Metadata

	// These fields are set when executing the template if they are necessary.
	PodCIDR        string
	ServiceCIDR    string
	MasterEndpoint string
}

func nodeMetadata(token string, cluster *clusterv1.Cluster, machine *clusterv1.Machine, metadata *machinesetup.Metadata) (map[string]string, error) {
	params := metadataParams{
		Token:          token,
		Cluster:        cluster,
		Machine:        machine,
		Metadata:       metadata,
		PodCIDR:        getSubnet(cluster.Spec.ClusterNetwork.Pods),
		ServiceCIDR:    getSubnet(cluster.Spec.ClusterNetwork.Services),
		MasterEndpoint: getEndpoint(cluster.Status.APIEndpoints[0]),
	}

	nodeMetadata := map[string]string{}
	var buf bytes.Buffer
	if err := nodeEnvironmentVarsTemplate.Execute(&buf, params); err != nil {
		return nil, err
	}
	buf.WriteString(params.Metadata.StartupScript)
	nodeMetadata["startup-script"] = buf.String()
	return nodeMetadata, nil
}

func masterMetadata(token string, cluster *clusterv1.Cluster, machine *clusterv1.Machine, project string, metadata *machinesetup.Metadata) (map[string]string, error) {
	params := metadataParams{
		Token:       token,
		Cluster:     cluster,
		Machine:     machine,
		Project:     project,
		Metadata:    metadata,
		PodCIDR:     getSubnet(cluster.Spec.ClusterNetwork.Pods),
		ServiceCIDR: getSubnet(cluster.Spec.ClusterNetwork.Services),
	}

	masterMetadata := map[string]string{}
	var buf bytes.Buffer
	if err := masterEnvironmentVarsTemplate.Execute(&buf, params); err != nil {
		return nil, err
	}
	buf.WriteString(params.Metadata.StartupScript)
	masterMetadata["startup-script"] = buf.String()
	return masterMetadata, nil
}

func getEndpoint(apiEndpoint clusterv1.APIEndpoint) string {
	return fmt.Sprintf("%s:%d", apiEndpoint.Host, apiEndpoint.Port)
}

var (
	masterEnvironmentVarsTemplate *template.Template
	nodeEnvironmentVarsTemplate   *template.Template
)

func init() {
	masterEnvironmentVarsTemplate = template.Must(template.New("masterEnvironmentVars").Parse(masterEnvironmentVars))
	nodeEnvironmentVarsTemplate = template.Must(template.New("nodeEnvironmentVars").Parse(nodeEnvironmentVars))
}

// TODO(kcoronado): replace with actual network and node tag args when they are added into provider config.
const masterEnvironmentVars = `
#!/bin/bash
KUBELET_VERSION={{ .Machine.Spec.Versions.Kubelet }}
VERSION=v${KUBELET_VERSION}
TOKEN={{ .Token }}
PORT=443
MACHINE={{ .Machine.ObjectMeta.Name }}
CONTROL_PLANE_VERSION={{ .Machine.Spec.Versions.ControlPlane }}
CLUSTER_DNS_DOMAIN={{ .Cluster.Spec.ClusterNetwork.ServiceDomain }}
POD_CIDR={{ .PodCIDR }}
SERVICE_CIDR={{ .ServiceCIDR }}
# Environment variables for GCE cloud config
PROJECT={{ .Project }}
NETWORK=default
SUBNETWORK=kubernetes
NODE_TAG=worker
`

const nodeEnvironmentVars = `
#!/bin/bash
KUBELET_VERSION={{ .Machine.Spec.Versions.Kubelet }}
TOKEN={{ .Token }}
MASTER={{ .MasterEndpoint }}
MACHINE={{ .Machine.ObjectMeta.Name }}
CLUSTER_DNS_DOMAIN={{ .Cluster.Spec.ClusterNetwork.ServiceDomain }}
POD_CIDR={{ .PodCIDR }}
SERVICE_CIDER={{ .ServiceCIDR }}
`
