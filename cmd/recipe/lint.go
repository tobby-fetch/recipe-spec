// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	v1alpha1 "github.com/tobby-fetch/recipe-spec/recipe/v1alpha1"
)

// The lint subcommand is a thin wrapper around the SDK: it expands the
// command line into files, splits multi-document streams (RECIPE-SPEC.md
// §5), and reports what Parse and Validate return. Every validation rule
// lives in recipe/v1alpha1; none is duplicated here.

const lintUsage = `usage: recipe lint [-profile draft|cooked] [-output text|json] <file|dir>...

Parses and validates every given document against the Recipe specification
(apiVersion recipe.tobby.dev/v1alpha1), via the recipe-spec Go SDK. A
directory argument stands for every *.yaml and *.yml file under it,
recursively. Multi-document streams ("---" separators) are accepted; each
document is validated independently (§5).

Exit code: 0 when every document is valid, 1 when there are findings,
2 on a usage or I/O error — including when the arguments name no recipe
document at all (an empty directory is not a valid input, it is a mistake).

Flags:
`

// finding is one reported violation, in a shape that is stable for
// machine consumption (-output json) and rendered one per line as text.
type finding struct {
	// File is the path of the offending file, as derived from the
	// command line.
	File string `json:"file"`
	// Document is the 1-based index of the offending document within the
	// file's YAML stream; 1 for single-document files.
	Document int `json:"document"`
	// Path, Rule, and Message mirror the SDK's FieldError.
	Path    string `json:"path"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// fileResult is the outcome of linting one file.
type fileResult struct {
	findings  []finding
	documents int  // documents checked (empty stream entries excluded)
	multiDoc  bool // the file is a multi-document stream
}

// runLint implements "recipe lint".
func runLint(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("recipe lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profile := flags.String("profile", string(v1alpha1.ProfileDraft),
		`validation profile for Recipe documents: "draft" or "cooked" (§8); Retriever documents have no profiles and always validate the same way`)
	output := flags.String("output", "text",
		`report format: "text" (one line per finding plus a summary) or "json" (a stable array of {file, document, path, rule, message})`)
	flags.Usage = func() {
		fprint(flags.Output(), lintUsage)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	switch v1alpha1.Profile(*profile) {
	case v1alpha1.ProfileDraft, v1alpha1.ProfileCooked:
	default:
		fprintf(stderr, "recipe lint: unknown profile %q: use draft or cooked\n", *profile)
		return exitUsage
	}
	if *output != "text" && *output != "json" {
		fprintf(stderr, "recipe lint: unknown output format %q: use text or json\n", *output)
		return exitUsage
	}
	if flags.NArg() == 0 {
		fprint(stderr, "recipe lint: no file or directory given\n\n")
		flags.Usage()
		return exitUsage
	}

	files, ioErr := expandArgs(flags.Args(), stderr)
	if len(files) == 0 {
		// Arguments that resolve to nothing — typically a directory with
		// no *.yaml or *.yml under it. "0 file(s) checked: all valid"
		// with exit 0 would let a mistyped path pass a CI gate; this is
		// an input error, and it exits like one.
		fprint(stderr, "recipe lint: no recipe documents found in the given arguments\n")
		return exitUsage
	}
	var findings []finding
	multiDoc := make(map[string]bool)
	filesChecked, documents := 0, 0
	for _, file := range files {
		res, err := lintFile(file, v1alpha1.Profile(*profile))
		if err != nil {
			fprintf(stderr, "recipe lint: %v\n", err)
			ioErr = true
			continue
		}
		filesChecked++
		documents += res.documents
		multiDoc[file] = res.multiDoc
		findings = append(findings, res.findings...)
	}

	if *output == "json" {
		if err := writeJSON(stdout, findings); err != nil {
			fprintf(stderr, "recipe lint: %v\n", err)
			return exitUsage
		}
	} else {
		writeText(stdout, findings, multiDoc, filesChecked, documents)
	}
	switch {
	case ioErr:
		return exitUsage
	case len(findings) > 0:
		return exitFindings
	}
	return exitOK
}

// expandArgs resolves the file and directory arguments into the list of
// files to lint, in argument order with directory contents in lexical walk
// order. Arguments that cannot be resolved are reported and flagged as I/O
// errors, but do not stop the remaining arguments from being linted.
func expandArgs(args []string, stderr io.Writer) (files []string, ioErr bool) {
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			fprintf(stderr, "recipe lint: %v\n", err)
			ioErr = true
			continue
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		err = filepath.WalkDir(arg, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && isYAMLFile(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			fprintf(stderr, "recipe lint: %v\n", err)
			ioErr = true
		}
	}
	return files, ioErr
}

// isYAMLFile reports whether the path names a YAML document by extension.
func isYAMLFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	}
	return false
}

// lintFile validates every document of one file. The returned error
// reports I/O failures only; validation problems are findings.
func lintFile(file string, profile v1alpha1.Profile) (fileResult, error) {
	// #nosec G304 -- reading files named on the command line is the
	// purpose of the tool.
	data, err := os.ReadFile(file)
	if err != nil {
		return fileResult{}, err
	}
	docs := splitYAMLStream(data)
	res := fileResult{multiDoc: len(docs) > 1}
	for i, doc := range docs {
		errs := lintDocument(doc, profile)
		// In a multi-document stream, documents that are merely empty
		// (a trailing "---", a comment-only block between separators)
		// are formatting artifacts, not violations: §5 asks tools to
		// treat each document independently, and there is no document
		// to treat. A file with no document at all is still an error.
		if res.multiDoc && isEmptyDocument(errs) {
			continue
		}
		res.documents++
		res.findings = appendFindings(res.findings, file, i+1, errs)
	}
	if res.documents == 0 {
		// Every document of the stream was empty: report the file once,
		// with the SDK's empty-document error for the first document.
		res.documents = 1
		res.findings = appendFindings(res.findings, file, 1, lintDocument(docs[0], profile))
	}
	return res, nil
}

// appendFindings converts an SDK error list into findings for one document.
func appendFindings(findings []finding, file string, document int, errs v1alpha1.ErrorList) []finding {
	for _, fe := range errs {
		findings = append(findings, finding{
			File:     file,
			Document: document,
			Path:     fe.Path,
			Rule:     fe.Rule,
			Message:  fe.Message,
		})
	}
	return findings
}

// lintDocument validates one document through the SDK and returns its
// violations. Parsing already enforces the draft profile; with
// ProfileCooked, recipes are additionally checked for full pinning (§8).
func lintDocument(doc []byte, profile v1alpha1.Profile) v1alpha1.ErrorList {
	obj, err := v1alpha1.Parse(doc)
	if err != nil {
		return asErrorList(err)
	}
	if r, ok := obj.(*v1alpha1.Recipe); ok && profile == v1alpha1.ProfileCooked {
		if err := r.Validate(profile); err != nil {
			return asErrorList(err)
		}
	}
	return nil
}

// asErrorList recovers the SDK's ErrorList from an error. The SDK always
// returns one; the fallback merely keeps an unexpected error visible.
func asErrorList(err error) v1alpha1.ErrorList {
	var list v1alpha1.ErrorList
	if errors.As(err, &list) {
		return list
	}
	return v1alpha1.ErrorList{{Rule: v1alpha1.RuleInternal, Message: err.Error()}}
}

// isEmptyDocument reports whether the error list says exactly "there is no
// document here".
func isEmptyDocument(errs v1alpha1.ErrorList) bool {
	return len(errs) == 1 && errs[0].Rule == v1alpha1.RuleEmptyDocument
}

// writeText renders findings one per line —
// "<file>[#doc N]: <path>: <message> (<rule>)" — followed by a summary
// line. The document index is shown only for multi-document files, where
// the file name alone would not locate the document.
func writeText(w io.Writer, findings []finding, multiDoc map[string]bool, files, documents int) {
	invalid := make(map[string]bool)
	for _, f := range findings {
		location := f.File
		if multiDoc[f.File] {
			location = fmt.Sprintf("%s#doc %d", f.File, f.Document)
		}
		invalid[fmt.Sprintf("%s#%d", f.File, f.Document)] = true
		if f.Path == "" {
			fprintf(w, "%s: %s (%s)\n", location, f.Message, f.Rule)
			continue
		}
		fprintf(w, "%s: %s: %s (%s)\n", location, f.Path, f.Message, f.Rule)
	}
	summary := fmt.Sprintf("%d file(s), %d document(s) checked", files, documents)
	if len(findings) == 0 {
		fprintf(w, "%s: all valid\n", summary)
		return
	}
	fprintf(w, "%s: %d finding(s) in %d document(s)\n", summary, len(findings), len(invalid))
}

// writeJSON renders the findings as one stable JSON array, "[]" when every
// document is valid.
func writeJSON(w io.Writer, findings []finding) error {
	if findings == nil {
		findings = []finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(findings)
}
