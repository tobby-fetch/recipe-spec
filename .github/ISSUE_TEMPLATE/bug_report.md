---
name: Bug report
about: Report incorrect behavior in the spec, schemas, or SDK
title: ""
labels: bug
assignees: ""
---

## Description

A clear description of what went wrong. If this is a specification issue
(ambiguous wording, inconsistency between `RECIPE-SPEC.md` and the schemas),
say so explicitly.

## Steps to reproduce

1. ...
2. ...
3. ...

If the bug is in the Go SDK, a minimal code snippet or failing `go test`
case is the most useful reproduction.

## Expected behavior

What you expected to happen instead.

## Actual behavior

What actually happened — include the exact error message or validation
output if available.

## Version and environment

- `recipe-spec` module version / commit:
- `apiVersion` of the recipe or retriever involved (e.g. `recipe.tobby.dev/v1alpha1`):
- Go version (if SDK-related):

## Additional context

Recipe or retriever excerpts, schema fragments, anything else that helps.

<!--
Think this might be a security vulnerability instead of a bug? Please do not
file it here — see SECURITY.md for the private reporting channel.
-->
