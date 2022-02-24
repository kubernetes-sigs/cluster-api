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
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// LoggerCustomizer is a util to create a LoggerCustomizer.
func LoggerCustomizer(log logr.Logger, controllerName, kind string) func(_ logr.Logger, req reconcile.Request) logr.Logger {
	// FIXME: We need further discussion on how we want to to customize the CR logger/context
	// e.g. to make it future-proof for tracing.
	var rngSeed int64
	_ = binary.Read(crand.Reader, binary.LittleEndian, &rngSeed)
	randSource := rand.New(rand.NewSource(rngSeed)) //nolint:gosec // math/rand is enough, we don't need crypto/rand.

	return func(_ logr.Logger, req reconcile.Request) logr.Logger {
		return log.
			WithValues("controller", controllerName).
			WithValues("reconcileID", generateReconcileID(randSource)).
			WithValues(kind, klog.KRef(req.Namespace, req.Name))
	}
}

// logReconciler adds logging.
type logReconciler struct {
	Reconciler reconcile.Reconciler
	randSource *rand.Rand
}

// Reconciler creates a reconciles which wraps the current reconciler and adds logs & traces.
func Reconciler(reconciler reconcile.Reconciler) reconcile.Reconciler {
	var rngSeed int64
	_ = binary.Read(crand.Reader, binary.LittleEndian, &rngSeed)
	return &logReconciler{
		Reconciler: reconciler,
		randSource: rand.New(rand.NewSource(rngSeed)), //nolint:gosec // math/rand is enough, we don't need crypto/rand.
	}
}

// Reconcile
// FIXME: We should really make sure the log.Error in CR gets our k/v pairs too (currently reconcileID).
// * creating this span in CR.
// * disabling the error log in CR and doing it ourselves.
func (r *logReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	//// Add reconcileID to logger.
	//log := ctrl.LoggerFrom(ctx)
	//log = log.WithValues("reconcileID", generateReconcileID(r.randSource))
	//ctx = ctrl.LoggerInto(ctx, log)
	//
	//log.Info("Reconciliation started")
	//
	res, err := r.Reconciler.Reconcile(ctx, req)
	//if err != nil {
	//	log.Error(err, "Reconciliation finished with error")
	//} else {
	//	log.Info("Reconciliation finished")
	//}
	//
	return res, err
}

func generateReconcileID(randSource *rand.Rand) string {
	id := [16]byte{}
	_, _ = randSource.Read(id[:])
	return hex.EncodeToString(id[:])
}

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
