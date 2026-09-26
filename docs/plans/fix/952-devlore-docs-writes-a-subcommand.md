---
title: "devlore-docs writes a subcommand named index as index.md, which the site reads as its directory's page: two star pages collide and the site has not built since 2026-09-24"
issue: https://github.com/NobleFactor/devlore-cli/issues/952
status: active
created: 2026-09-26
updated: 2026-09-26
---

# Plan: every generated page states its own URL path

Companion: devlore.noblefactor.com#457 (the site's schema). Found on site#455, the last box of #948.

## Issue 952

`devlore-docs` names a group's page `<group>.md` and each subcommand `<group>/<name>.md`. Two star
subcommands are named `index`, so `knowledge/index.md` and `package/index.md` exist, and Astro reads a
file named `index.md` as its directory's page. Each collides with the group page beside it; the site's
build has failed on every push since the first sync carrying star's pages (2026-09-24 07:34), and
production is frozen at the build before it.

The fix, ruled 2026-09-26: every page states its URL path in its frontmatter, `slug`, the field Astro
reads in place of the path it would derive. The value is the command's words joined with `/`, which is
what Astro derives today for every file but the two named `index.md`; no existing URL changes.

## Goals

1. `knowledge/index.md` carries `slug: "star/devlore/knowledge/index"`, and `package/index.md` its own;
   the site's collection has no two entries with one slug.
2. Every other page carries the slug Astro already gave it, so nothing moves.
3. A test fails if a page's slug ever disagrees with its path, so the next subcommand someone names
   `index` cannot bring this back.

## Requirements

### Requirement 1: `PageData.Slug`

`cmd/devlore-docs/template.go`: `PageData` gains `Slug string`, and the frontmatter gains one line after
`title`:

```
slug: "{{ .Slug }}"
```

`cmd/devlore-docs/generator.go`: `BuildPageData` sets `Slug` to `strings.Join(commandParts(cmd), "/")`.
`outputPath` is unchanged: the file layout stays the command tree; only the URL is stated rather than
derived. Astro's `cli` collection schema is a non-strict `z.object`, and `slug` is a field Astro reserves
and removes before validation, so the site accepts the line without a schema change.

### Requirement 2: Tests

- `TestBuildPageData` asserts `Slug` for a root, a subcommand, and a nested subcommand.
- A new `TestGenerateTree_SlugMatchesPath` in `generator_test.go`: a command tree that includes a
  subcommand named `index`, generated into a temp dir; every `.md` under it is read, its `slug:` line
  parsed, and compared with its path relative to the output directory minus `.md`. This is goal 3's
  guard, and it reproduces the collision on the tree before the fix.
- `TestRun_EveryProgramHasATree` (#787's guard) is unchanged and still passes.

### Requirement 3: The generated tree is not committed

`docs/cli/` is not tracked; `make build` regenerates it and `docs-publish.yaml` syncs it to the site on
push to `develop`. So the fix reaches the site by the merge alone: the next sync carries pages with
`slug`, and, with site#457 merged, the site builds.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The change

- [ ] `PageData.Slug`, the template line, `BuildPageData` (Requirement 1)
- [ ] The tests (Requirement 2); `gofmt -w` on every edited file, then `gofmt -l` clean

### Phase 3: Verify

- [ ] `go test ./cmd/devlore-docs/` passes, and the new test fails when the `Slug` line is removed
      (run once with the fix reverted, to prove the guard bites)
- [ ] `make build` regenerates `docs/cli/`; `grep -c '^slug:' docs/cli` counts every page;
      `knowledge/index.md` and `package/index.md` carry their own slugs; no two files share one
- [ ] The repository's gates on this host: `make vet-all`, `go test ./cmd/devlore-docs/`, `star lint
      shell .`. `make lint-all` and `star lint go` need golangci-lint v2.13.2, which is not on
      DANOBLE-UD24-1; CI runs them, and installing the pin here is the owner's call, not the plan's

### Phase 4: Merge, sync, build

- [ ] PR script written, shown, and handed over
- [ ] After the merge: `docs-publish.yaml` opens the site's sync PR carrying pages with `slug`; with
      site#457 merged, the site's build goes green for the first time since 2026-09-24. Stays open until
      that build is seen

## Out of Scope

- **The site's schema and index page** — site#457.
- **Renaming `star devlore knowledge index` or `package index`.** A rename would also dissolve the
  collision; the owner has that question on devlore-registry#92's naming, and the generator fix holds
  regardless.
- **`--dry_run`** on `knowledge index`, an underscore where every other flag has a hyphen. Command-line
  epic, its own issue.
