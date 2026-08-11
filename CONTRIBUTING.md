# Contributing to recipe-spec

Thanks for your interest in the Recipe format. This document covers the
workflow, conventions, and legal requirements for contributing to the
specification, its JSON Schemas, examples, and the Go SDK.

The format is meant to become a lingua franca implementable by any tool, so
review and outside implementations are explicitly welcome — file an issue if
something in [RECIPE-SPEC.md](RECIPE-SPEC.md) is ambiguous or hard to
implement, even without a matching code change.

## Workflow

The project is **trunk-based**:

- `main` is always releasable and is protected: no direct pushes, every
  change lands through a pull request.
- Feature branches are short-lived (days, not weeks) and branch off `main`.
- A pull request needs **at least one approving review** and a **green CI
  run** before it can merge. Prefer several small PRs over one large one.

```sh
git checkout -b feat/short-description main
# ... work, committing with sign-off (see DCO below) ...
git push -u origin feat/short-description
# open a pull request against main
```

Changes to the format itself (`RECIPE-SPEC.md`, `schemas/`) are more
consequential than changes to the SDK: the format is versioned Kubernetes-style
(`v1alpha1` → `v1beta1` → `v1`), and breaking changes are only expected before
`v1beta1`. Flag format changes clearly in the PR description.

## Commit messages: Conventional Commits

Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org),
which drives the automated changelog. Common types used in this repository:

| Type | Use for |
|---|---|
| `feat` | A new capability in the format or the SDK |
| `fix` | A bug fix (SDK parsing/validation logic, schema defect) |
| `docs` | Documentation only (`RECIPE-SPEC.md`, README, examples) |
| `test` | Adding or fixing tests, no production code change |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `build` | Build system, dependencies |
| `ci` | CI/CD workflow changes |
| `chore` | Everything else (repo maintenance, tooling) |

Example: `feat: add FileSet mount-path validation`.

A breaking change is flagged with a `!` after the type (`feat!: ...`) or a
`BREAKING CHANGE:` footer. Before `v1beta1`, breaking changes to the format
are expected as the design settles; call them out explicitly regardless.

## Developer Certificate of Origin (DCO)

Every commit must carry a **sign-off**: a `Signed-off-by` trailer certifying
that you wrote the change, or otherwise have the right to submit it under the
project's license, per the
[Developer Certificate of Origin](https://developercertificate.org).

Sign off with `-s` (git adds the trailer using your configured name and
email):

```sh
git commit -s -m "feat: add FileSet mount-path validation"
```

If you forgot on the last commit: `git commit --amend -s --no-edit`. For a
whole branch: `git rebase --signoff main`.

This project does not use a CLA — the DCO is lighter-weight and does not
require signing over any rights; you keep copyright on your own
contributions (see [ADR-0003](https://github.com/tobby-fetch/tobby-fetch/blob/main/docs/adr/ADR-0003-repo-and-licensing-split.md)
in the `tobby-fetch` repository). CI checks every commit in a pull request
for a valid sign-off (`.github/workflows/dco.yml`); a PR with an unsigned
commit is blocked until it is fixed.

## Setting up a development environment

Tooling is pinned and installed with [mise](https://mise.jdx.dev):

```sh
mise install        # installs the pinned Go, golangci-lint, gitleaks, ...
mise run setup       # one-time: activates the repo's git hooks (secret
                      # scanning on every commit — see .githooks/pre-commit)
```

The Go SDK is a regular Go module and tests with `go test`:

```sh
go test ./...                     # SDK tests
mise run test                     # same, as CI runs it: -race -count=2
mise run lint                     # golangci-lint run, strict profile
```

Run `mise run lint` and `mise run test` before opening a pull request — CI
also runs `gofmt -l .` and validates every example in `examples/` against the
SDK and JSON Schemas.

## Code style

- Format with `gofmt` (`gofmt -l .` must report nothing).
- Lint with `golangci-lint` against the repository's strict profile
  (`.golangci.yml`): no default exclusions, and every `//nolint` must name
  its linter and carry a written justification.
- Keep exported identifiers documented; the SDK is consumed by third-party
  tools, so its public API surface (`recipe/v1alpha1`) is held to a higher
  documentation and stability bar than typical internal code.

## SPDX headers

Every new source file carries an SPDX license header and copyright line at
the top:

```go
// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors
```

Use the year of the file's creation. Don't edit the header of a file you are
merely modifying.

## License of contributions

This repository is licensed under the [Apache License 2.0](LICENSE). By
submitting a pull request, you agree that your contribution is licensed
under the same terms.

## Reporting bugs and requesting features

Use the issue templates (`.github/ISSUE_TEMPLATE/`). For anything that looks
like a security vulnerability, do **not** open a public issue — see
[SECURITY.md](SECURITY.md).
