# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
The Recipe/Retriever format itself is versioned Kubernetes-style through its
`apiVersion` (`recipe.tobby.dev/v1alpha1` → `v1beta1` → `v1`); the Go SDK
module follows Go's module versioning conventions.

## [Unreleased]

### Added

- Repository governance: license, `CONTRIBUTING.md`, `SECURITY.md`, DCO
  enforcement in CI, issue and pull request templates.
- `recipe.tobby.dev/v1alpha1` draft: specification document
  (`RECIPE-SPEC.md`), JSON Schemas (draft 2020-12, strict) for the `Recipe`
  and `Retriever` kinds, and example recipes.
- Go SDK (`recipe/v1alpha1`): strict parsing, draft and cooked profile
  validation, and the `§9` version-constraint grammar (`ParseConstraint`,
  `Match`, `Resolve`).
- `§11.2`: the layer's `org.opencontainers.image.title` annotation is
  stated to be illustrative rather than normative. It was only ever shown
  in the example manifest, no implementation checked it, and requiring it
  would contradict the compatibility with generic OCI tooling promised in
  the same paragraph — that tooling writes whatever file name it was
  handed. The publishing guide claimed the opposite and is corrected.
- Go SDK (`cookbook`): the registry-independent half of `§11` — `Build`
  validates a document and assembles the `§11.2` artifact for a given
  publication location (refusing drafts per `§8` and metadata that
  disagrees with the location per `§11.3`), `VerifyManifest` enforces the
  same layout on the consuming side, and `DecideRepublication` states what
  writing an existing tag means under `§8` immutability. No network, no
  registry client, no new dependency: implementations share what a recipe
  artifact *is* and keep their own transport.
- CLI (`cmd/recipe`): `recipe lint` validates files and directories
  through the SDK, with draft and cooked profiles, multi-document YAML
  streams (each document validated independently, `§5`), text or JSON
  reporting, and CI-friendly exit codes (0 valid, 1 findings, 2 usage or
  I/O error). Install with
  `go install github.com/tobby-fetch/recipe-spec/cmd/recipe@latest`.
- Website guides, linked from the README and the landing page:
  *Publishing recipes with standard OCI tooling* (`oras` push per the
  `§11.2` artifact layout, key-based `cosign` signing and offline
  verification, signature-preserving copies across zones) and *Packaging
  a FileSet* (reproducible single-layer OCI image with a deterministic
  digest, producer-side `§14.5` safety rules, mount and extraction
  consumption).
- CI: lint (`gofmt`, `golangci-lint`), build, `go vet`, example validation
  against the SDK and schemas, and race-detected tests with coverage.

[Unreleased]: https://github.com/tobby-fetch/recipe-spec/compare/main...HEAD
