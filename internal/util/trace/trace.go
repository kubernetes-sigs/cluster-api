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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	tracing "k8s.io/component-base/tracing"
	tracingapi "k8s.io/component-base/tracing/api/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// traceReconciler adds tracing.
type traceReconciler struct {
	Reconciler     reconcile.Reconciler
	Tracer         oteltrace.Tracer
	controllerName string
	kind           string
}

// Reconciler creates a reconciles which wraps the current reconciler and adds logs & traces.
func Reconciler(reconciler reconcile.Reconciler, tracerProvider oteltrace.TracerProvider, reconcilerName, kind string) reconcile.Reconciler {
	return &traceReconciler{
		Reconciler:     reconciler,
		Tracer:         tracerProvider.Tracer(reconcilerName),
		controllerName: reconcilerName,
		kind:           kind,
	}
}

// Reconcile
// FIXME: Open issue: we should really make sure the log.Error in CR gets the trace id too, by either:
// * creating this span in CR.
// * disabling the error log in CR and doing it ourselves.
func (r *traceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, span := r.Tracer.Start(ctx, fmt.Sprintf("%s.Reconciler.Reconcile", r.controllerName),
		oteltrace.WithAttributes(
			attribute.String(r.kind, klog.KRef(req.Namespace, req.Name).String()),
			attribute.String("controller", r.controllerName),
			attribute.String("reconcileID", string(controller.ReconcileIDFromContext(ctx))),
		),
	)
	defer span.End()

	// Add tracer to context.
	ctx = addTracerToContext(ctx, r.Tracer)

	// Add traceID to logger.
	log := ctrl.LoggerFrom(ctx)
	log = log.WithValues("traceID", span.SpanContext().TraceID().String())
	ctx = ctrl.LoggerInto(ctx, log)

	res, err := r.Reconciler.Reconcile(ctx, req)
	if err != nil {
		span.SetAttributes(attribute.String("error", "true"))
	}

	return res, err
}

// NewProvider provides a NewProvider.
func NewProvider(tracingConfiguration *tracingapi.TracingConfiguration, service, appName string) (oteltrace.TracerProvider, error) {
	if tracingConfiguration == nil || tracingConfiguration.Endpoint == nil || *tracingConfiguration.Endpoint == "" {
		return oteltrace.NewNoopTracerProvider(), nil
	}

	resourceOpts := []otelsdkresource.Option{
		// FIXME: reevaluate
		otelsdkresource.WithProcess(),
		otelsdkresource.WithAttributes(
			// ~ aligned to kube-apiserver
			attribute.Key("app").String(appName),
			// semconv.HostNameKey.String(hostname),
			semconv.ServiceNameKey.String(service),
			// This should probably be the pod name
			// semconv.ServiceInstanceIDKey.String(service+uuid.New().String()),
		),
	}
	tp, err := tracing.NewProvider(context.Background(), tracingConfiguration, []otlptracegrpc.Option{}, resourceOpts)
	if err != nil {
		return nil, fmt.Errorf("could not configure tracer provider: %w", err)
	}
	return tp, nil
}

// tracerFromContext gets an oteltrace.Tracer from the current context.
func tracerFromContext(ctx context.Context) oteltrace.Tracer {
	r, ok := ctx.Value(tracerKey{}).(oteltrace.Tracer)
	if !ok {
		return oteltrace.NewNoopTracerProvider().Tracer("noop")
	}

	return r
}

// tracerKey is a context.Context Value key. Its associated value should
// be a types.UID.
type tracerKey struct{}

func addTracerToContext(ctx context.Context, tracer oteltrace.Tracer) context.Context {
	return context.WithValue(ctx, tracerKey{}, tracer)
}

// Span is a span.
type Span struct {
	span oteltrace.Span
	// FIXME(sbueringer) skipping go tracing for now
	// task *trace.Task //nolint:gocritic
}

// Start starts a span.
func Start(ctx context.Context, name string) (context.Context, *Span) {
	tracer := tracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, name)

	// ctx, task := trace.NewTask(ctx, name) //nolint:gocritic
	return ctx, &Span{
		span: span,
		//task: task,
	}
}

// End ends a span.
func (s *Span) End() {
	s.span.End()
	// s.task.End() //nolint:gocritic
}

// AddEvent adds an event with the provided name and options.
func (s *Span) AddEvent(name string, options ...oteltrace.EventOption) {
	s.span.AddEvent(name, options...)
}
