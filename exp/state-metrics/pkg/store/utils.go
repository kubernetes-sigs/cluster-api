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

/*
The original file is located at [1].
[1]: https://github.com/kubernetes/kube-state-metrics/blob/813c85ae4efa/internal/store/testutils.go

The original source was adjusted to remove unused functions as well as to adjust
the import alias for `k8s.io/api/core/v1`.
*/

package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	"k8s.io/kube-state-metrics/v2/pkg/options"
)

var (
	invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	matchAllCap        = regexp.MustCompile("([a-z0-9])([A-Z])")
	conditionStatuses  = []corev1.ConditionStatus{corev1.ConditionTrue, corev1.ConditionFalse, corev1.ConditionUnknown}
)

func boolFloat64(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// addConditionMetrics generates one metric for each possible condition
// status. For this function to work properly, the last label in the metric
// description must be the condition.
func addConditionMetrics(cs corev1.ConditionStatus) []*metric.Metric {
	ms := make([]*metric.Metric, len(conditionStatuses))

	for i, status := range conditionStatuses {
		ms[i] = &metric.Metric{
			LabelValues: []string{strings.ToLower(string(status))},
			Value:       boolFloat64(cs == status),
		}
	}

	return ms
}

func kubeLabelsToPrometheusLabels(labels map[string]string) ([]string, []string) {
	return mapToPrometheusLabels(labels, "label")
}

func mapToPrometheusLabels(labels map[string]string, prefix string) ([]string, []string) {
	labelKeys := make([]string, 0, len(labels))
	labelValues := make([]string, 0, len(labels))

	sortedKeys := make([]string, 0)
	for key := range labels {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	// conflictDesc holds some metadata for resolving potential label conflicts
	type conflictDesc struct {
		// the number of conflicting label keys we saw so far
		count int

		// the offset of the initial conflicting label key, so we could
		// later go back and rename "label_foo" to "label_foo_conflict1"
		initial int
	}

	conflicts := make(map[string]*conflictDesc)
	for _, k := range sortedKeys {
		labelKey := labelName(prefix, k)
		if conflict, ok := conflicts[labelKey]; ok {
			if conflict.count == 1 {
				// this is the first conflict for the label,
				// so we have to go back and rename the initial label that we've already added
				labelKeys[conflict.initial] = labelConflictSuffix(labelKeys[conflict.initial], conflict.count)
			}

			conflict.count++
			labelKey = labelConflictSuffix(labelKey, conflict.count)
		} else {
			// we'll need this info later in case there are conflicts
			conflicts[labelKey] = &conflictDesc{
				count:   1,
				initial: len(labelKeys),
			}
		}
		labelKeys = append(labelKeys, labelKey)
		labelValues = append(labelValues, labels[k])
	}
	return labelKeys, labelValues
}

func labelName(prefix, labelName string) string {
	return prefix + "_" + lintLabelName(sanitizeLabelName(labelName))
}

func sanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}

func lintLabelName(s string) string {
	return toSnakeCase(s)
}

func toSnakeCase(s string) string {
	snake := matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}

func labelConflictSuffix(label string, count int) string {
	return fmt.Sprintf("%s_conflict%d", label, count)
}

// createLabelKeysValues takes in passed kubernetes labels and allowed list in kubernetes label format
// it returns only those allowed labels that exist in the list converting them to Prometheus labels.
func createLabelKeysValues(allKubeLabels map[string]string, allowList []string) ([]string, []string) {
	allowedKubeLabels := make(map[string]string)

	if len(allowList) > 0 {
		if allowList[0] == options.LabelWildcard {
			return kubeLabelsToPrometheusLabels(allKubeLabels)
		}

		for _, l := range allowList {
			v, found := allKubeLabels[l]
			if found {
				allowedKubeLabels[l] = v
			}
		}
	}
	return kubeLabelsToPrometheusLabels(allowedKubeLabels)
}
