/*
Copyright 2022 The Kubernetes Authors.

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

// Package trace implements utilities for tracing.
package trace

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/component-base/traces"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// traceReconciler adds tracing.
type traceReconciler struct {
	Reconciler     reconcile.Reconciler
	Tracer         trace.Tracer
	controllerName string
	kind           string
}

// Reconciler creates a reconciles which wraps the current reconciler and adds logs & traces.
func Reconciler(reconciler reconcile.Reconciler, traceProvider trace.TracerProvider, reconcilerName, kind string) reconcile.Reconciler {
	return &traceReconciler{
		Reconciler:     reconciler,
		Tracer:         traceProvider.Tracer("capi"),
		controllerName: reconcilerName,
		kind:           kind,
	}
}

// Reconcile
// FIXME: Open issue: we should really make sure the log.Error in CR gets the trace id too, by either:
// * creating this span in CR.
// * disabling the error log in CR and doing it ourselves.
func (r *traceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, span := r.Tracer.Start(ctx, fmt.Sprintf("%s.Reconcile", r.controllerName),
		trace.WithAttributes(
			attribute.String(r.kind, klog.KRef(req.Namespace, req.Name).String()),
			attribute.String("controller", r.controllerName),
		))
	defer span.End()

	// Add traceID to logger.
	log := ctrl.LoggerFrom(ctx)
	log = log.WithValues("traceID", span.SpanContext().TraceID().String())
	ctx = ctrl.LoggerInto(ctx, log)

	log.Info("Reconciliation started")

	res, err := r.Reconciler.Reconcile(ctx, req)
	if err != nil {
		span.SetAttributes(attribute.String("error", "true"))
		log.Error(err, "Reconciliation finished with error")
	} else {
		log.Info("Reconciliation finished")
	}

	return res, err
}

// NewProvider provides a NewProvider.
func NewProvider(tracingEndpoint string, tracingSamplingRatePerMillion int, service string) trace.TracerProvider {
	opts := []otlpgrpc.Option{}
	if tracingEndpoint != "" {
		opts = append(opts, otlpgrpc.WithEndpoint(tracingEndpoint))
	}
	sampler := sdktrace.NeverSample()
	if tracingSamplingRatePerMillion > 0 {
		sampler = sdktrace.TraceIDRatioBased(float64(tracingSamplingRatePerMillion) / float64(1000000))
	}
	resourceOpts := []resource.Option{
		// FIXME: just testing.
		resource.WithProcess(),
		// Check https://github.com/open-telemetry/opentelemetry-operator/blob/main/pkg/instrumentation/sdk.go#L101
		// for how to get k8s attributes
		resource.WithAttributes(
			// ~ aligned to kube-apiserver
			attribute.Key("app").String(service),
			semconv.ServiceNameKey.String(service),
			// This should probably be the pod name
			semconv.ServiceInstanceIDKey.String(service+uuid.New().String()),
		),
	}
	return traces.NewProvider(context.Background(), sampler, resourceOpts, opts...)
}
