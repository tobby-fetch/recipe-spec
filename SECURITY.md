# Security Policy

This repository publishes the Recipe/Retriever format specification, its
JSON Schemas, and the reference Go SDK. Recipes are the signed, digest-pinned
description that Tobby and other tools use to verify what they transfer, so
correctness of the schemas and the SDK's validation logic has direct
security consequences for anything built on them.

## Reporting a vulnerability

**Please do not open a public GitHub issue for a security vulnerability.**

Report privately through GitHub Security Advisories:

1. Go to the [Security tab](https://github.com/tobby-fetch/recipe-spec/security)
   of this repository.
2. Click **"Report a vulnerability"**.
3. Describe the issue: affected version (`apiVersion` and, for the SDK, the
   module version), reproduction steps, impact, and any proof-of-concept you
   can share — e.g. a crafted recipe that bypasses validation or a schema gap
   that lets an unsafe manifest pass as valid.

This opens a private advisory visible only to you and the maintainers, with
its own discussion thread — the right channel for exchanging details,
including a fix, before anything is public.

## Response process

- **Acknowledgement**: within 7 days of the report.
- **Coordinated disclosure**: we work with you on a fix and an advisory,
  and agree on a disclosure date before anything is published. We ask that
  you do not disclose the issue publicly until a fix is released.
- **Fix and release**: confirmed vulnerabilities are fixed and released as a
  new tagged version, accompanied by a GitHub Security Advisory that credits
  the reporter (unless anonymity is requested) and, where applicable, a CVE.

## Supported versions

The format and SDK are pre-`v1` (currently `v1alpha1`, draft). Only the
**latest tagged minor version** is supported with security fixes; there is
no long-term support line before the format reaches `v1` (stable). This
section will be updated once `v1` ships.

## Scope

This policy covers the specification document, the JSON Schemas
(`schemas/`), the examples, and the Go SDK (`recipe/`) in this repository.
For the Tobby application itself, report to
[`tobby-fetch/tobby-fetch`](https://github.com/tobby-fetch/tobby-fetch/security)
instead.

## Acknowledgements

We are grateful to everyone who reports vulnerabilities responsibly. Unless
you ask to stay anonymous, we credit reporters in the published advisory.
