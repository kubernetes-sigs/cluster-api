/*
Copyright 2023 The Kubernetes Authors.

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

package predicates

import (
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// LabelMatcher holds initialized Label Selector predicate for evaluation of incoming
// objects on events and filtering which objects to reconcile.
type LabelMatcher struct {
	selector  labels.Selector
	predicate predicate.Predicate
}

// ComposeFilterExpression will return a valid label selector string representation
// by converting watchFilter to a default for watchExpression.
func ComposeFilterExpression(watchExpression, watchFilter string) string {
	if watchExpression == "" && watchFilter != "" {
		return fmt.Sprintf("%s = %s", clusterv1.WatchLabel, watchFilter)
	}
	return watchExpression
}

// InitLabelMatcher initializes expression which will apply on all processed objects in events in every controller.
func InitLabelMatcher(log logr.Logger, labelExpression string) (LabelMatcher, error) {
	matcher := LabelMatcher{}
	if labelExpression == "" {
		return matcher, nil
	}

	expr, err := metav1.ParseToLabelSelector(labelExpression)
	if err != nil {
		log.Error(err, fmt.Sprintf("Unable to compile LabelSelector from %s", labelExpression))
		return matcher, err
	}
	log.Info("Initialized label selector predicate for the given rule: ", labelExpression)

	labelPredicate, err := predicate.LabelSelectorPredicate(*expr)
	if err != nil {
		log.Error(err, fmt.Sprintf("Unable to compile label selector for %s", labelExpression))
		return matcher, err
	}

	selector, err := metav1.LabelSelectorAsSelector(expr)
	if err != nil {
		log.Error(err, "Unable to parse label selector for cache filtering for %s", labelExpression)
		return matcher, err
	}

	matcher.selector = selector
	matcher.predicate = labelPredicate
	return matcher, nil
}

// Selector returns compiled label selector if it was initialized from expression.
func (m *LabelMatcher) Selector() labels.Selector {
	return m.selector
}

// Matches returns a predicate that accepts objects only matching
// watch-filter or watch-expression value.
func (m *LabelMatcher) Matches(log logr.Logger) predicate.Predicate {
	if m.predicate == nil {
		log.Info("Skipping filter apply, no label expression was set")
		return predicate.Funcs{}
	}

	return m.predicate
}
