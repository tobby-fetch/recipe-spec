// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package main

import "bytes"

// splitYAMLStream splits a YAML stream into its documents, so that each one
// can be handed to the SDK independently — the SDK deliberately parses one
// document per call, and RECIPE-SPEC.md §5 lets tools accept multi-document
// streams as long as every document is treated independently.
//
// A document begins at a directives-end marker: a line whose content is
// exactly "---", or "---" followed by whitespace (possibly with the root
// node on the same line). YAML reserves such lines — they cannot occur
// inside a scalar in block context — so splitting on them is faithful to
// the grammar without parsing the stream twice. A marker preceded only by
// blank lines, comments, and %directives is the explicit start of the
// current document, not a separator, so a file that opens with a comment
// block followed by "---" still counts as a single document.
//
// Each returned document keeps its original bytes (its marker line
// included): the SDK sees exactly what the author wrote.
func splitYAMLStream(data []byte) [][]byte {
	var docs [][]byte
	start := 0       // byte offset where the current document begins
	preamble := true // current document holds only blanks/comments/directives
	for offset := 0; offset < len(data); {
		end := bytes.IndexByte(data[offset:], '\n')
		var next int
		var line []byte
		if end < 0 {
			line = data[offset:]
			next = len(data)
		} else {
			line = data[offset : offset+end]
			next = offset + end + 1
		}
		switch {
		case isDirectivesEndMarker(line):
			if !preamble {
				docs = append(docs, data[start:offset])
				start = offset
			}
			// The marker starts real content either way: a following
			// "---" is always a separator.
			preamble = false
		case preamble && !isPreambleLine(line):
			preamble = false
		}
		offset = next
	}
	if start < len(data) || len(docs) == 0 {
		docs = append(docs, data[start:])
	}
	return docs
}

// isDirectivesEndMarker reports whether the line is a "---" document
// marker: exactly three dashes, alone or followed by whitespace (and then
// possibly the root node of the document).
func isDirectivesEndMarker(line []byte) bool {
	line = trimCR(line)
	if !bytes.HasPrefix(line, []byte("---")) {
		return false
	}
	rest := line[3:]
	return len(rest) == 0 || rest[0] == ' ' || rest[0] == '\t'
}

// isPreambleLine reports whether the line carries no document content:
// blank, a #comment, or a %directive. Only such lines may precede a "---"
// marker that opens (rather than separates) a document.
func isPreambleLine(line []byte) bool {
	line = bytes.TrimSpace(trimCR(line))
	return len(line) == 0 || line[0] == '#' || line[0] == '%'
}

// trimCR drops a trailing carriage return, so CRLF input splits like LF.
func trimCR(line []byte) []byte {
	return bytes.TrimSuffix(line, []byte("\r"))
}
