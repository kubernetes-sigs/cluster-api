//go:build tools
// +build tools

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

package main

// notesGenerator orchestrates the release notes generation.
// Lists the selected PRs for this collection of notes,
// process them to generate one entry per PR and then
// formats and prints the results.
type notesGenerator struct {
	lister    prLister
	processor prProcessor
	printer   entriesPrinter
}

func newNotesGenerator(lister prLister, processor prProcessor, printer entriesPrinter) *notesGenerator {
	return &notesGenerator{
		lister:    lister,
		processor: processor,
		printer:   printer,
	}
}

// PR is a GitHub PR.
type pr struct {
	number uint64
	title  string
	labels []string
}

// prLister returns a list of PRs.
type prLister interface {
	listPRs() ([]pr, error)
}

// notesEntry represents a line item for the release notes.
type notesEntry struct {
	title    string
	section  string
	prNumber string
}

// prProcessor generates notes entries for a list of PRs.
type prProcessor interface {
	process([]pr) []notesEntry
}

// entriesPrinter formats and outputs to stdout the notes
// based on a list of entries.
type entriesPrinter interface {
	print([]notesEntry)
}

// run generates and prints the notes.
func (g *notesGenerator) run() error {
	prs, err := g.lister.listPRs()
	if err != nil {
		return err
	}

	entries := g.processor.process(prs)

	g.printer.print(entries)

	return nil
}
