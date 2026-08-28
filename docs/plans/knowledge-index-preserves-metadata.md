---
title: "The knowledge indexer preserves what it did not write"
issue: https://github.com/NobleFactor/devlore-cli/issues/XX
status: draft
created: 2026-08-27
updated: 2026-08-27
---

# Plan: The knowledge indexer preserves what it did not write

## Summary

`star devlore knowledge index` rebuilds each domain's `index.yaml` from a directory listing and
writes `{"name": filename}` per entry. Everything else in the file is discarded. Until
2026-08-27 this was invisible, because `update-indexes` had been failing for months. Unfreezing
it (registry #78, #79) let the workflow run — and registry #80 deleted curated metadata from
three domains on its first successful pass.

This makes the indexer additive: it may add entries for new files and drop entries for deleted
ones, and it may touch nothing else.

## Authority

`devlore-cli/docs/devlore-ai-design-brief.md` (#356, *"Status: next order of business"*) is what
these indexes are for, and it is the reason this is worth fixing properly rather than patching.

The brief targets CAG packs served from devlore-registry through an **MCP facade** — packs as
resources (`devlore://packs/<name>@<ver>`) and a `get_pack(name, max_tokens)` tool. The knowledge
domains are the substrate those packs are assembled from: `devlore-conventions` and
`lore-package-authoring` are named as the first pack set.

Three of its requirements bear directly on this defect:

- **§4, determinism gate at publish** — *"serialize twice, compare hashes, reject on mismatch. A
  pack that cannot be reproducibly serialized cannot be honestly content-addressed."* An indexer
  that silently changes the shape of its output cannot support that gate. The strict-section rule
  below is what makes the output stable enough to hash.
- **§4, artifact classes** — an index is a *derived* artifact: regenerable, invalidated by input
  hashes. It is currently treated as neither source nor derived, which is why nothing noticed it
  being rewritten destructively.
- **§3, pack anatomy** — per-asset metadata (`purpose`, `description`, `source_system`) is what a
  `get_pack` facade would describe assets with. Flattening entries to `{"name": filename}` destroys
  exactly the material the delivery socket needs.

This plan does not implement any of that. It stops the destruction so the material still exists
when that work starts. Making it *right* — derived-class validation, determinism gate, provenance —
is later work, and the brief is where it is specified.

## The damage, measured

| Domain | Lines | Lost |
| --- | --- | --- |
| `knowledge/packages/index.yaml` | 28 → 5 | `concepts:` section; every slot's `package`, `description`, `platforms`, `install_types`; an entire `ollama` slot |
| `knowledge/migration/index.yaml` | 31 → 23 | `concepts:` |
| `knowledge/shared/index.yaml` | 18 → 9 | `providers:` |

Recoverable only from git history, at registry `b435dd0^`.

## What is unmined today

The destruction in #80 was the loud half. Surveying the tree turned up as much again that the
indexer simply cannot see, and reports nothing about:

| Gap | Detail |
| --- | --- |
| A ninth asset type | `package-authoring/bindings/` — `reference.md`, `reference.yaml`, `rules.yaml`. Zero mentions in its index |
| Loose domain-root files | `migration/README.md` (2 KB), `migration/systems-reference.yaml` (1.8 KB) — real content, under no asset type |
| A misfiled entry | `signatures/dotfile-systems.yaml` is indexed; the file is at `migration/dotfile-systems.yaml` |
| An empty domain | `manifest-authoring` has no subdirectories and an index with no sections |
| Per-entry metadata | `purpose`, `description`, `source_system`, slot fields — flattened to `{"name": f}` |
| Unrecognized types | `concepts` (migration, packages), `providers` (shared) |

The misfiled entry matters more than its size. An indexer that merely drops entries whose file is
absent would have deleted that entry silently — the file is not gone, it moved. Dropping is the
same class of error as flattening: a change nobody was asked about.

## The principle this plan enforces

Every gap above has one shape: **the indexer's silence is indistinguishable from success.** It
cannot see `bindings/`, cannot see loose files, cannot tell a moved file from a deleted one — and
says nothing either way, while `validate-yaml-schemas` goes green because the output parses.

So: *anything the indexer cannot classify is an error, not an omission.* That is what converts
silent incompleteness into a red build, and it is the whole point of the strict contract below.

## Two failure modes, not one

**Per-entry metadata.** `build_asset_entries` returns `{"name": filename}`, so
`purpose`, `description`, `source_system`, and any per-entry field a human wrote is dropped.

**Unknown top-level sections.** `build_index` emits `domain` plus the six keys in `ASSET_TYPES`.
`concepts:` and `providers:` are in neither list, so they vanish. This one matters more than it
looks: it means any section anybody invents is deleted on the next CI run, silently.

## Why not simply use cmd/devlore-index

It was the obvious answer and it is not sufficient. The Go tool fixes the first failure mode
with `mergeEntries`, but its `KnowledgeIndex` struct has fields for exactly the same six asset
types — no `Concepts`, no `Providers`. It would have destroyed both sections too.

It is also uninvoked: created 2026-08-10, six months after the Starlark extension, never wired
into any workflow, and its header still emits `go run ./cmd/gen-index`, a command name that has
not existed since it was renamed. It goes.

## Requirements

### Merge, do not rebuild

For each file present on disk, reuse the existing entry **whole** if one matches by `name`;
otherwise emit `{"name": filename}`. Drop entries whose file is gone. This is `mergeEntries` from
the Go tool, which is the correct algorithm.

### Sections are a strict contract

The set of top-level sections must match what the directory tree implies, exactly. Any
disagreement stops the run:

- a section in the file with no corresponding directory -> **refuse** (it is about to be dropped)
- a directory with assets but no section in the file -> **refuse** (it is about to be added silently)
- a section name that is not a known asset type -> **refuse**

The indexer may then only update entries *within* sections. Every structural change is a human
decision. This is stricter than carrying unknown sections through, and it is the rule that would
have caught registry #80 on its first run rather than after the fact.

It also lines up with docs/devlore-ai-design-brief.md §4, which requires a determinism gate at
publish -- "serialize twice, compare hashes, reject on mismatch". An indexer that silently changes
the shape of its output cannot support that.

### concepts, providers, and bindings become real asset types

Consequence of the rule above, and a prerequisite rather than a follow-up: `concepts:` appears in `migration` and `packages`, `providers:` in `shared`, and
`bindings/` exists on disk in `package-authoring` while appearing in no index at all. An indexer that refuses unknown sections
refuses on three of six domains until ASSET_TYPES grows from six to nine.

### A missing file refuses; it does not drop

If an indexed entry has no file, the indexer stops. The file may have been deleted, or it may have
moved — `signatures/dotfile-systems.yaml` is the latter — and only a human knows which. Silently
dropping the entry loses the metadata attached to it and reports success.

### Unclassifiable content refuses

A file under a domain but outside any asset-type directory is an error. Today
`migration/README.md` and `migration/systems-reference.yaml` are invisible; ignoring them is the
status quo, and the status quo is a false positive.

### Nothing is required to write a header

`yaml.encode` does not retain comments, so the hand-written header comments in these files cannot
survive a regeneration. A standard generated-by header replaces them — the Go tool does the same.
This is a real, accepted loss, and it should be stated in the file rather than discovered.

### The provenance string is wrong today

`Package/commands/index.star:91` hardcodes `"generated": "star devlore knowledge index packages"`
in the **package** indexer's output. `packages/index.yaml` has been misattributing itself. One
line, fixed here because it is the same family of defect.

## Implementation Phases

### Phase 0: Stop the bleeding — devlore-registry

- [ ] Disable the `Update knowledge indexes` step in `update-indexes.yaml`, with a comment naming
      this plan
- [ ] Leave `Update package index` running; it is not lossy

The next push to `develop` re-strips the three files, so this lands before anything else.

**Files**: `.github/workflows/update-indexes.yaml` — Modify

### Phase 1: Restore — devlore-registry

- [ ] Restore the three `index.yaml` files from `b435dd0^`
- [ ] Verify `validate-yaml-schemas` still passes against the restored content

**Files**: `knowledge/{packages,migration,shared}/index.yaml` — Modify

### Phase 2: Fix the indexer — devlore-cli

- [ ] Read the existing index: `yaml.parse(file.read_text(index_path))` when present
- [ ] `merge_entries(files, existing)` — reuse whole entries by `name`
- [ ] Carry through unknown top-level keys
- [ ] Emit a generated-by header stating that metadata is preserved and edited by hand
- [ ] Fix the package indexer's provenance string
- [ ] Tests: metadata survives; new file appears; deleted file disappears; unknown section survives

No provider work is needed. `yaml.parse`, `yaml.encode`, `file.read_text`, and `file.write_text`
all exist and the extension already uses the last two.

**Files**: `star/extensions/com.noblefactor.devlore.Knowledge/commands/index.star`,
`star/extensions/com.noblefactor.devlore.Package/commands/index.star` — Modify

### Phase 3: Re-enable and prove — devlore-registry

- [ ] Re-enable the knowledge index step
- [ ] Confirm a run over the restored files produces **no diff**

A no-op run against curated content is the only convincing evidence the fix works. Re-enabling
without it repeats the original mistake.

### Phase 4: Remove the Go tool — devlore-cli

- [ ] `git rm -r cmd/devlore-index`
- [ ] Drop `devlore-index` from the Makefile's `TOOLS`
- [ ] Update `docs/package-reference.md` and `docs/package-hierarchy.md`

**Files**: `cmd/devlore-index/` — Delete; `Makefile`, two docs — Modify

## Risks

**Phase 3 could still be lossy in a way Phase 2's tests miss.** The domains differ; only
`packages`, `migration`, and `shared` are known to carry extras. Running against all six and
diffing is the check.

**Restoring is not the same as regenerating.** The restored files describe the file tree as it
was at `b435dd0^`. If files have been added or removed since, the first correct run will produce
a legitimate diff, and that must not be mistaken for further damage.

## Open Questions

- [ ] `manifest-authoring` has no sections at all. Under the strict rule that is legal if its
      directories are empty -- but is it an index nobody ever populated?
- [ ] The design brief treats an index as a *derived* artifact with its own validation. Making
      that true is "later"; this plan only stops the destruction.
