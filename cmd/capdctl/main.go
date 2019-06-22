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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/kubernetes-sigs/cluster-api-provider-docker/execer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// TODO: Generate the RBAC stuff from somewhere instead of copy pasta

const (
	// Important to keep this consistent.
	controlPlaneSet = "controlplane"
)

type machineOptions struct {
	name, namespace, clusterName, set, version *string
}

func (mo *machineOptions) initFlags(fs *flag.FlagSet) {
	mo.name = fs.String("name", "my-machine", "The name of the machine")
	mo.namespace = fs.String("namespace", "my-namespace", "The namespece of the machine")
	mo.clusterName = fs.String("cluster-name", "my-cluster", "The name of the cluster the machine belongs to")
	mo.set = fs.String("set", "worker", "The role of the machine. Valid entries ['worker', 'controlplane']")
	mo.version = fs.String("version", "v1.14.2", "The Kubernetes version to run")
}

func main() {
	setup := flag.NewFlagSet("setup", flag.ExitOnError)
	managementClusterName := setup.String("cluster-name", "kind", "The name of the management cluster")

	// crds takes no args

	capd := flag.NewFlagSet("capd", flag.ExitOnError)
	capdImage := capd.String("capd-image", "gcr.io/kubernetes1-226021/capd-manager:latest", "The capd manager image to run")
	capiImage := capd.String("capi-image", "gcr.io/k8s-cluster-api/cluster-api-controller:0.1.3", "The capi manager image to run")

	controlPlane := flag.NewFlagSet("control-plane", flag.ExitOnError)
	controlPlaneOpts := new(machineOptions)
	controlPlaneOpts.initFlags(controlPlane)
	*controlPlaneOpts.set = controlPlaneSet

	worker := flag.NewFlagSet("worker", flag.ExitOnError)
	workerOpts := new(machineOptions)
	workerOpts.initFlags(worker)
	*workerOpts.set = "worker"

	cluster := flag.NewFlagSet("cluster", flag.ExitOnError)
	clusterName := cluster.String("cluster-name", "my-cluster", "The name of the cluster")
	clusterNamespace := cluster.String("namespace", "my-namespace", "The namespace the cluster belongs to")

	if len(os.Args) < 2 {
		fmt.Println("At least one subcommand is requied.")
		fmt.Println(usage())
	}

	switch os.Args[1] {
	case "setup":
		setup.Parse(os.Args[2:])
		makeManagementCluster(*managementClusterName)
	case "crds":
		printCRDs()
	case "capd":
		capd.Parse(os.Args[2:])
		printClusterAPIPlane(*capdImage, *capiImage)
	case "control-plane":
		controlPlane.Parse(os.Args[2:])
		fmt.Fprintf(os.Stdout, machineYAML(controlPlaneOpts))
	case "worker":
		worker.Parse(os.Args[2:])
		fmt.Fprintf(os.Stdout, machineYAML(workerOpts))
	case "cluster":
		cluster.Parse(os.Args[2:])
		fmt.Fprintf(os.Stdout, clusterYAML(*clusterName, *clusterNamespace))
	case "help":
		fmt.Println(usage())
	default:
		fmt.Println(usage())
		os.Exit(1)
	}
}

func usage() string {
	return `capdctl gets you up and running with capd

subcommands are:

  setup - Create a management cluster
    example: capdctl setup --name my-management-cluster-name

  crds - Write Cluster API CRDs required to run capd to stdout
    example: capdctl crds | kubectl apply -f -

  capd - Write capd kubernetes components that run necessary managers to stdout
    example: capdctl capd -capd-image gcr.io/kubernetes1-226021/capd-manager:latest -capi-image gcr.io/k8s-cluster-api/cluster-api-controller:0.1.2 | kubeclt apply -f -

  control-plane - Write a capd control plane machine to stdout
    example: capdctl control-plane -name my-control-plane -namespace my-namespace -cluster-name my-cluster -version v1.14.1 | kubectl apply -f -

  worker - Write a capd worker machine to stdout
    example: capdctl worker -name my-worker -namespace my-namespace -cluster-name my-cluster -version 1.14.2 | kubectl apply -f -

  cluster - Write a capd cluster object to stdout
    example: capdctl cluster -cluster-name my-cluster -namespace my-namespace | kubectl apply -f -
`
}

func clusterYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: %s
  namespace: %s
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "cluster.local"
  providerSpec: {}`, name, namespace)
}

func machineYAML(opts *machineOptions) string {
	machine := v1alpha1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: "cluster.k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      *opts.name,
			Namespace: *opts.namespace,
			Labels: map[string]string{
				"cluster.k8s.io/cluster-name": *opts.clusterName,
				"set":                         *opts.set,
			},
		},
		Spec: v1alpha1.MachineSpec{
			ProviderSpec: v1alpha1.ProviderSpec{},
		},
	}
	if *opts.set == controlPlaneSet {
		machine.Spec.Versions.ControlPlane = *opts.version
	}
	if *opts.set == "worker" {
		machine.Spec.Versions.Kubelet = *opts.version
	}
	b, err := json.Marshal(machine)
	// TODO don't panic on the error
	if err != nil {
		panic(err)
	}
	return string(b)
}

func makeManagementCluster(clusterName string) {
	kind := execer.NewClient("kind")
	// start kind with docker mount
	kindConfig, err := kindConfigFile()
	if err != nil {
		panic(err)
	}
	if err := kind.RunCommand("create", "cluster", "--name", clusterName, "--config", kindConfig); err != nil {
		panic(err)
	}
}

// TODO if possible, use the kind library instead of the command line tool
func kindConfigFile() (string, error) {
	kfg := `kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
- role: control-plane
  extraMounts:
  - containerPath: /var/run/docker.sock
    hostPath: /var/run/docker.sock
`

	f, err := ioutil.TempFile("", "*-kind-config.yaml")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(kfg); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func printCRDs() {
	fmt.Fprintln(os.Stdout, crds)
}

func printClusterAPIPlane(capdImage, capiImage string) {
	fmt.Fprintln(os.Stdout, getCAPDPlane(capdImage, capiImage))
}

func getCAPDPlane(capdImage, capiImage string) string {
	return fmt.Sprintf(capiPlane, capdImage, capiImage)
}

var capiPlane = `
apiVersion: v1
kind: Namespace
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: docker-provider-system
---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: cluster-api-system
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  labels:
    control-plane: controller-manager
  name: docker-provider-controller-manager
  namespace: docker-provider-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  serviceName: docker-provider-controller-manager-service
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      containers:
      - name: capd-manager
        image: %s
        command:
        - capd-manager
        volumeMounts:
        - mountPath: /var/run/docker.sock
          name: dockersock
        - mountPath: /var/lib/docker
          name: dockerlib
        securityContext:
          privileged: true
      volumes:
      - name: dockersock
        hostPath:
          path: /var/run/docker.sock
          type: Socket
      - name: dockerlib
        hostPath:
          path: /var/lib/docker
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      - key: CriticalAddonsOnly
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/notReady
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/unreachable
        operator: Exists
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  labels:
    control-plane: controller-manager
    controller-tools.k8s.io: "1.0"
  name: cluster-api-controller-manager
  namespace: cluster-api-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
      controller-tools.k8s.io: "1.0"
  serviceName: cluster-api-controller-manager-service
  template:
    metadata:
      labels:
        control-plane: controller-manager
        controller-tools.k8s.io: "1.0"
    spec:
      containers:
      - command:
        - /manager
        image: %s
        name: manager
`

// TODO generate the CRDs
var crds = `
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: clusters.cluster.k8s.io
spec:
  group: cluster.k8s.io
  names:
    kind: Cluster
    plural: clusters
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          properties:
            clusterNetwork:
              description: Cluster network configuration
              properties:
                pods:
                  description: The network ranges from which Pod networks are allocated.
                  properties:
                    cidrBlocks:
                      items:
                        type: string
                      type: array
                  required:
                  - cidrBlocks
                  type: object
                serviceDomain:
                  description: Domain name for services.
                  type: string
                services:
                  description: The network ranges from which service VIPs are allocated.
                  properties:
                    cidrBlocks:
                      items:
                        type: string
                      type: array
                  required:
                  - cidrBlocks
                  type: object
              required:
              - services
              - pods
              - serviceDomain
              type: object
            providerSpec:
              description: Provider-specific serialized configuration to use during
                cluster creation. It is recommended that providers maintain their
                own versioned API types that should be serialized/deserialized from
                this field.
              properties:
                value:
                  description: Value is an inlined, serialized representation of the
                    resource configuration. It is recommended that providers maintain
                    their own versioned API types that should be serialized/deserialized
                    from this field, akin to component config.
                  type: object
                valueFrom:
                  description: Source for the provider configuration. Cannot be used
                    if value is not empty.
                  properties:
                    machineClass:
                      description: The machine class from which the provider config
                        should be sourced.
                      properties:
                        provider:
                          description: Provider is the name of the cloud-provider
                            which MachineClass is intended for.
                          type: string
                      type: object
                  type: object
              type: object
          required:
          - clusterNetwork
          type: object
        status:
          properties:
            apiEndpoints:
              description: APIEndpoint represents the endpoint to communicate with
                the IP.
              items:
                properties:
                  host:
                    description: The hostname on which the API server is serving.
                    type: string
                  port:
                    description: The port on which the API server is serving.
                    format: int64
                    type: integer
                required:
                - host
                - port
                type: object
              type: array
            errorMessage:
              description: If set, indicates that there is a problem reconciling the
                state, and will be set to a descriptive error message.
              type: string
            errorReason:
              description: If set, indicates that there is a problem reconciling the
                state, and will be set to a token value suitable for programmatic
                interpretation.
              type: string
            providerStatus:
              description: Provider-specific status. It is recommended that providers
                maintain their own versioned API types that should be serialized/deserialized
                from this field.
              type: object
          type: object
  version: v1alpha1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: machineclasses.cluster.k8s.io
spec:
  group: cluster.k8s.io
  names:
    kind: MachineClass
    plural: machineclasses
  scope: Namespaced
  validation:
    openAPIV3Schema:
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        providerSpec:
          description: Provider-specific configuration to use during node creation.
          type: object
      required:
      - providerSpec
  version: v1alpha1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: machinedeployments.cluster.k8s.io
spec:
  group: cluster.k8s.io
  names:
    kind: MachineDeployment
    plural: machinedeployments
  scope: Namespaced
  subresources:
    scale:
      labelSelectorPath: .status.labelSelector
      specReplicasPath: .spec.replicas
      statusReplicasPath: .status.replicas
    status: {}
  validation:
    openAPIV3Schema:
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          properties:
            minReadySeconds:
              description: Minimum number of seconds for which a newly created machine
                should be ready. Defaults to 0 (machine will be considered available
                as soon as it is ready)
              format: int32
              type: integer
            paused:
              description: Indicates that the deployment is paused.
              type: boolean
            progressDeadlineSeconds:
              description: The maximum time in seconds for a deployment to make progress
                before it is considered to be failed. The deployment controller will
                continue to process failed deployments and a condition with a ProgressDeadlineExceeded
                reason will be surfaced in the deployment status. Note that progress
                will not be estimated during the time a deployment is paused. Defaults
                to 600s.
              format: int32
              type: integer
            replicas:
              description: Number of desired machines. Defaults to 1. This is a pointer
                to distinguish between explicit zero and not specified.
              format: int32
              type: integer
            revisionHistoryLimit:
              description: The number of old MachineSets to retain to allow rollback.
                This is a pointer to distinguish between explicit zero and not specified.
                Defaults to 1.
              format: int32
              type: integer
            selector:
              description: Label selector for machines. Existing MachineSets whose
                machines are selected by this will be the ones affected by this deployment.
                It must match the machine template's labels.
              type: object
            strategy:
              description: The deployment strategy to use to replace existing machines
                with new ones.
              properties:
                rollingUpdate:
                  description: Rolling update config params. Present only if MachineDeploymentStrategyType
                    = RollingUpdate.
                  properties:
                    maxSurge:
                      description: 'The maximum number of machines that can be scheduled
                        above the desired number of machines. Value can be an absolute
                        number (ex: 5) or a percentage of desired machines (ex: 10%).
                        This can not be 0 if MaxUnavailable is 0. Absolute number
                        is calculated from percentage by rounding up. Defaults to
                        1. Example: when this is set to 30%, the new MachineSet can
                        be scaled up immediately when the rolling update starts, such
                        that the total number of old and new machines do not exceed
                        130% of desired machines. Once old machines have been killed,
                        new MachineSet can be scaled up further, ensuring that total
                        number of machines running at any time during the update is
                        at most 130% of desired machines.'
                      oneOf:
                      - type: string
                      - type: integer
                    maxUnavailable:
                      description: 'The maximum number of machines that can be unavailable
                        during the update. Value can be an absolute number (ex: 5)
                        or a percentage of desired machines (ex: 10%). Absolute number
                        is calculated from percentage by rounding down. This can not
                        be 0 if MaxSurge is 0. Defaults to 0. Example: when this is
                        set to 30%, the old MachineSet can be scaled down to 70% of
                        desired machines immediately when the rolling update starts.
                        Once new machines are ready, old MachineSet can be scaled
                        down further, followed by scaling up the new MachineSet, ensuring
                        that the total number of machines available at all times during
                        the update is at least 70% of desired machines.'
                      oneOf:
                      - type: string
                      - type: integer
                  type: object
                type:
                  description: Type of deployment. Currently the only supported strategy
                    is "RollingUpdate". Default is RollingUpdate.
                  type: string
              type: object
            template:
              description: Template describes the machines that will be created.
              properties:
                metadata:
                  description: 'Standard object''s metadata. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata'
                  type: object
                spec:
                  description: 'Specification of the desired behavior of the machine.
                    More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status'
                  properties:
                    configSource:
                      description: ConfigSource is used to populate in the associated
                        Node for dynamic kubelet config. This field already exists
                        in Node, so any updates to it in the Machine spec will be
                        automatically copied to the linked NodeRef from the status.
                        The rest of dynamic kubelet config support should then work
                        as-is.
                      type: object
                    metadata:
                      description: ObjectMeta will autopopulate the Node created.
                        Use this to indicate what labels, annotations, name prefix,
                        etc., should be used when creating the Node.
                      type: object
                    providerID:
                      description: ProviderID is the identification ID of the machine
                        provided by the provider. This field must match the provider
                        ID as seen on the node object corresponding to this machine.
                        This field is required by higher level consumers of cluster-api.
                        Example use case is cluster autoscaler with cluster-api as
                        provider. Clean-up logic in the autoscaler compares machines
                        to nodes to find out machines at provider which could not
                        get registered as Kubernetes nodes. With cluster-api as a
                        generic out-of-tree provider for autoscaler, this field is
                        required by autoscaler to be able to have a provider view
                        of the list of machines. Another list of nodes is queried
                        from the k8s apiserver and then a comparison is done to find
                        out unregistered machines and are marked for delete. This
                        field will be set by the actuators and consumed by higher
                        level entities like autoscaler that will be interfacing with
                        cluster-api as generic provider.
                      type: string
                    providerSpec:
                      description: ProviderSpec details Provider-specific configuration
                        to use during node creation.
                      properties:
                        value:
                          description: Value is an inlined, serialized representation
                            of the resource configuration. It is recommended that
                            providers maintain their own versioned API types that
                            should be serialized/deserialized from this field, akin
                            to component config.
                          type: object
                        valueFrom:
                          description: Source for the provider configuration. Cannot
                            be used if value is not empty.
                          properties:
                            machineClass:
                              description: The machine class from which the provider
                                config should be sourced.
                              properties:
                                provider:
                                  description: Provider is the name of the cloud-provider
                                    which MachineClass is intended for.
                                  type: string
                              type: object
                          type: object
                      type: object
                    taints:
                      description: Taints is the full, authoritative list of taints
                        to apply to the corresponding Node. This list will overwrite
                        any modifications made to the Node on an ongoing basis.
                      items:
                        type: object
                      type: array
                    versions:
                      description: Versions of key software to use. This field is
                        optional at cluster creation time, and omitting the field
                        indicates that the cluster installation tool should select
                        defaults for the user. These defaults may differ based on
                        the cluster installer, but the tool should populate the values
                        it uses when persisting Machine objects. A Machine spec missing
                        this field at runtime is invalid.
                      properties:
                        controlPlane:
                          description: ControlPlane is the semantic version of the
                            Kubernetes control plane to run. This should only be populated
                            when the machine is a control plane.
                          type: string
                        kubelet:
                          description: Kubelet is the semantic version of kubelet
                            to run
                          type: string
                      required:
                      - kubelet
                      type: object
                  required:
                  - providerSpec
                  type: object
              type: object
          required:
          - selector
          - template
          type: object
        status:
          properties:
            availableReplicas:
              description: Total number of available machines (ready for at least
                minReadySeconds) targeted by this deployment.
              format: int32
              type: integer
            observedGeneration:
              description: The generation observed by the deployment controller.
              format: int64
              type: integer
            readyReplicas:
              description: Total number of ready machines targeted by this deployment.
              format: int32
              type: integer
            replicas:
              description: Total number of non-terminated machines targeted by this
                deployment (their labels match the selector).
              format: int32
              type: integer
            unavailableReplicas:
              description: Total number of unavailable machines targeted by this deployment.
                This is the total number of machines that are still required for the
                deployment to have 100% available capacity. They may either be machines
                that are running but not yet available or machines that still have
                not been created.
              format: int32
              type: integer
            updatedReplicas:
              description: Total number of non-terminated machines targeted by this
                deployment that have the desired template spec.
              format: int32
              type: integer
          type: object
  version: v1alpha1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: machines.cluster.k8s.io
spec:
  additionalPrinterColumns:
  - JSONPath: .spec.providerID
    description: Provider ID
    name: ProviderID
    type: string
  - JSONPath: .status.phase
    description: Machine status such as Terminating/Pending/Running/Failed etc
    name: Phase
    type: string
  group: cluster.k8s.io
  names:
    kind: Machine
    plural: machines
  scope: Namespaced
  subresources:
    status: {}
  validation:
    openAPIV3Schema:
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          properties:
            configSource:
              description: ConfigSource is used to populate in the associated Node
                for dynamic kubelet config. This field already exists in Node, so
                any updates to it in the Machine spec will be automatically copied
                to the linked NodeRef from the status. The rest of dynamic kubelet
                config support should then work as-is.
              type: object
            metadata:
              description: ObjectMeta will autopopulate the Node created. Use this
                to indicate what labels, annotations, name prefix, etc., should be
                used when creating the Node.
              type: object
            providerID:
              description: ProviderID is the identification ID of the machine provided
                by the provider. This field must match the provider ID as seen on
                the node object corresponding to this machine. This field is required
                by higher level consumers of cluster-api. Example use case is cluster
                autoscaler with cluster-api as provider. Clean-up logic in the autoscaler
                compares machines to nodes to find out machines at provider which
                could not get registered as Kubernetes nodes. With cluster-api as
                a generic out-of-tree provider for autoscaler, this field is required
                by autoscaler to be able to have a provider view of the list of machines.
                Another list of nodes is queried from the k8s apiserver and then comparison
                is done to find out unregistered machines and are marked for delete.
                This field will be set by the actuators and consumed by higher level
                entities like autoscaler that will be interfacing with cluster-api
                as generic provider.
              type: string
            providerSpec:
              description: ProviderSpec details Provider-specific configuration to
                use during node creation.
              properties:
                value:
                  description: Value is an inlined, serialized representation of the
                    resource configuration. It is recommended that providers maintain
                    their own versioned API types that should be serialized/deserialized
                    from this field, akin to component config.
                  type: object
                valueFrom:
                  description: Source for the provider configuration. Cannot be used
                    if value is not empty.
                  properties:
                    machineClass:
                      description: The machine class from which the provider config
                        should be sourced.
                      properties:
                        provider:
                          description: Provider is the name of the cloud-provider
                            which MachineClass is intended for.
                          type: string
                      type: object
                  type: object
              type: object
            taints:
              description: Taints is the full, authoritative list of taints to apply
                to the corresponding Node. This list will overwrite any modifications
                made to the Node on an ongoing basis.
              items:
                type: object
              type: array
            versions:
              description: Versions of key software to use. This field is optional
                at cluster creation time, and omitting the field indicates that the
                cluster installation tool should select defaults for the user. These
                defaults may differ based on the cluster installer, but the tool should
                populate the values it uses when persisting Machine objects. A Machine
                spec missing this field at runtime is invalid.
              properties:
                controlPlane:
                  description: ControlPlane is the semantic version of the Kubernetes
                    control plane to run. This should only be populated when the machine
                    is a control plane.
                  type: string
                kubelet:
                  description: Kubelet is the semantic version of kubelet to run
                  type: string
              required:
              - kubelet
              type: object
          required:
          - providerSpec
          type: object
        status:
          properties:
            addresses:
              description: Addresses is a list of addresses assigned to the machine.
                Queried from cloud provider, if available.
              items:
                type: object
              type: array
            conditions:
              description: 'Conditions lists the conditions synced from the node conditions
                of the corresponding node-object. Machine-controller is responsible
                for keeping conditions up-to-date. MachineSet controller will be taking
                these conditions as a signal to decide if machine is healthy or needs
                to be replaced. Refer: https://kubernetes.io/docs/concepts/architecture/nodes/#condition'
              items:
                type: object
              type: array
            errorMessage:
              description: ErrorMessage will be set in the event that there is a terminal
                problem reconciling the Machine and will contain a more verbose string
                suitable for logging and human consumption.  This field should not
                be set for transitive errors that a controller faces that are expected
                to be fixed automatically over time (like service outages), but instead
                indicate that something is fundamentally wrong with the Machine's
                spec or the configuration of the controller, and that manual intervention
                is required. Examples of terminal errors would be invalid combinations
                of settings in the spec, values that are unsupported by the controller,
                or the responsible controller itself being critically misconfigured.  Any
                transient errors that occur during the reconciliation of Machines
                can be added as events to the Machine object and/or logged in the
                controller's output.
              type: string
            errorReason:
              description: ErrorReason will be set in the event that there is a terminal
                problem reconciling the Machine and will contain a succinct value
                suitable for machine interpretation.  This field should not be set
                for transitive errors that a controller faces that are expected to
                be fixed automatically over time (like service outages), but instead
                indicate that something is fundamentally wrong with the Machine's
                spec or the configuration of the controller, and that manual intervention
                is required. Examples of terminal errors would be invalid combinations
                of settings in the spec, values that are unsupported by the controller,
                or the responsible controller itself being critically misconfigured.  Any
                transient errors that occur during the reconciliation of Machines
                can be added as events to the Machine object and/or logged in the
                controller's output.
              type: string
            lastOperation:
              description: LastOperation describes the last-operation performed by
                the machine-controller. This API should be useful as a history in
                terms of the latest operation performed on the specific machine. It
                should also convey the state of the latest-operation for example if
                it is still on-going, failed or completed successfully.
              properties:
                description:
                  description: Description is the human-readable description of the
                    last operation.
                  type: string
                lastUpdated:
                  description: LastUpdated is the timestamp at which LastOperation
                    API was last-updated.
                  format: date-time
                  type: string
                state:
                  description: State is the current status of the last performed operation.
                    E.g. Processing, Failed, Successful etc
                  type: string
                type:
                  description: Type is the type of operation which was last performed.
                    E.g. Create, Delete, Update etc
                  type: string
              type: object
            lastUpdated:
              description: LastUpdated identifies when this status was last observed.
              format: date-time
              type: string
            nodeRef:
              description: NodeRef will point to the corresponding Node if it exists.
              type: object
            phase:
              description: Phase represents the current phase of machine actuation.
                E.g. Pending, Running, Terminating, Failed etc.
              type: string
            providerStatus:
              description: ProviderStatus details a Provider-specific status. It is
                recommended that providers maintain their own versioned API types
                that should be serialized/deserialized from this field.
              type: object
            versions:
              description: 'Versions specifies the current versions of software on
                the corresponding Node (if it exists). This is provided for a few
                reasons:  1) It is more convenient than checking the NodeRef, traversing
                it to    the Node, and finding the appropriate field in Node.Status.NodeInfo    (which
                uses different field names and formatting). 2) It removes some of
                the dependency on the structure of the Node,    so that if the structure
                of Node.Status.NodeInfo changes, only    machine controllers need
                to be updated, rather than every client    of the Machines API. 3)
                There is no other simple way to check the control plane    version.
                A client would have to connect directly to the apiserver    running
                on the target node in order to find out its version.'
              properties:
                controlPlane:
                  description: ControlPlane is the semantic version of the Kubernetes
                    control plane to run. This should only be populated when the machine
                    is a control plane.
                  type: string
                kubelet:
                  description: Kubelet is the semantic version of kubelet to run
                  type: string
              required:
              - kubelet
              type: object
          type: object
  version: v1alpha1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
---
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  labels:
    controller-tools.k8s.io: "1.0"
  name: machinesets.cluster.k8s.io
spec:
  group: cluster.k8s.io
  names:
    kind: MachineSet
    plural: machinesets
  scope: Namespaced
  subresources:
    scale:
      labelSelectorPath: .status.labelSelector
      specReplicasPath: .spec.replicas
      statusReplicasPath: .status.replicas
    status: {}
  validation:
    openAPIV3Schema:
      properties:
        apiVersion:
          description: 'APIVersion defines the versioned schema of this representation
            of an object. Servers should convert recognized schemas to the latest
            internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources'
          type: string
        kind:
          description: 'Kind is a string value representing the REST resource this
            object represents. Servers may infer this from the endpoint the client
            submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds'
          type: string
        metadata:
          type: object
        spec:
          properties:
            deletePolicy:
              description: DeletePolicy defines the policy used to identify nodes
                to delete when downscaling. Defaults to "Random".  Valid values are
                "Random, "Newest", "Oldest"
              enum:
              - Random
              - Newest
              - Oldest
              type: string
            minReadySeconds:
              description: MinReadySeconds is the minimum number of seconds for which
                a newly created machine should be ready. Defaults to 0 (machine will
                be considered available as soon as it is ready)
              format: int32
              type: integer
            replicas:
              description: Replicas is the number of desired replicas. This is a pointer
                to distinguish between explicit zero and unspecified. Defaults to
                1.
              format: int32
              type: integer
            selector:
              description: 'Selector is a label query over machines that should match
                the replica count. Label keys and values that must match in order
                to be controlled by this MachineSet. It must match the machine template''s
                labels. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors'
              type: object
            template:
              description: Template is the object that describes the machine that
                will be created if insufficient replicas are detected.
              properties:
                metadata:
                  description: 'Standard object''s metadata. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#metadata'
                  type: object
                spec:
                  description: 'Specification of the desired behavior of the machine.
                    More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#spec-and-status'
                  properties:
                    configSource:
                      description: ConfigSource is used to populate in the associated
                        Node for dynamic kubelet config. This field already exists
                        in Node, so any updates to it in the Machine spec will be
                        automatically copied to the linked NodeRef from the status.
                        The rest of dynamic kubelet config support should then work
                        as-is.
                      type: object
                    metadata:
                      description: ObjectMeta will autopopulate the Node created.
                        Use this to indicate what labels, annotations, name prefix,
                        etc., should be used when creating the Node.
                      type: object
                    providerID:
                      description: ProviderID is the identification ID of the machine
                        provided by the provider. This field must match the provider
                        ID as seen on the node object corresponding to this machine.
                        This field is required by higher level consumers of cluster-api.
                        Example use case is cluster autoscaler with cluster-api as
                        provider. Clean-up logic in the autoscaler compares machines
                        to nodes to find out machines at provider which could not
                        get registered as Kubernetes nodes. With cluster-api as a
                        generic out-of-tree provider for autoscaler, this field is
                        required by autoscaler to be able to have a provider view
                        of the list of machines. Another list of nodes is queried
                        from the k8s apiserver and then a comparison is done to find
                        out unregistered machines and are marked for delete. This
                        field will be set by the actuators and consumed by higher
                        level entities like autoscaler that will be interfacing with
                        cluster-api as generic provider.
                      type: string
                    providerSpec:
                      description: ProviderSpec details Provider-specific configuration
                        to use during node creation.
                      properties:
                        value:
                          description: Value is an inlined, serialized representation
                            of the resource configuration. It is recommended that
                            providers maintain their own versioned API types that
                            should be serialized/deserialized from this field, akin
                            to component config.
                          type: object
                        valueFrom:
                          description: Source for the provider configuration. Cannot
                            be used if value is not empty.
                          properties:
                            machineClass:
                              description: The machine class from which the provider
                                config should be sourced.
                              properties:
                                provider:
                                  description: Provider is the name of the cloud-provider
                                    which MachineClass is intended for.
                                  type: string
                              type: object
                          type: object
                      type: object
                    taints:
                      description: Taints is the full, authoritative list of taints
                        to apply to the corresponding Node. This list will overwrite
                        any modifications made to the Node on an ongoing basis.
                      items:
                        type: object
                      type: array
                    versions:
                      description: Versions of key software to use. This field is
                        optional at cluster creation time, and omitting the field
                        indicates that the cluster installation tool should select
                        defaults for the user. These defaults may differ based on
                        the cluster installer, but the tool should populate the values
                        it uses when persisting Machine objects. A Machine spec missing
                        this field at runtime is invalid.
                      properties:
                        controlPlane:
                          description: ControlPlane is the semantic version of the
                            Kubernetes control plane to run. This should only be populated
                            when the machine is a control plane.
                          type: string
                        kubelet:
                          description: Kubelet is the semantic version of kubelet
                            to run
                          type: string
                      required:
                      - kubelet
                      type: object
                  required:
                  - providerSpec
                  type: object
              type: object
          required:
          - selector
          type: object
        status:
          properties:
            availableReplicas:
              description: The number of available replicas (ready for at least minReadySeconds)
                for this MachineSet.
              format: int32
              type: integer
            errorMessage:
              type: string
            errorReason:
              description: In the event that there is a terminal problem reconciling
                the replicas, both ErrorReason and ErrorMessage will be set. ErrorReason
                will be populated with a succinct value suitable for machine interpretation,
                while ErrorMessage will contain a more verbose string suitable for
                logging and human consumption.  These fields should not be set for
                transitive errors that a controller faces that are expected to be
                fixed automatically over time (like service outages), but instead
                indicate that something is fundamentally wrong with the MachineTemplate's
                spec or the configuration of the machine controller, and that manual
                intervention is required. Examples of terminal errors would be invalid
                combinations of settings in the spec, values that are unsupported
                by the machine controller, or the responsible machine controller itself
                being critically misconfigured.  Any transient errors that occur during
                the reconciliation of Machines can be added as events to the MachineSet
                object and/or logged in the controller's output.
              type: string
            fullyLabeledReplicas:
              description: The number of replicas that have labels matching the labels
                of the machine template of the MachineSet.
              format: int32
              type: integer
            observedGeneration:
              description: ObservedGeneration reflects the generation of the most
                recently observed MachineSet.
              format: int64
              type: integer
            readyReplicas:
              description: The number of ready replicas for this MachineSet. A machine
                is considered ready when the node has been created and is "Ready".
              format: int32
              type: integer
            replicas:
              description: Replicas is the most recently observed number of replicas.
              format: int32
              type: integer
          required:
          - replicas
          type: object
  version: v1alpha1
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: docker-provider-manager-role
rules:
- apiGroups:
  - cluster.k8s.io
  resources:
  - clusters
  - clusters/status
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - machines
  - machines/status
  - machinedeployments
  - machinedeployments/status
  - machinesets
  - machinesets/status
  - machineclasses
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - clusters
  - clusters/status
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - nodes
  - events
  - secrets
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cluster-api-manager-role
rules:
- apiGroups:
  - cluster.k8s.io
  resources:
  - clusters
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - get
  - list
  - watch
  - create
- apiGroups:
  - cluster.k8s.io
  resources:
  - machines
  - machines/status
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - machinedeployments
  - machinedeployments/status
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - machinesets
  - machinesets/status
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - machines
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - cluster.k8s.io
  resources:
  - machines
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  creationTimestamp: null
  name: docker-provider-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: docker-provider-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: docker-provider-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  creationTimestamp: null
  name: cluster-api-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-api-manager-role
subjects:
- kind: ServiceAccount
  name: default
  namespace: cluster-api-system
`
