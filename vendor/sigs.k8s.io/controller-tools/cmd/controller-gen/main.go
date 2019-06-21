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
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-tools/pkg/crd"
	"sigs.k8s.io/controller-tools/pkg/deepcopy"
	"sigs.k8s.io/controller-tools/pkg/genall"
	"sigs.k8s.io/controller-tools/pkg/markers"
	"sigs.k8s.io/controller-tools/pkg/rbac"
	"sigs.k8s.io/controller-tools/pkg/webhook"
)

// Options are specified to controller-gen by turning generators and output rules into
// markers, and then parsing them using the standard registry logic (without the "+").
// Each marker and output rule should thus be usable as a marker target.

var (
	// allGenerators maintains the list of all known generators, giving
	// them names for use on the command line.
	// each turns into a command line option,
	// and has options for output forms.
	allGenerators = map[string]genall.Generator{
		"crd":     crd.Generator{},
		"rbac":    rbac.Generator{},
		"object":  deepcopy.Generator{},
		"webhook": webhook.Generator{},
	}

	// allOutputRules defines the list of all known output rules, giving
	// them names for use on the command line.
	// Each output rule turns into two command line options:
	// - output:<generator>:<form> (per-generator output)
	// - output:<form> (default output)
	allOutputRules = map[string]genall.OutputRule{
		"dir":       genall.OutputToDirectory(""),
		"none":      genall.OutputToNothing,
		"stdout":    genall.OutputToStdout,
		"artifacts": genall.OutputArtifacts{},
	}

	// optionsRegistry contains all the marker definitions used to process command line options
	optionsRegistry = &markers.Registry{}
)

// rootPaths is the marker value for the "paths" option
type rootPaths []string

func init() {
	for genName, gen := range allGenerators {
		// make the generator options marker itself
		if err := optionsRegistry.Define(genName, markers.DescribesPackage, gen); err != nil {
			panic(err)
		}

		// make per-generation output rule markers
		for ruleName, rule := range allOutputRules {
			if err := optionsRegistry.Define(fmt.Sprintf("output:%s:%s", genName, ruleName), markers.DescribesPackage, rule); err != nil {
				panic(err)
			}
		}
	}

	// make "default output" output rule markers
	for ruleName, rule := range allOutputRules {
		if err := optionsRegistry.Define("output:"+ruleName, markers.DescribesPackage, rule); err != nil {
			panic(err)
		}
	}

	// make the "paths" rule
	if err := optionsRegistry.Define("paths", markers.DescribesPackage, rootPaths(nil)); err != nil {
		panic(err)
	}
}

// fieldHelp prints the help for a particular marker argument.
func fieldHelp(name string, arg markers.Argument, first bool, out *strings.Builder) {
	if arg.Optional {
		out.WriteString("[")
	}

	if !first {
		out.WriteRune(',')
	} else if name != "" {
		out.WriteRune(':')
	}

	if name != "" {
		out.WriteString(name)
	}
	out.WriteRune('=')

	out.WriteRune('<')
	out.WriteString(arg.TypeString())
	out.WriteRune('>')

	if arg.Optional {
		out.WriteString("]")
	}
}

// optionHelp assembles help for a given named option (specified via marker).
func optionHelp(def *markers.Definition, out *strings.Builder) {
	out.WriteString(def.Name)
	if def.Empty() {
		return
	}

	first := true
	for argName, arg := range def.Fields {
		fieldHelp(argName, arg, first, out)
		first = false
	}
}

// allOptionsHelp documents all accepted options
func allOptionsHelp(reg *markers.Registry, descTarget bool, sortFunc func(string, string) bool) string {
	out := &strings.Builder{}
	allMarkers := reg.AllDefinitions()
	sort.Slice(allMarkers, func(i, j int) bool {
		// sort grouping marker options together
		iName, jName := allMarkers[i].Name, allMarkers[j].Name
		return sortFunc(iName, jName)
	})
	for _, def := range allMarkers {
		out.WriteString("  ")
		optionHelp(def, out)
		if descTarget {
			switch def.Target {
			case markers.DescribesPackage:
				out.WriteString(" (package)")
			case markers.DescribesType:
				out.WriteString(" (type)")
			case markers.DescribesField:
				out.WriteString(" (struct field)")
			}
		}
		out.WriteRune('\n')
	}
	return out.String()
}

func compOptionsForSort(iName, jName string) bool {
	iParts := strings.Split(iName, ":")
	jParts := strings.Split(jName, ":")

	iGen := ""
	iRule := ""
	jGen := ""
	jRule := ""

	switch len(iParts) {
	case 1:
		iGen = iParts[0]
	// two means a default output rule, so ignore
	case 2:
		iRule = iParts[1]
	case 3:
		iGen = iParts[1]
		iRule = iParts[2]
	}
	switch len(jParts) {
	case 1:
		jGen = jParts[0]
	// two means a default output rule, so ignore
	case 2:
		jRule = jParts[1]
	case 3:
		jGen = jParts[1]
		jRule = jParts[2]
	}

	if iGen != jGen {
		return iGen > jGen
	}

	return iRule < jRule
}

func main() {
	printMarkersOnly := false
	cmd := &cobra.Command{
		Use:   "controller-gen",
		Short: "Generate Kubernetes API extension resources and code.",
		Long:  "Generate Kubernetes API extension resources and code.",
		Example: `	# Generate RBAC manifests and crds for all types under apis/,
	# outputting crds to /tmp/crds and everything else to stdout
	controller-gen rbac:roleName=<role name> crd paths=./apis/... output:crd:dir=/tmp/crds output:stdout

	# Generate deepcopy implementations for a particular file
	controller-gen deepcopy paths=./apis/v1beta1/some_types.go

	# Run all the generators for a given project
	controller-gen paths=./apis/...
`,
		RunE: func(_ *cobra.Command, rawOpts []string) error {
			return runGenerators(rawOpts, printMarkersOnly)
		},
	}
	cmd.Flags().BoolVarP(&printMarkersOnly, "which-markers", "w", false, "print out all markers available with the requested generators")
	cmd.SetUsageTemplate(cmd.UsageTemplate() + "\n\nOptions\n\n" + allOptionsHelp(optionsRegistry, false, compOptionsForSort))

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// splitOutputRuleName splits a marker name of "output:rule:gen" or "output:rule"
// into its compontent rule and generator name.
func splitOutputRuleOption(name string) (ruleName string, genName string) {
	parts := strings.SplitN(name, ":", 3)
	if len(parts) == 3 {
		// output:<generator>:<rule>
		return parts[2], parts[1]
	}
	// output:<rule>
	return parts[1], ""
}

// parseOptions parses out options as markers into generators and output rules
// (the only current types of options).
func parseOptions(options []string) (genall.Generators, genall.OutputRules, []string, error) {
	var gens genall.Generators
	rules := genall.OutputRules{
		ByGenerator: make(map[genall.Generator]genall.OutputRule),
	}
	var paths []string

	// collect the generators first, so that we can key the output on the actual
	// generator, which matters if there's settings in the gen object and it's not a pointer.
	outputByGen := make(map[string]genall.OutputRule)
	gensByName := make(map[string]genall.Generator)

	for _, rawOpt := range options {
		rawOpt = "+" + rawOpt // add a `+` to make it acceptable for usage with the registry
		defn := optionsRegistry.Lookup(rawOpt, markers.DescribesPackage)
		if defn == nil {
			return nil, genall.OutputRules{}, nil, fmt.Errorf("unknown option %q", rawOpt[1:])
		}

		val, err := defn.Parse(rawOpt)
		if err != nil {
			return nil, genall.OutputRules{}, nil, fmt.Errorf("unable to parse option %q: %v", rawOpt[1:], err)
		}

		switch val := val.(type) {
		case genall.Generator:
			gens = append(gens, val)
			gensByName[defn.Name] = val
		case genall.OutputRule:
			_, genName := splitOutputRuleOption(defn.Name)
			if genName == "" {
				// it's a default rule
				rules.Default = val
				continue
			}

			outputByGen[genName] = val
			continue
		case rootPaths:
			paths = val
		default:
			return nil, genall.OutputRules{}, nil, fmt.Errorf("unknown option marker %q", defn.Name)
		}
	}

	// if no generators were specified, we want all of them
	if len(gens) == 0 {
		for genName, gen := range allGenerators {
			gens = append(gens, gen)
			gensByName[genName] = gen
		}
	}

	// actually associate the rules now that we know the generators
	for genName, outputRule := range outputByGen {
		gen, knownGen := gensByName[genName]
		if !knownGen {
			return nil, genall.OutputRules{}, nil, fmt.Errorf("non-invoked generator %q", genName)
		}

		rules.ByGenerator[gen] = outputRule
	}

	// attempt to figure out what the user wants without a lot of verbose specificity:
	// if the user specifies a default rule, assume that they probably want to fall back
	// to that.  Otherwise, assume that they just wanted to customize one option from the
	// set, and leave the rest in the standard configuration.
	outRules := genall.DirectoryPerGenerator("config", gensByName)
	if rules.Default != nil {
		return gens, rules, paths, nil
	}

	for gen, rule := range rules.ByGenerator {
		outRules.ByGenerator[gen] = rule
	}

	return gens, outRules, paths, nil
}

func runGenerators(rawOptions []string, printMarkersOnly bool) error {
	gens, outputRules, rootPaths, err := parseOptions(rawOptions)
	if err != nil {

		return err
	}

	if printMarkersOnly {
		reg := &markers.Registry{}
		if err := gens.RegisterMarkers(reg); err != nil {
			return err
		}
		sortFunc := func(iName string, jName string) bool { return iName < jName }
		fmt.Println(allOptionsHelp(reg, true, sortFunc))
		return nil
	}

	rt, err := gens.ForRoots(rootPaths...)
	if err != nil {
		return err
	}
	rt.OutputRules = outputRules

	if hadErrs := rt.Run(); hadErrs {
		return fmt.Errorf("not all generators ran succesfully")
	}

	return nil
}
