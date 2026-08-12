// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

// Command recipe is the command-line companion of the recipe-spec Go SDK.
//
// Its only subcommand today is lint, which parses and validates Recipe and
// Retriever documents (RECIPE-SPEC.md, apiVersion recipe.tobby.dev/v1alpha1)
// through the SDK — the command adds file handling and reporting, never
// validation logic of its own.
//
// Install it with:
//
//	go install github.com/tobby-fetch/recipe-spec/cmd/recipe@latest
package main

import (
	"fmt"
	"io"
	"os"
)

// Exit codes of every subcommand: 0 all documents valid, 1 at least one
// finding, 2 usage or I/O error.
const (
	exitOK       = 0
	exitFindings = 1
	exitUsage    = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches to the subcommands. It is the testable entry point:
// main only binds it to the process arguments, streams, and exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "lint":
		return runLint(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fprintf(stderr, "recipe: unknown command %q\n\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func usage(w io.Writer) {
	fprint(w, `usage: recipe <command> [arguments]

Commands:
  lint    parse and validate Recipe and Retriever documents

Run "recipe lint -h" for the details of a command.
`)
}

// fprintf and fprint write best-effort CLI output: a failure to write the
// report streams (closed pipe, full disk) has nowhere better to be
// reported, and the exit code carries the outcome regardless.
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

func fprint(w io.Writer, s string) { _, _ = fmt.Fprint(w, s) }
