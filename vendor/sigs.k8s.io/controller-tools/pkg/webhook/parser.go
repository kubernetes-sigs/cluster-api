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

// Package webhook contains libraries for generating webhookconfig manifests
// from markers in Go source files.
//
// The markers take the form:
//
//  +kubebuilder:webhook:failurePolicy=<string>,groups=<[]string>,resources=<[]string>,verbs=<[]string>,versions=<[]string>,name=<string>,path=<string>,mutating=<bool>
package webhook

import (
	"strings"

	admissionreg "k8s.io/api/admissionregistration/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

var (
	// ConfigDefinition s a marker for defining Webhook manifests.
	// Call ToWebhook on the value to get a Kubernetes Webhook.
	ConfigDefinition = markers.Must(markers.MakeDefinition("kubebuilder:webhook", markers.DescribesPackage, Config{}))
)

// Config is a marker value that describes a kubernetes Webhook config.
type Config struct {
	Mutating      bool
	FailurePolicy string

	Groups    []string
	Resources []string
	Verbs     []string
	Versions  []string

	Name string
	Path string
}

// verbToAPIVariant converts a marker's verb to the proper value for the API.
// Unrecognized verbs are passed through.
func verbToAPIVariant(verbRaw string) admissionreg.OperationType {
	switch strings.ToLower(verbRaw) {
	case strings.ToLower(string(admissionreg.Create)):
		return admissionreg.Create
	case strings.ToLower(string(admissionreg.Update)):
		return admissionreg.Update
	case strings.ToLower(string(admissionreg.Delete)):
		return admissionreg.Delete
	case strings.ToLower(string(admissionreg.Connect)):
		return admissionreg.Connect
	case strings.ToLower(string(admissionreg.OperationAll)):
		return admissionreg.OperationAll
	default:
		return admissionreg.OperationType(verbRaw)
	}
}

// ToRule converts this rule to its Kubernetes API form.
func (c Config) ToWebhook() admissionreg.Webhook {
	whConfig := admissionreg.RuleWithOperations{
		Rule: admissionreg.Rule{
			APIGroups:   c.Groups,
			APIVersions: c.Versions,
			Resources:   c.Resources,
		},
		Operations: make([]admissionreg.OperationType, len(c.Verbs)),
	}

	for i, verbRaw := range c.Verbs {
		whConfig.Operations[i] = verbToAPIVariant(verbRaw)
	}

	// fix the group names, since letting people type "core" is nice
	for i, group := range whConfig.APIGroups {
		if group == "core" {
			whConfig.APIGroups[i] = ""
		}
	}

	var failurePolicy admissionreg.FailurePolicyType
	switch strings.ToLower(c.FailurePolicy) {
	case strings.ToLower(string(admissionreg.Ignore)):
		failurePolicy = admissionreg.Ignore
	case strings.ToLower(string(admissionreg.Fail)):
		failurePolicy = admissionreg.Fail
	default:
		failurePolicy = admissionreg.FailurePolicyType(c.FailurePolicy)
	}
	path := c.Path
	return admissionreg.Webhook{
		Name:          c.Name,
		Rules:         []admissionreg.RuleWithOperations{whConfig},
		FailurePolicy: &failurePolicy,
		ClientConfig: admissionreg.WebhookClientConfig{
			Service: &admissionreg.ServiceReference{
				Name:      "webhook-service",
				Namespace: "system",
				Path:      &path,
			},
			// OpenAPI marks the field as required before 1.13 because of a bug that got fixed in
			// https://github.com/kubernetes/api/commit/e7d9121e9ffd63cea0288b36a82bcc87b073bd1b
			// Put "\n" as an placeholder as a workaround til 1.13+ is almost everywhere.
			CABundle: []byte("\n"),
		},
	}
}

// Generator is a genall.Generator that generates Webhook manifests.
type Generator struct{}

func (Generator) RegisterMarkers(into *markers.Registry) error {
	return into.Register(ConfigDefinition)
}

func (Generator) Generate(ctx *genall.GenerationContext) error {
	var mutatingCfgs []admissionreg.Webhook
	var validatingCfgs []admissionreg.Webhook
	for _, root := range ctx.Roots {
		markerSet, err := markers.PackageMarkers(ctx.Collector, root)
		if err != nil {
			root.AddError(err)
		}

		for _, cfg := range markerSet[ConfigDefinition.Name] {
			cfg := cfg.(Config)
			if cfg.Mutating {
				mutatingCfgs = append(mutatingCfgs, cfg.ToWebhook())
			} else {
				validatingCfgs = append(validatingCfgs, cfg.ToWebhook())
			}
		}
	}

	var objs []interface{}
	if len(mutatingCfgs) > 0 {
		objs = append(objs, &admissionreg.MutatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				Kind:       "MutatingWebhookConfiguration",
				APIVersion: admissionreg.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "mutating-webhook-configuration",
			},
			Webhooks: mutatingCfgs,
		})
	}

	if len(validatingCfgs) > 0 {
		objs = append(objs, &admissionreg.ValidatingWebhookConfiguration{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ValidatingWebhookConfiguration",
				APIVersion: admissionreg.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "validating-webhook-configuration",
			},
			Webhooks: validatingCfgs,
		})

	}

	if err := ctx.WriteYAML("manifests.yaml", objs...); err != nil {
		return err
	}

	return nil
}
