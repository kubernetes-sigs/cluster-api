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

package controllers

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	configv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/utils/pointer"

	operatorv1 "sigs.k8s.io/cluster-api/exp/operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

func intPtr(v int) *int {
	return &v
}

func TestCustomizeDeployment(t *testing.T) {
	sevenHours, _ := time.ParseDuration("7h")
	memTestQuantity, _ := resource.ParseQuantity("16Gi")
	managerDepl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manager",
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "manager",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "manager",
						Image: "k8s.gcr.io/a-manager:1.6.2",
						Env: []corev1.EnvVar{
							{
								Name:  "test1",
								Value: "value1",
							},
						},
						Args: []string{"--webhook-port=2345"},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromString("healthz"),
								},
							},
						},
						ReadinessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readyz",
									Port: intstr.FromString("healthz"),
								},
							},
						},
					}},
				},
			},
		},
	}
	tests := []struct {
		name  string
		dSpec *operatorv1.DeploymentSpec
		mSpec *operatorv1.ManagerSpec
		dIn   *appsv1.Deployment
		dOut  *appsv1.Deployment
	}{
		{
			name:  "empty",
			dSpec: &operatorv1.DeploymentSpec{},
			dIn:   managerDepl.DeepCopy(),
			dOut:  managerDepl,
		},
		{
			name: "all deployment options",
			dSpec: &operatorv1.DeploymentSpec{
				NodeSelector: map[string]string{"a": "b"},
				Tolerations: []corev1.Toleration{
					{
						Key:    "node-role.kubernetes.io/master",
						Effect: corev1.TaintEffectNoSchedule,
					},
				},
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
							NodeSelectorTerms: []corev1.NodeSelectorTerm{
								{
									MatchExpressions: []corev1.NodeSelectorRequirement{},
								},
							},
						},
					},
				},
				Containers: []operatorv1.ContainerSpec{
					{
						Name: "manager",
						Image: &operatorv1.ImageMeta{
							Name:       pointer.StringPtr("mydns"),
							Repository: pointer.StringPtr("quay.io/dev"),
							Tag:        pointer.StringPtr("v3.4.2"),
						},
						Env: []corev1.EnvVar{
							{
								Name:  "test1",
								Value: "value2",
							},
							{
								Name:  "new1",
								Value: "value22",
							},
						},
						Args: map[string]string{
							"--webhook-port": "3456",
							"--log_dir":      "/var/log",
						},
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceMemory: memTestQuantity},
						},
					},
				},
			},
			dIn: managerDepl.DeepCopy(),
			dOut: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manager",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name: "manager",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "manager",
									Image: "quay.io/dev/mydns:v3.4.2",
									Env: []corev1.EnvVar{
										{
											Name:  "test1",
											Value: "value2",
										},
										{
											Name:  "new1",
											Value: "value22",
										},
									},
									Args: []string{"--webhook-port=3456", "--log_dir=/var/log"},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{corev1.ResourceMemory: memTestQuantity},
									},
									LivenessProbe: &corev1.Probe{
										Handler: corev1.Handler{
											HTTPGet: &corev1.HTTPGetAction{
												Path: "/healthz",
												Port: intstr.FromString("healthz"),
											},
										},
									},
									ReadinessProbe: &corev1.Probe{
										Handler: corev1.Handler{
											HTTPGet: &corev1.HTTPGetAction{
												Path: "/readyz",
												Port: intstr.FromString("healthz"),
											},
										},
									},
								},
							},
							NodeSelector: map[string]string{"a": "b"},
							Tolerations: []corev1.Toleration{
								{
									Key:    "node-role.kubernetes.io/master",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
							Affinity: &corev1.Affinity{
								NodeAffinity: &corev1.NodeAffinity{
									RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
										NodeSelectorTerms: []corev1.NodeSelectorTerm{
											{
												MatchExpressions: []corev1.NodeSelectorRequirement{},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "all manager options",
			mSpec: &operatorv1.ManagerSpec{
				Debug:           true,
				FeatureGates:    map[string]bool{"TEST": true, "ANOTHER": false},
				ProfilerAddress: pointer.StringPtr("localhost:1234"),
				ControllerManagerConfigurationSpec: v1alpha1.ControllerManagerConfigurationSpec{
					CacheNamespace: "testNS",
					SyncPeriod:     &metav1.Duration{Duration: sevenHours},
					Controller:     &v1alpha1.ControllerConfigurationSpec{GroupKindConcurrency: map[string]int{"machine": 3}},
					Metrics:        v1alpha1.ControllerMetrics{BindAddress: ":4567"},
					Health: v1alpha1.ControllerHealth{
						HealthProbeBindAddress: ":6789",
						ReadinessEndpointName:  "readyish",
						LivenessEndpointName:   "mostly",
					},
					Webhook: v1alpha1.ControllerWebhook{
						Port:    intPtr(3579),
						CertDir: "/tmp/certs",
					},
					LeaderElection: &configv1alpha1.LeaderElectionConfiguration{
						LeaderElect:       pointer.BoolPtr(true),
						ResourceName:      "foo",
						ResourceNamespace: "here",
						LeaseDuration:     metav1.Duration{Duration: sevenHours},
						RenewDeadline:     metav1.Duration{Duration: sevenHours},
						RetryPeriod:       metav1.Duration{Duration: sevenHours},
					},
				},
			},
			dIn: managerDepl.DeepCopy(),
			dOut: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "manager",
					Namespace: metav1.NamespaceSystem,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Name: "manager",
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "manager",
									Image: "k8s.gcr.io/a-manager:1.6.2",
									Env: []corev1.EnvVar{
										{
											Name:  "test1",
											Value: "value1",
										},
									},
									Args: []string{
										"--webhook-port=3579",
										"--machine-concurrency=3",
										"--namespace=testNS",
										"--health-addr=:6789",
										"--enable-leader-election=true",
										"--leader-election-id=here/foo",
										"--leader-elect-lease-duration=25200s",
										"--leader-elect-renew-deadline=25200s",
										"--leader-elect-retry-period=25200s",
										"--metrics-addr=:4567",
										"--webhook-cert-dir=/tmp/certs",
										"--sync-period=25200s",
										"--profiler-address=localhost:1234",
										"--v=5",
										"--feature-gates=ANOTHER=false,TEST=true"},
									LivenessProbe: &corev1.Probe{
										Handler: corev1.Handler{
											HTTPGet: &corev1.HTTPGetAction{
												Path: "/mostly",
												Port: intstr.FromString("healthz"),
											},
										},
									},
									ReadinessProbe: &corev1.Probe{
										Handler: corev1.Handler{
											HTTPGet: &corev1.HTTPGetAction{
												Path: "/readyish",
												Port: intstr.FromString("healthz"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customizeDeployment(operatorv1.ProviderSpec{
				Deployment: tt.dSpec,
				Manager:    tt.mSpec,
			}, tt.dIn)
			if !reflect.DeepEqual(tt.dIn.Spec, tt.dOut.Spec) {
				t.Error(cmp.Diff(tt.dOut.Spec, tt.dIn.Spec))
			}
		})
	}
}
