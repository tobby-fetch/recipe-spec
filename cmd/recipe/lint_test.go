// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The lint tests exercise the CLI through its testable entry point, on the
// repository examples, on the SDK's crafted invalid corpus, and on CLI-only
// fixtures (multi-document streams).
const (
	examplesDir      = "../../examples"
	invalidCorpusDir = "../../recipe/v1alpha1/testdata/invalid"
	multiMixedFile   = "testdata/multi-mixed.yaml"
)

// validRecipe is a minimal valid draft recipe for generated fixtures.
const validRecipe = `apiVersion: recipe.tobby.dev/v1alpha1
kind: Recipe
metadata:
  name: hello
  version: 1.0.0
spec:
  ingredients:
    - name: hello
      kind: ContainerImage
      ref: docker.io/library/hello-world
      version: linux
`

// runCLI runs the command in-process and captures its streams.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, err bytes.Buffer
	code = run(args, &out, &err)
	return code, out.String(), err.String()
}

func TestLintExamplesAreValidDrafts(t *testing.T) {
	code, stdout, stderr := runCLI(t, "lint", examplesDir)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, stdout, stderr)
	}
	if want := "4 file(s), 4 document(s) checked: all valid\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestLintCookedProfile(t *testing.T) {
	// wordpress.yaml is cooked: fully pinned.
	code, stdout, stderr := runCLI(t, "lint", "--profile", "cooked", filepath.Join(examplesDir, "wordpress.yaml"))
	if code != exitOK {
		t.Fatalf("wordpress: exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, stdout, stderr)
	}

	// ai-model.yaml is a draft: its second ingredient has no digest and a
	// constraint version, which the cooked profile must both report.
	code, stdout, _ = runCLI(t, "lint", "--profile", "cooked", filepath.Join(examplesDir, "ai-model.yaml"))
	if code != exitFindings {
		t.Fatalf("ai-model: exit code = %d, want %d\nstdout:\n%s", code, exitFindings, stdout)
	}
	for _, want := range []string{
		"spec.ingredients[1].digest",
		"(cooked-digest-missing)",
		"spec.ingredients[1].version",
		"(cooked-version-not-exact)",
		"1 file(s), 1 document(s) checked: 2 finding(s) in 1 document(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestLintInvalidCorpus(t *testing.T) {
	code, stdout, stderr := runCLI(t, "lint", invalidCorpusDir)
	if code != exitFindings {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitFindings, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty (invalid documents are findings, not I/O errors)", stderr)
	}
	// Spot-check the "<file>: <path>: <message> (<rule>)" line format.
	if want := "digest-malformed.yaml: spec.ingredients[0].digest: "; !strings.Contains(stdout, want) {
		t.Errorf("stdout does not contain %q:\n%s", want, stdout)
	}
}

// TestLintAcceptsMultiDocumentStreams pins the split of responsibilities
// with the SDK: ParseRecipe rejects multi-document input, while the CLI
// splits the stream and validates each document independently, as §5
// permits. The SDK's multiple-documents fixture is two valid recipes, so
// lint accepts it.
func TestLintAcceptsMultiDocumentStreams(t *testing.T) {
	code, stdout, stderr := runCLI(t, "lint", filepath.Join(invalidCorpusDir, "multiple-documents.yaml"))
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, stdout, stderr)
	}
	if want := "1 file(s), 2 document(s) checked: all valid\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestLintMultiDocumentFindings(t *testing.T) {
	code, stdout, _ := runCLI(t, "lint", multiMixedFile)
	if code != exitFindings {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s", code, exitFindings, stdout)
	}
	// Documents 1 and 3 are valid, the trailing empty document is
	// skipped, and every finding names document 2.
	for _, want := range []string{
		multiMixedFile + "#doc 2: ",
		"digset",
		"1 file(s), 3 document(s) checked: 1 finding(s) in 1 document(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "#doc 1") || strings.Contains(stdout, "#doc 3") || strings.Contains(stdout, "#doc 4") {
		t.Errorf("findings reported for a valid or empty document:\n%s", stdout)
	}
}

func TestLintJSONOutput(t *testing.T) {
	code, stdout, _ := runCLI(t, "lint", "--output", "json", multiMixedFile)
	if code != exitFindings {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s", code, exitFindings, stdout)
	}
	var got []finding
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	f := got[0]
	if f.File != multiMixedFile || f.Document != 2 || f.Rule != "schema" ||
		!strings.Contains(f.Path, "spec.ingredients[0]") || !strings.Contains(f.Message, "digset") {
		t.Errorf("unexpected finding: %+v", f)
	}

	// A valid input yields an empty array, not null.
	code, stdout, _ = runCLI(t, "lint", "--output", "json", filepath.Join(examplesDir, "wordpress.yaml"))
	if code != exitOK {
		t.Fatalf("wordpress: exit code = %d, want %d", code, exitOK)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("stdout = %q, want an empty JSON array", stdout)
	}
}

func TestLintEmptyInputs(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"empty.yaml":         "",
		"comments-only.yaml": "# nothing here\n",
		"separators.yaml":    "---\n---\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			code, stdout, _ := runCLI(t, "lint", path)
			if code != exitFindings {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s", code, exitFindings, stdout)
			}
			if !strings.Contains(stdout, "(empty-document)") {
				t.Errorf("stdout does not report an empty document:\n%s", stdout)
			}
		})
	}
}

func TestLintDirectoryRecursion(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dir, "top.yaml"):    validRecipe,
		filepath.Join(sub, "deep.yml"):    validRecipe,
		filepath.Join(sub, "ignored.txt"): "not yaml\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	code, stdout, stderr := runCLI(t, "lint", dir)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitOK, stdout, stderr)
	}
	if want := "2 file(s), 2 document(s) checked: all valid\n"; stdout != want {
		t.Errorf("stdout = %q, want %q (both *.yaml and *.yml, nothing else)", stdout, want)
	}
}

func TestLintUsageErrors(t *testing.T) {
	valid := filepath.Join(examplesDir, "wordpress.yaml")
	cases := map[string][]string{
		"no command":      {},
		"unknown command": {"cook"},
		"no arguments":    {"lint"},
		"unknown flag":    {"lint", "--frobnicate", valid},
		"unknown profile": {"lint", "--profile", "raw", valid},
		"unknown output":  {"lint", "--output", "xml", valid},
		"missing path":    {"lint", "does-not-exist.yaml"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, _, stderr := runCLI(t, args...)
			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d\nstderr:\n%s", code, exitUsage, stderr)
			}
			if stderr == "" {
				t.Error("stderr is empty; usage and I/O errors must be reported")
			}
		})
	}
}

// TestLintIOErrorTrumpsFindings pins the exit-code precedence: a run with
// both findings and an I/O failure exits 2, so scripts never mistake a
// partial run for a complete one.
func TestLintIOErrorTrumpsFindings(t *testing.T) {
	code, stdout, stderr := runCLI(t, "lint", "does-not-exist.yaml", filepath.Join(invalidCorpusDir, "empty.yaml"))
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, exitUsage, stdout, stderr)
	}
	if !strings.Contains(stdout, "(empty-document)") {
		t.Errorf("the readable file was not linted:\n%s", stdout)
	}
}

func TestHelp(t *testing.T) {
	for name, args := range map[string][]string{
		"help command": {"help"},
		"lint -h":      {"lint", "-h"},
	} {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, args...)
			if code != exitOK {
				t.Fatalf("exit code = %d, want %d", code, exitOK)
			}
			if !strings.Contains(stdout+stderr, "usage:") {
				t.Errorf("no usage text printed\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
		})
	}
}
