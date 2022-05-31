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

This original source was renamed to re-use the unit tests for functions in utils.go.
*/

package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

type generateMetricsTestCase struct {
	Obj             interface{}
	MetricNames     []string
	AllowLabelsList []string
	Want            string
	Headers         []string
	Func            func(interface{}) []metric.FamilyInterface
}

func (testCase *generateMetricsTestCase) run() error {
	metricFamilies := testCase.Func(testCase.Obj)
	metricFamilyStrings := []string{}
	for _, f := range metricFamilies {
		metricFamilyStrings = append(metricFamilyStrings, string(f.ByteSlice()))
	}
	metric := strings.Split(strings.Join(metricFamilyStrings, ""), "\n")
	filteredMetrics := filterMetricNames(metric, testCase.MetricNames)
	filteredHeaders := filterMetricNames(testCase.Headers, testCase.MetricNames)
	headers := strings.Join(filteredHeaders, "\n")
	metrics := strings.Join(filteredMetrics, "\n")
	out := headers + "\n" + metrics

	if err := compareOutput(testCase.Want, out); err != nil {
		return errors.Wrap(err, "expected wanted output to equal output")
	}

	return nil
}

func compareOutput(expected, actual string) error {
	entities := []string{expected, actual}
	// Align wanted and actual
	for i := 0; i < len(entities); i++ {
		for _, f := range []func(string) string{removeUnusedWhitespace, sortLabels, sortByLine} {
			entities[i] = f(entities[i])
		}
	}

	if entities[0] != entities[1] {
		return errors.Errorf("\nEXPECTED:\n--------------\n%v\nACTUAL:\n--------------\n%v", entities[0], entities[1])
	}

	return nil
}

// sortLabels sorts the order of labels in each line of the given metric. The
// Prometheus exposition format does not force ordering of labels. Hence a test
// should not fail due to different metric orders.
func sortLabels(s string) string {
	sorted := []string{}

	for _, line := range strings.Split(s, "\n") {
		// skipping if its headers
		if strings.HasPrefix(line, "# ") {
			sorted = append(sorted, line)
			continue
		}
		split := strings.Split(line, "{")
		if len(split) != 2 {
			panic(fmt.Sprintf("failed to sort labels in \"%v\"", line))
		}
		name := split[0]

		split = strings.Split(split[1], "}")
		value := split[1]

		labels := strings.Split(split[0], ",")
		sort.Strings(labels)

		sorted = append(sorted, fmt.Sprintf("%v{%v}%v", name, strings.Join(labels, ","), value))
	}

	return strings.Join(sorted, "\n")
}

func sortByLine(s string) string {
	split := strings.Split(s, "\n")
	sort.Strings(split)
	return strings.Join(split, "\n")
}

// filterMetricNames removes those metrics and headers that
// are not part of the names.
func filterMetricNames(ms []string, names []string) []string {
	// In case the test case is based on all returned metric, MetricNames does
	// not need to me defined.
	if names == nil {
		return ms
	}
	filtered := []string{}

	regexps := []*regexp.Regexp{}
	for _, n := range names {
		regexps = append(regexps, regexp.MustCompile(fmt.Sprintf(".*%v.*$", n)))
	}

	for _, m := range ms {
		drop := true
		for _, r := range regexps {
			if r.MatchString(m) {
				drop = false
				break
			}
		}
		if !drop {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

func removeUnusedWhitespace(s string) string {
	var (
		trimmedLine  string
		trimmedLines []string
		lines        = strings.Split(s, "\n")
	)

	for _, l := range lines {
		trimmedLine = strings.TrimSpace(l)

		if len(trimmedLine) > 0 {
			trimmedLines = append(trimmedLines, trimmedLine)
		}
	}

	return strings.Join(trimmedLines, "\n")
}
