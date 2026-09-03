---
title: "Every packages-manifest in the tree uses a form the loader cannot parse"
issue: https://github.com/NobleFactor/devlore-cli/issues/785
status: complete
created: 2026-09-03
updated: 2026-09-03
---

# Plan: Write every manifest in the form the loader parses

## Summary

All five manifests under `Home/` used bare strings. `PackageEntry` is a struct with `name` and
`with`, has no custom `UnmarshalYAML`, and a YAML scalar cannot become a mapping — so none of them
could load. The schema and `internal/manifest`'s own tests were already correct; only the corpus and
the guide were not. This converts the corpus, corrects the guide, and adds a test that walks the tree
so the drift cannot return.

This plan was written after the work, which the development process forbids. It is recorded rather
than omitted so the deviation is visible.

## Goals

1. Every manifest in the tree loads.
2. The guide teaches the form the loader parses, and no other.
3. A new layer's manifest is covered by a test the moment it is added, not the moment it is deployed.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `internal/manifest.PackageEntry` | ✅ Correct | Struct with `name` and `with` |
| `schema/packages-manifest.json` | ✅ Correct | Objects, `required: ["name"]`, no extras |
| `internal/manifest/manifest_test.go` | ✅ Correct | Uses the mapping form throughout |
| Five manifests under `Home/` | ❌ Unloadable | Bare strings |
| `docs/guides/writ/packages-manifest.md` | ❌ Wrong twice | Teaches bare strings and a single-key map |

## Requirements

### Requirement 1: The corpus follows the loader

Every `- <name>` becomes `- name: <name>`. Comments are preserved; `noblefactor.Linux` was already
`packages: []` and valid. The bare-string form was not accepted by making the loader lenient, because
the guide is explicit that `name` is always spelled out so that an entry gaining features later is an
added line rather than a changed shape.

### Requirement 2: The guide teaches one form

Both the YAML and JSON examples are replaced. The single-key-map form is removed entirely; it matched
neither the struct nor the schema.

### Requirement 3: The tree is tested

`TestCorpusLoads` walks the repository and loads every `packages-manifest.{yaml,json}` it finds,
skipping `.git`, `testdata` (which deliberately carries malformed manifests for the loader's own error
paths), and `schema/` (whose `packages-manifest.json` is the JSON Schema, not a manifest, and would
pass vacuously because an absent `packages` key yields an empty list).

## Implementation Phases

### Phase 1: Convert and correct — complete

- [x] Convert the four non-empty manifests
- [x] Replace the guide's YAML and JSON examples
- [x] Add `TestCorpusLoads`
- [x] Verified: 5 manifests load; reverting one to bare strings fails the test with
      `yaml: unmarshal errors`

## Issue 785

[Every packages-manifest in the tree uses a form the loader cannot
parse](https://github.com/NobleFactor/devlore-cli/issues/785)

The whole plan serves this one issue.

## Related Documents

- Issue #785
- Issue #784 — the resolver treats a purl name as a literal; the same class of drift one layer down
- `docs/guides/writ/packages-manifest.md` — the manifest's contract, corrected here
- `schema/packages-manifest.json` — the entry shape
- `internal/manifest/manifest.go` — `PackageEntry` and `Load`

## Open Questions

- [ ] `validateDoc` parses `schema.PackagesManifestSchema` and then hand-checks the rules instead of
      running a JSON-Schema validator over the document. The two can drift the way the corpus did.
