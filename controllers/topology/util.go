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

package topology

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/external"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// loggerFrom returns a logger with predefined values from a context.Context.
// The logger, when used with controllers, can be expected to contain basic information about the object
// that's being reconciled like:
// - `reconciler group` and `reconciler kind` coming from the For(...) object passed in when building a controller.
// - `name` and `namespace` injected from the reconciliation request.
//
// This is meant to be used with the context supplied in a struct that satisfies the Reconciler interface.
func loggerFrom(ctx context.Context) logger {
	log := ctrl.LoggerFrom(ctx)
	return &topologyReconcileLogger{
		Logger: log,
	}
}

// logger provides a wrapper to log.Logger to be used for topology reconciler.
type logger interface {
	// WithObject adds to the logger information about the object being modified by reconcile, which in most case it is
	// a resources being part of the Cluster by reconciled.
	WithObject(obj client.Object) logger

	// WithMachineDeployment adds to the logger information about the MachineDeployment object being processed.
	WithMachineDeployment(md *clusterv1.MachineDeployment) logger

	// V returns a logger value for a specific verbosity level, relative to
	// this logger.
	V(level int) logger

	// Infof logs to the INFO log.
	// Arguments are handled in the manner of fmt.Printf.
	Infof(msg string, a ...interface{})

	// Into takes a context and sets the logger as one of its keys.
	//
	// This is meant to be used in reconcilers to enrich the logger within a context with additional values.
	Into(ctx context.Context) (context.Context, logger)
}

// topologyReconcileLogger implements Logger.
type topologyReconcileLogger struct {
	logr.Logger
}

// WithObject adds to the logger information about the object being modified by reconcile, which in most case it is
// a resources being part of the Cluster by reconciled.
func (l *topologyReconcileLogger) WithObject(obj client.Object) logger {
	l.Logger = l.Logger.WithValues(
		"object groupVersion", obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		"object kind", obj.GetObjectKind().GroupVersionKind().Kind,
		"object", obj.GetName(),
	)
	return l
}

// WithMachineDeployment adds to the logger information about the MachineDeployment object being processed.
func (l *topologyReconcileLogger) WithMachineDeployment(md *clusterv1.MachineDeployment) logger {
	topologyName := md.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
	l.Logger = l.Logger.WithValues(
		"machineDeployment name", md.GetName(),
		"machineDeployment topologyName", topologyName,
	)
	return l
}

// V returns a logger value for a specific verbosity level, relative to
// this logger.
func (l *topologyReconcileLogger) V(level int) logger {
	l.Logger = l.Logger.V(level)
	return l
}

// Infof logs to the INFO log.
// Arguments are handled in the manner of fmt.Printf.
func (l *topologyReconcileLogger) Infof(msg string, a ...interface{}) {
	l.Logger.Info(fmt.Sprintf(msg, a...))
}

// Into takes a context and sets the logger as one of its keys.
//
// This is meant to be used in reconcilers to enrich the logger within a context with additional values.
func (l *topologyReconcileLogger) Into(ctx context.Context) (context.Context, logger) {
	return ctrl.LoggerInto(ctx, l.Logger), l
}

// KRef return a reference to a Kubernetes object in the same format used by kubectl commands (kind/name).
type KRef struct {
	Obj client.Object
}

func (ref KRef) String() string {
	if ref.Obj == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", ref.Obj.GetObjectKind().GroupVersionKind().Kind, ref.Obj.GetName())
}

// bootstrapTemplateNamePrefix calculates the name prefix for a BootstrapTemplate.
func bootstrapTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-bootstrap-", clusterName, machineDeploymentTopologyName)
}

// infrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func infrastructureMachineTemplateNamePrefix(clusterName, machineDeploymentTopologyName string) string {
	return fmt.Sprintf("%s-%s-infra-", clusterName, machineDeploymentTopologyName)
}

// infrastructureMachineTemplateNamePrefix calculates the name prefix for a InfrastructureMachineTemplate.
func controlPlaneInfrastructureMachineTemplateNamePrefix(clusterName string) string {
	return fmt.Sprintf("%s-control-plane-", clusterName)
}

// getReference gets the object referenced in ref.
// If necessary, it updates the ref to the latest apiVersion of the current contract.
func (r *ClusterReconciler) getReference(ctx context.Context, ref *corev1.ObjectReference) (*unstructured.Unstructured, error) {
	if ref == nil {
		return nil, errors.New("reference is not set")
	}
	if err := utilconversion.UpdateReferenceAPIContract(ctx, r.Client, ref); err != nil {
		return nil, err
	}

	obj, err := external.Get(ctx, r.UnstructuredCachingClient, ref, ref.Namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve %s %q in namespace %q", ref.Kind, ref.Name, ref.Namespace)
	}
	return obj, nil
}

// refToUnstructured returns an unstructured object with details from an ObjectReference.
func refToUnstructured(ref *corev1.ObjectReference) *unstructured.Unstructured {
	uns := &unstructured.Unstructured{}
	uns.SetAPIVersion(ref.APIVersion)
	uns.SetKind(ref.Kind)
	uns.SetNamespace(ref.Namespace)
	uns.SetName(ref.Name)
	return uns
}
