# Spike — Helm integration: Go library vs. external binary

> Decision record for `recipe cook`, a planned recipe-authoring companion to
> the SDK and CLI in this repository. To author a recipe from a Helm chart,
> the tool renders the chart to discover the container images the delivery
> will need. Rendering can be done in-process, by importing Helm as a Go
> library, or out-of-process, by invoking a `helm` binary. The Trivy spike in
> the [tobby-fetch repository](https://github.com/tobby-fetch/tobby-fetch/blob/main/docs/spikes/trivy-library-vs-binary.md)
> measured the same trade-off for a different dependency and concluded
> "external binary"; its arguments are re-measured here rather than assumed
> to transpose.

**Date:** 2026-08-18 · **Toolchain:** go1.25.6 (recipe-spec), go1.26.6 (forced
by the library, see below) · **Helm:** v4.2.4 (library), v4.2.3 and v3.19.0
(binaries) · **Host:** darwin/arm64

## Methodology

Four throwaway modules, all built `CGO_ENABLED=0 -trimpath -ldflags "-s -w"`:

- **baseline** — what `recipe cook` needs with no Helm at all: an OCI client
  (`go-containerregistry`), the recipe-spec SDK, YAML. This is the floor.
- **narrow library** — baseline plus `helm.sh/helm/v4/pkg/chart/v2/loader` and
  `pkg/engine`: load a chart archive, render it, read the images. The smallest
  import surface that renders anything.
- **full library** — `pkg/action` (Install, client dry-run), `pkg/registry`,
  `pkg/cli`: the API surface `helm template` itself uses, and therefore the
  one needed for values precedence, `--set`, subchart resolution, schema
  validation and capabilities. Measured without the baseline imports; add
  ~20 modules and ~0.25 MB for those, as measured on the narrow variant.
- **binary** — the official release binaries, invoked as `helm template`.

Workload: the two OCI-published charts of the example bench,
`victoria-metrics-operator` 0.67.2 (230 KB archive, 25 CRDs) and
`opentelemetry-collector` 0.170.0 (41 KB archive), pulled with `helm pull` and
rendered from the local archive.

## Measurements

### Build and dependency footprint

| Metric | baseline (no Helm) | narrow library | full library | binary path |
|---|---|---|---|---|
| Static binary | **7.3 MB** | 34.7 MB (×4.8) | 44.9 MB (×6.2) | 7.3 MB + helm |
| Module graph (`go list -m all`) | **53** | 241 (×4.5) | 265 (×5.0) | 53 |
| Modules linked in | **13** | 62 | 104 | 13 |
| `go.sum` lines | **40** | 159 | 400 | 40 |
| Cold build | **3.8 s wall / 16.6 s CPU** | 10.2 s / 69.7 s | — | 3.8 s / 16.6 s |
| `go` directive required | **1.25.6** | **1.26.0** | **1.26.0** | 1.25.6 |
| Artifact to ship | — | — | — | 58.6 MB (helm 4.2.3) / 56.8 MB (helm 3.19.0) |

Two entries deserve emphasis.

**The toolchain moves.** `helm.sh/helm/v4@v4.2.4` declares `go >= 1.26.0`;
recipe-spec is pinned at go1.25.6 in `mise.toml` and `go.mod`. Importing the
library switches the toolchain for that module and puts the repository's Go
version on Helm's release schedule — the same coupling the Trivy spike found,
found again.

**The full path is the one you actually need.** The narrow path is small
because it is incomplete: it renders a chart, and nothing else. Reaching
`helm template` parity means reimplementing values-file precedence and deep
merge, `--set` parsing, subchart resolution and download, JSON-schema
validation of values, and `Capabilities`/`--kube-version` handling — or
importing `pkg/action`, which is the 44.9 MB / 265-module column, and which
pulls `go-sql-driver/mysql` and `filippo.io/edwards25519` into a tool that
never touches a database (Helm's release-storage drivers).

### Known vulnerabilities in the dependency set

| Scan | baseline | narrow library |
|---|---|---|
| `trivy fs --scanners vuln` on `go.mod` | 0 | **0** |
| `govulncheck`, called dependency vulnerabilities | 0 | **0** |
| `govulncheck`, imported but not called | 8 | 14 |

**This is where the Trivy spike does not transpose.** Trivy-as-a-library
imported two HIGH CVEs on day one and would have turned the release pipeline
red for code Tobby never called. Helm imports none today. The supply-chain
argument that decided the Trivy question is simply absent here, and saying
otherwise would be reasoning by analogy against the measurement.

### Runtime

| Metric | narrow library (in-process) | binary (exec) |
|---|---|---|
| `helm template` on victoria-metrics-operator, 5 runs | 0.07 – 0.16 s | 0.11 – 0.17 s |

The exec overhead is 30–50 ms, below the spread of either path. Unlike the
Trivy case there is not even a latency story to trade away: a `recipe cook`
run renders a handful of charts and then spends seconds talking to registries.

### Fidelity

Identical image inventories from all three renderers on both charts:

| Chart | helm 3.19.0 | helm 4.2.3 | narrow library (v4.2.4) |
|---|---|---|---|
| victoria-metrics-operator 0.67.2 | `victoriametrics/operator:v0.74.0` | idem | idem |
| opentelemetry-collector 0.170.0, with values | `…/opentelemetry-collector-contrib:0.158.0` | idem | idem |
| opentelemetry-collector 0.170.0, defaults | render **fails** | render **fails** | render **fails** |

Two findings fall out of this table and are inputs to the tool's design rather
than to this decision:

- The "chart renders nothing" escape class has **two shapes**. At chart
  0.170.0 the OpenTelemetry chart does not render an empty inventory, it
  fails loudly: `[ERROR] 'image.repository' must be set`, from a `fail` in
  `NOTES.txt`. A tool that only looks for an empty image list would treat a
  failed render as an unrelated error. Both shapes must be caught.
- **Rendering is not deterministic.** Two runs of the same helm binary on the
  same victoria-metrics-operator archive differ by 56 lines: the chart mints a
  self-signed CA at render time. The image inventory is stable, the manifests
  are not — so the tool may cache and compare inventories, never rendered
  bytes.

Helm 3 and Helm 4 agree on this bench. That is a fact about two charts on one
day, not a compatibility guarantee: Helm's Go API carries none either, as this
spike's own port from `ClientOnly`/`DryRun` to `DryRunStrategy`, and from
`pkg/chart` to `pkg/chart/v2`, demonstrates. Each Helm major would be a code
migration inside the tool.

## Analysis

- **Whose Helm is the ground truth.** This is the argument that has no
  counterpart in the Trivy spike, and it points the other way from every
  footprint number. A recipe describes what an operator's cluster will pull.
  That inventory is produced by *their* Helm, from *their* values, at whatever
  version they deploy with. A tool that embeds one pinned Helm renders with a
  Helm nobody deploys with, and any divergence it introduces is invisible
  precisely where this project cannot afford invisibility. Invoking the
  operator's own binary makes the renderer an input the author chooses, sees,
  and can record.
- **Helm is already there.** Trivy had to be shipped, because target
  environments do not carry a scanner; that is what made "one artifact instead
  of two" a real comparison. Anyone authoring a recipe from a Helm chart has
  Helm installed — it is how they got the chart in the first place. The
  external dependency costs nothing on the machine where the feature is used,
  and nothing at all on the machines where it is not: cooking a draft, the
  primary use, never renders anything.
- **Isolation.** `helm template` executes chart-supplied Go templates with
  Sprig, whose function set includes DNS resolution and cryptographic
  generation. Rendering a chart is executing someone else's code on the
  author's workstation. Out-of-process is not a decisive argument, but it is
  the same one that applied to the scanner, and it still applies.
- **Toolchain and graph.** A ×4.5 module graph and a forced Go 1.26 on a
  repository whose adoptability argument *is* its small graph. The cook module
  is separate from the SDK module, so the SDK's 9-module graph is safe either
  way — but the repository would carry two Go versions and two lint
  configurations for one feature.
- **What the library buys.** A self-contained binary and 30 ms. Both real,
  neither decisive.

## Decision: **external `helm` binary**, resolved from the author's environment

Same verdict as the Trivy spike, reached on different evidence — and with one
deliberate inversion. Trivy is a **digest-pinned binary shipped by us, never
resolved from PATH**, because the scanner must be an attested artifact we
control. Helm is the opposite: resolved from `PATH` or `--helm`, because
fidelity with the operator's own deployment is the property that matters, and
pinning would destroy it.

Consequences for the implementation:

- The renderer sits behind a `Renderer` seam (chart archive + values files →
  rendered manifests + the Helm version that produced them). If Helm ever
  publishes a slim, stable rendering API, swapping the implementation costs
  nothing architecturally.
- The resolved Helm version is **recorded** — in the blind-spot report and in
  the emitted recipe's annotations. A render is only reproducible against the
  binary that produced it, so the binary is named in the record.
- **The tool pulls the chart itself**, with the OCI client it already has, and
  verifies its digest before handing the local archive to `helm template`.
  Helm therefore never contacts a registry: no credential handling duplicated
  across two tools, no endpoint policy divergence, and the bytes rendered are
  provably the bytes pinned in the recipe.
- Helm is required **only** by the from-chart path. `recipe cook` on a draft or
  a meta-recipe with no chart source runs with no external dependency at all.
  Its absence is reported with the tested version range, never guessed around.
- CI renders the bench with **both** helm 3 and helm 4, pinned in `mise.toml`,
  and fails if their inventories diverge — the assumption in the fidelity
  table above becomes a test instead of a memory.

## Reproducing

The four modules, the pulled charts and the raw measurements are throwaway by
design; the commands are:

```sh
go mod init spike && go get helm.sh/helm/v4@v4.2.4
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o spike .
go list -m all | wc -l && go version -m spike | grep -c '^	dep'
trivy fs --scanners vuln . && govulncheck ./...
helm template t <chart>.tgz -f values.yaml | grep -E '^\s*image:'
```
