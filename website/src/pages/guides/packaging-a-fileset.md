---
layout: ../../layouts/Guide.astro
title: Packaging a FileSet
description: >-
  How to build a reproducible, single-layer OCI image out of arbitrary
  files — deterministic digest included — with standard tooling.
---

A `FileSet` ingredient
([§7.4](https://github.com/tobby-fetch/recipe-spec/blob/main/RECIPE-SPEC.md#74-fileset))
packages arbitrary files — configuration bundles, scripts, certificates,
offline documentation, datasets — **as a standard OCI image**. Being a
plain image buys two consumption modes for free: mounting read-only in
Kubernetes via image volumes, and extraction to disk. This guide is the
normative packaging procedure until a built-in command ships (a
`recipe cook`/`tobby fileset pack` packing step is identified for a later
release). Section references (§) point into
[RECIPE-SPEC.md](https://github.com/tobby-fetch/recipe-spec/blob/main/RECIPE-SPEC.md).

Two properties matter and this guide is built around them:

- **Reproducibility.** Rebuilding the same content MUST yield the same
  digest. A recipe pins the FileSet by digest (§8); if repacking unchanged
  files changes the digest, every rebuild looks like a content change and
  signatures, caches, and audits all churn for nothing.
- **Extraction safety (§14.5).** Consumers enforce strict rules when
  extracting; a FileSet that violates them fails at the destination. Build
  it clean at the source.

## 1. Stage the tree

Lay the files out under a staging directory exactly as they must appear
relative to the **image root** (the same paths `extract.paths` patterns
will match against):

```sh
mkdir -p rootfs/etc/pki/trust-anchors rootfs/opt/scripts
cp anchors/*.pem  rootfs/etc/pki/trust-anchors/
cp provision.sh   rootfs/opt/scripts/
```

Producer-side safety rules, mirroring what extractors enforce (§14.5):

- all paths are **relative** to the image root — no absolute entries, no
  `..` components anywhere;
- symlinks MUST stay inside the tree: relative targets only, never
  escaping the staging root (extractors reject or rewrite escaping links);
- only regular files, directories, and symlinks — device nodes, FIFOs,
  and sockets are ignored on extraction;
- do not rely on setuid/setgid bits: extractors drop them by default;
- keep size, file count, and path depth reasonable — extractors apply
  decompression-bomb limits.

## 2. Build a reproducible tar archive

Archive the tree with GNU tar, pinning every source of nondeterminism:
entry order, timestamps, ownership, and extended attributes. On macOS,
install GNU tar (`brew install gnu-tar`) and use `gtar`; bsdtar takes
different flags and adds metadata of its own.

```sh
# Fix all timestamps to one instant; SOURCE_DATE_EPOCH is the
# reproducible-builds.org convention. Any fixed date works.
export SOURCE_DATE_EPOCH=1735689600   # 2025-01-01T00:00:00Z

tar \
  --format=pax \
  --sort=name \
  --mtime="@${SOURCE_DATE_EPOCH}" \
  --owner=0 --group=0 --numeric-owner \
  --pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime \
  --no-xattrs \
  -C rootfs -cf fileset.tar .

# Deterministic compression: -n omits gzip's embedded name and timestamp.
gzip -n -9 < fileset.tar > fileset.tar.gz
```

Flag by flag: `--sort=name` fixes the entry order regardless of directory
read order; `--mtime` pins modification times; `--owner=0 --group=0
--numeric-owner` stores uid/gid 0 with no user names; the `--pax-option`
scrubs the per-entry atime/ctime pax headers and keeps header names
stable; `--no-xattrs` keeps host-specific extended attributes out.

Audit the archive against the §14.5 rules before shipping it:

```sh
# Absolute paths or '..' components — MUST print nothing:
tar -tf fileset.tar | grep -E '^/|(^|/)\.\.(/|$)' || echo "paths OK"

# Review every link entry and its target — all targets internal:
tar -tvf fileset.tar | awk '$1 ~ /^l/'
```

## 3. Push as a single-layer OCI image

Publish the archive as an image with **one layer** (single-layer images
keep the merged filesystem trivial — no whiteouts, one diff). Two standard
tools do it; both produce a deterministic manifest, because neither embeds
a timestamp.

### With crane

`crane append` on the empty OCI base builds the config (with the layer's
diff ID) and manifest, and pushes in one step:

```sh
crane append \
  --oci-empty-base \
  -f fileset.tar.gz \
  -t registry.example.com/filesets/site-config:2.3.1
```

### With oras

`oras push` uploads the layer as-is but needs a hand-written image config,
since a mountable FileSet is a real image, not a custom artifact: runtimes
require `architecture`, `os`, and the `rootfs.diff_ids` of the
**uncompressed** layer.

```sh
DIFF_ID="sha256:$(sha256sum fileset.tar | awk '{print $1}')"
printf '{"architecture":"amd64","os":"linux","rootfs":{"type":"layers","diff_ids":["%s"]},"config":{}}' \
  "$DIFF_ID" > config.json

oras push registry.example.com/filesets/site-config:2.3.1 \
  --config config.json:application/vnd.oci.image.config.v1+json \
  fileset.tar.gz:application/vnd.oci.image.layer.v1.tar+gzip
```

(The two tools emit slightly different manifests — layer annotations,
config details — so pick one and stick to it; determinism is per
pipeline.)

## 4. Prove the digest is deterministic

Run your pipeline twice — stage, tar, gzip, push (to a scratch tag for the
check) — and compare. Same inputs MUST give the same digest at every
level:

```sh
# Layer level: rebuild and compare the archive bytes.
sha256sum fileset.tar.gz fileset-rebuild.tar.gz
# <same digest>  fileset.tar.gz
# <same digest>  fileset-rebuild.tar.gz

# Manifest level: push the rebuild elsewhere and compare manifest digests.
crane digest registry.example.com/filesets/site-config:2.3.1
crane digest registry.example.com/filesets/site-config:rebuild-check
# both print the same sha256:… — this is the digest the recipe will pin
```

If the digests differ, something nondeterministic crept in — unsorted
entries, live mtimes, xattrs, gzip's embedded timestamp, or different tool
versions. Fix the pipeline before publishing anything.

## 5. Platforms

Platform-independent content SHOULD be published as a **single-manifest
image**, exactly as built above (§7.4) — consumers then never need
platform selection. If the content genuinely differs per platform, build
one image per platform (matching `architecture`/`os` in each config), push
them under staging tags, and combine them into an index; the ingredient's
`digest` then pins the index, and its `platforms` field selects what a
destination requires:

```sh
crane index append \
  -m registry.example.com/filesets/site-config:2.3.1-linux-amd64 \
  -m registry.example.com/filesets/site-config:2.3.1-linux-arm64 \
  -t registry.example.com/filesets/site-config:2.3.1
```

## 6. Record it in a recipe

Pin the published image in the recipe by tag **and** digest:

```sh
DIGEST=$(crane digest registry.example.com/filesets/site-config:2.3.1)
```

```yaml
- name: site-config
  kind: FileSet
  ref: registry.example.com/filesets/site-config
  version: 2.3.1
  digest: sha256:…            # the crane digest output
  extract:
    paths:                    # consumer hint (§7.4): what to materialize;
      - "etc/pki/trust-anchors/**"   # the image always travels in full
      - "opt/scripts/**"
```

`recipe lint --profile cooked` then accepts the ingredient as
publication-ready.

## 7. Consume it

**Mounted, no extraction** — Kubernetes image volumes mount the image
read-only straight from the runtime, referenced by the pinned digest:

```yaml
volumes:
  - name: site-config
    image:
      reference: registry.example.com/filesets/site-config@sha256:…
containers:
  - name: app
    volumeMounts:
      - name: site-config
        mountPath: /etc/site-config
        readOnly: true
```

**Extracted** — implementations follow §7.4: select the platform manifest,
apply layers in order with OCI whiteout handling, materialize the
`extract.paths` selection, and enforce §14.5 while doing so. A one-layer
FileSet makes this equivalent to unpacking the single tar. To inspect what
a consumer would see:

```sh
crane export registry.example.com/filesets/site-config@"$DIGEST" - | tar -tv
```
