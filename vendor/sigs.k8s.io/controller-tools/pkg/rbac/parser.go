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

// Package rbac contain libraries for generating RBAC manifests from RBAC
// markers in Go source files.
//
// The markers take the form:
//
//  +kubebuilder:rbac:groups=<groups>,resources=<resources>,verbs=<verbs>,urls=<non resource urls>
package rbac

import (
	"sort"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/markers"
)

var (
	// RuleDefinition is a marker for defining RBAC rules.
	// Call ToRule on the value to get a Kubernetes RBAC policy rule.
	RuleDefinition = markers.Must(markers.MakeDefinition("kubebuilder:rbac", markers.DescribesPackage, Rule{}))
)

// Rule is a marker value that describes a kubernetes RBAC rule.
type Rule struct {
	Groups    []string `marker:",optional"`
	Resources []string `marker:",optional"`
	Verbs     []string
	URLs      []string `marker:"urls,optional"`
}

// ruleKey represents the resources and non-resources a Rule applies.
type ruleKey struct {
	Groups    string
	Resources string
	URLs      string
}

// key normalizes the Rule and returns a ruleKey object.
func (r *Rule) key() ruleKey {
	r.normalize()
	return ruleKey{
		Groups:    strings.Join(r.Groups, "&"),
		Resources: strings.Join(r.Resources, "&"),
		URLs:      strings.Join(r.URLs, "&"),
	}
}

// addVerbs adds new verbs into a Rule.
// The duplicates in `r.Verbs` will be removed, and then `r.Verbs` will be sorted.
func (r *Rule) addVerbs(verbs []string) {
	r.Verbs = removeDupAndSort(append(r.Verbs, verbs...))
}

// normalize removes duplicates from each field of a Rule, and sorts each field.
func (r *Rule) normalize() {
	r.Groups = removeDupAndSort(r.Groups)
	r.Resources = removeDupAndSort(r.Resources)
	r.Verbs = removeDupAndSort(r.Verbs)
	r.URLs = removeDupAndSort(r.URLs)
}

// removeDupAndSort removes duplicates in strs, sorts the items, and returns a
// new slice of strings.
func removeDupAndSort(strs []string) []string {
	set := make(map[string]bool)
	for _, str := range strs {
		if _, ok := set[str]; !ok {
			set[str] = true
		}
	}

	var result []string
	for str := range set {
		result = append(result, str)
	}
	sort.Strings(result)
	return result
}

// ToRule converts this rule to its Kubernetes API form.
func (r *Rule) ToRule() rbacv1.PolicyRule {
	// fix the group names first, since letting people type "core" is nice
	for i, group := range r.Groups {
		if group == "core" {
			r.Groups[i] = ""
		}
	}
	return rbacv1.PolicyRule{
		APIGroups:       r.Groups,
		Verbs:           r.Verbs,
		Resources:       r.Resources,
		NonResourceURLs: r.URLs,
	}
}

// Generator is a genall.Generator that generated RBAC manifests..
type Generator struct {
	RoleName string
}

func (Generator) RegisterMarkers(into *markers.Registry) error {
	return into.Register(RuleDefinition)
}

func (g Generator) Generate(ctx *genall.GenerationContext) error {
	rules := make(map[ruleKey]*Rule)
	for _, root := range ctx.Roots {
		markerSet, err := markers.PackageMarkers(ctx.Collector, root)
		if err != nil {
			root.AddError(err)
		}

		for _, markerValue := range markerSet[RuleDefinition.Name] {
			rule := markerValue.(Rule)
			key := rule.key()
			if _, ok := rules[key]; !ok {
				rules[key] = &rule
				continue
			}
			rules[key].addVerbs(rule.Verbs)
		}
	}

	var policyRules []rbacv1.PolicyRule
	for _, rule := range rules {
		policyRules = append(policyRules, rule.ToRule())

	}

	if len(policyRules) == 0 {
		return nil
	}

	if err := ctx.WriteYAML("role.yaml", rbacv1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: rbacv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: g.RoleName,
		},
		Rules: policyRules,
	}); err != nil {
		return err
	}

	return nil
}
