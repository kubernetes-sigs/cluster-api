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

package log

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// LoggerFrom returns a logger with predefined values from a context.Context.
// The logger, when used with controllers, can be expected to contain basic information about the object
// that's being reconciled like:
// - `reconciler group` and `reconciler kind` coming from the For(...) object passed in when building a controller.
// - `name` and `namespace` injected from the reconciliation request.
//
// This is meant to be used with the context supplied in a struct that satisfies the Reconciler interface.
func LoggerFrom(ctx context.Context) Logger {
	log := ctrl.LoggerFrom(ctx)
	return &topologyReconcileLogger{
		// We use call depth 1 so the logger prints the log line of the caller of the log func (e.g. Infof)
		// not of the log func.
		// NOTE: We do this once here, so that we don't have to do this on every log call.
		Logger: log.WithCallDepth(1),
	}
}

// Logger provides a wrapper to log.Logger to be used for topology reconciler.
type Logger interface {
	// WithObject adds to the logger information about the object being modified by reconcile, which in most case it is
	// a resources being part of the Cluster by reconciled.
	WithObject(obj client.Object) Logger

	// WithRef adds to the logger information about the object ref being modified by reconcile, which in most case it is
	// a resources being part of the Cluster by reconciled.
	WithRef(ref *corev1.ObjectReference) Logger

	// WithMachineDeployment adds to the logger information about the MachineDeployment object being processed.
	WithMachineDeployment(md *clusterv1.MachineDeployment) Logger

	// WithValues adds key-value pairs of context to a logger.
	WithValues(keysAndValues ...interface{}) Logger

	// V returns a logger value for a specific verbosity level, relative to
	// this logger.
	V(level int) Logger

	// Infof logs to the INFO log.
	// Arguments are handled in the manner of fmt.Printf.
	Infof(msg string, a ...interface{})

	// Into takes a context and sets the logger as one of its keys.
	//
	// This is meant to be used in reconcilers to enrich the logger within a context with additional values.
	Into(ctx context.Context) (context.Context, Logger)
}

// topologyReconcileLogger implements Logger.
type topologyReconcileLogger struct {
	logr.Logger
}

// WithObject adds to the logger information about the object being modified by reconcile, which in most case it is
// a resources being part of the Cluster by reconciled.
func (l *topologyReconcileLogger) WithObject(obj client.Object) Logger {
	return &topologyReconcileLogger{
		Logger: l.Logger.WithValues(
			"object groupVersion", obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			"object kind", obj.GetObjectKind().GroupVersionKind().Kind,
			"object", obj.GetName(),
		),
	}
}

// WithRef adds to the logger information about the object ref being modified by reconcile, which in most case it is
// a resources being part of the Cluster by reconciled.
func (l *topologyReconcileLogger) WithRef(ref *corev1.ObjectReference) Logger {
	return &topologyReconcileLogger{
		Logger: l.Logger.WithValues(
			"object groupVersion", ref.APIVersion,
			"object kind", ref.Kind,
			"object", ref.Name,
		),
	}
}

// WithMachineDeployment adds to the logger information about the MachineDeployment object being processed.
func (l *topologyReconcileLogger) WithMachineDeployment(md *clusterv1.MachineDeployment) Logger {
	topologyName := md.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
	return &topologyReconcileLogger{
		Logger: l.Logger.WithValues(
			"machineDeployment name", md.GetName(),
			"machineDeployment topologyName", topologyName,
		),
	}
}

// WithValues adds key-value pairs of context to a logger.
func (l *topologyReconcileLogger) WithValues(keysAndValues ...interface{}) Logger {
	l.Logger = l.Logger.WithValues(keysAndValues...)
	return l
}

// V returns a logger value for a specific verbosity level, relative to
// this logger.
func (l *topologyReconcileLogger) V(level int) Logger {
	return &topologyReconcileLogger{
		Logger: l.Logger.V(level),
	}
}

// Infof logs to the INFO log.
// Arguments are handled in the manner of fmt.Printf.
func (l *topologyReconcileLogger) Infof(msg string, a ...interface{}) {
	l.Logger.Info(fmt.Sprintf(msg, a...))
}

// Into takes a context and sets the logger as one of its keys.
//
// This is meant to be used in reconcilers to enrich the logger within a context with additional values.
func (l *topologyReconcileLogger) Into(ctx context.Context) (context.Context, Logger) {
	return ctrl.LoggerInto(ctx, l.Logger), l
}

// KObj return a reference to a Kubernetes object in the same format used by kubectl commands (kind/name).
// Note: We're intentionally not using klog.KObj as we want the kind/name format instead of namespace/name.
type KObj struct {
	Obj client.Object
}

func (ref KObj) String() string {
	if ref.Obj == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", ref.Obj.GetObjectKind().GroupVersionKind().Kind, ref.Obj.GetName())
}

// KRef return a reference to a Kubernetes object in the same format used by kubectl commands (kind/name).
type KRef struct {
	Ref *corev1.ObjectReference
}

func (ref KRef) String() string {
	if ref.Ref == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s", ref.Ref.Kind, ref.Ref.Name)
}
