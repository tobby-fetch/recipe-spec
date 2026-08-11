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
- CI: lint (`gofmt`, `golangci-lint`), build, `go vet`, example validation
  against the SDK and schemas, and race-detected tests with coverage.

[Unreleased]: https://github.com/tobby-fetch/recipe-spec/compare/main...HEAD
