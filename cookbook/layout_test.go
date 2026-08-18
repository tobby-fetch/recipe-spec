// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package cookbook_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tobby-fetch/recipe-spec/cookbook"
)

// wellFormed returns the manifest of a conforming recipe artifact, as a
// mutable map: each test breaks exactly one thing in it.
func wellFormed(t *testing.T) map[string]any {
	t.Helper()
	art, err := cookbook.Build([]byte(cookedDoc), "demo", "1.0.0")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(art.Manifest.Content, &m); err != nil {
		t.Fatalf("unmarshaling our own manifest: %v", err)
	}
	return m
}

func encode(t *testing.T, m map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshaling the broken manifest: %v", err)
	}
	return raw
}

func TestVerifyManifestRejectsEveryLayoutViolation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		corrupt func(m map[string]any)
		field   string
	}{
		{
			// An image manifest with no artifactType is an image, not a
			// recipe. Accepting it would let anything into a cookbook.
			name:    "no artifactType",
			corrupt: func(m map[string]any) { delete(m, "artifactType") },
			field:   "artifactType",
		},
		{
			name:    "some other artifact type",
			corrupt: func(m map[string]any) { m["artifactType"] = "application/vnd.cncf.helm.config.v1+json" },
			field:   "artifactType",
		},
		{
			name: "a real config instead of the empty one",
			corrupt: func(m map[string]any) {
				m["config"].(map[string]any)["mediaType"] = "application/vnd.oci.image.config.v1+json"
			},
			field: "config.mediaType",
		},
		{
			// §11.2: publishers MUST NOT attach additional layers. A
			// second layer is content nobody verified.
			name: "a second layer",
			corrupt: func(m map[string]any) {
				layers := m["layers"].([]any)
				m["layers"] = append(layers, layers[0])
			},
			field: "layers",
		},
		{
			name:    "no layer at all",
			corrupt: func(m map[string]any) { m["layers"] = []any{} },
			field:   "layers",
		},
		{
			name: "the layer is not a recipe document",
			corrupt: func(m map[string]any) {
				m["layers"].([]any)[0].(map[string]any)["mediaType"] = "application/vnd.oci.image.layer.v1.tar+gzip"
			},
			field: "layers[0].mediaType",
		},
		{
			// Refused before the download starts, not after 4 MiB have
			// already crossed the wire.
			name: "a document larger than the bound",
			corrupt: func(m map[string]any) {
				m["layers"].([]any)[0].(map[string]any)["size"] = float64(cookbook.MaxDocumentBytes + 1)
			},
			field: "layers[0].size",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := wellFormed(t)
			tc.corrupt(m)
			_, err := cookbook.VerifyManifest(encode(t, m))
			var le *cookbook.LayoutError
			if !errors.As(err, &le) {
				t.Fatalf("error = %v, want a LayoutError — this artifact must be rejected", err)
			}
			if le.Field != tc.field {
				t.Errorf("LayoutError.Field = %q, want %q", le.Field, tc.field)
			}
			if !strings.Contains(le.Error(), tc.field) {
				t.Errorf("message %q does not name the offending field", le.Error())
			}
		})
	}
}

func TestVerifyManifestRejectsNonJSON(t *testing.T) {
	_, err := cookbook.VerifyManifest([]byte("this is not a manifest"))
	var le *cookbook.LayoutError
	if !errors.As(err, &le) {
		t.Fatalf("error = %v, want a LayoutError", err)
	}
	if le.Field != "manifest" {
		t.Errorf("LayoutError.Field = %q, want %q", le.Field, "manifest")
	}
}

// The title is written but never required: §11.2 states no normative rule
// for it, and generic OCI tooling puts the pushed file's name there.
// Rejecting an artifact over it would break the interoperability §11.2
// promises.
func TestVerifyManifestDoesNotRequireTheLayerTitle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(layer map[string]any)
		want   string
	}{
		{"no annotations at all", func(l map[string]any) { delete(l, "annotations") }, ""},
		{"pushed under another file name", func(l map[string]any) {
			l["annotations"].(map[string]any)["org.opencontainers.image.title"] = "wordpress-recipe.yaml"
		}, "wordpress-recipe.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := wellFormed(t)
			tc.mutate(m["layers"].([]any)[0].(map[string]any))
			layout, err := cookbook.VerifyManifest(encode(t, m))
			if err != nil {
				t.Fatalf("VerifyManifest: %v", err)
			}
			if layout.Title != tc.want {
				t.Errorf("Title = %q, want %q", layout.Title, tc.want)
			}
		})
	}
}

// A document exactly at the bound is legal; one byte over is not.
func TestVerifyManifestSizeBoundIsInclusive(t *testing.T) {
	for _, tc := range []struct {
		size int64
		ok   bool
	}{
		{cookbook.MaxDocumentBytes, true},
		{cookbook.MaxDocumentBytes + 1, false},
	} {
		t.Run(fmt.Sprint(tc.size), func(t *testing.T) {
			m := wellFormed(t)
			m["layers"].([]any)[0].(map[string]any)["size"] = float64(tc.size)
			_, err := cookbook.VerifyManifest(encode(t, m))
			if (err == nil) != tc.ok {
				t.Errorf("size %d: err = %v, want ok = %v", tc.size, err, tc.ok)
			}
		})
	}
}
