// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package main

import (
	"bytes"
	"testing"
)

// splitCase describes one YAML stream and the documents it must split into.
type splitCase struct {
	name string
	in   string
	want []string
}

var splitCases = []splitCase{
	{
		name: "single document",
		in:   "a: 1\nb: 2\n",
		want: []string{"a: 1\nb: 2\n"},
	},
	{
		name: "empty input",
		in:   "",
		want: []string{""},
	},
	{
		name: "two documents",
		in:   "a: 1\n---\nb: 2\n",
		want: []string{"a: 1\n", "---\nb: 2\n"},
	},
	{
		// The comment block and the explicit "---" open the first (and
		// only) document; they must not split into an empty document
		// plus a real one.
		name: "leading comments then marker",
		in:   "# license\n# header\n---\na: 1\n",
		want: []string{"# license\n# header\n---\na: 1\n"},
	},
	{
		name: "directive then marker",
		in:   "%YAML 1.2\n---\na: 1\n",
		want: []string{"%YAML 1.2\n---\na: 1\n"},
	},
	{
		name: "marker with inline content",
		in:   "a: 1\n--- {b: 2}\n",
		want: []string{"a: 1\n", "--- {b: 2}\n"},
	},
	{
		name: "trailing separator",
		in:   "a: 1\n---\n",
		want: []string{"a: 1\n", "---\n"},
	},
	{
		name: "explicit empty documents",
		in:   "---\n---\n",
		want: []string{"---\n", "---\n"},
	},
	{
		name: "no trailing newline",
		in:   "a: 1\n---\nb: 2",
		want: []string{"a: 1\n", "---\nb: 2"},
	},
	{
		name: "crlf separators",
		in:   "a: 1\r\n---\r\nb: 2\r\n",
		want: []string{"a: 1\r\n", "---\r\nb: 2\r\n"},
	},
	{
		// "----" or "--- " glued to content is not a document marker.
		name: "lookalike lines",
		in:   "a: ----\n----\n---x\n",
		want: []string{"a: ----\n----\n---x\n"},
	},
	{
		// A "---" marker inside an indented block scalar cannot exist at
		// column zero, so indented dashes never split.
		name: "indented dashes",
		in:   "a: |\n  ---\n  b\n",
		want: []string{"a: |\n  ---\n  b\n"},
	},
}

func TestSplitYAMLStream(t *testing.T) {
	for _, tc := range splitCases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitYAMLStream([]byte(tc.in))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d documents, want %d:\n%q", len(got), len(tc.want), got)
			}
			for i := range got {
				if string(got[i]) != tc.want[i] {
					t.Errorf("document %d = %q, want %q", i+1, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestSplitYAMLStreamLossless checks that splitting never invents or drops
// bytes: the concatenation of the documents is the input.
func TestSplitYAMLStreamLossless(t *testing.T) {
	for _, tc := range splitCases {
		got := bytes.Join(splitYAMLStream([]byte(tc.in)), nil)
		if string(got) != tc.in {
			t.Errorf("%s: concatenated documents %q != input %q", tc.name, got, tc.in)
		}
	}
}
