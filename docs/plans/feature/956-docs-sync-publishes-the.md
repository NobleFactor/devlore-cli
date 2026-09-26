---
title: "The docs sync publishes the committed products: writ, lore and star; devlore-test's reference stays in the repository"
issue: https://github.com/NobleFactor/devlore-cli/issues/956
status: active
created: 2026-09-26
updated: 2026-09-26
---

# Plan: the docs sync publishes what ships

Lane 3 of #949. Ruled 2026-09-26: "we are committed to shipping star. we are not committed to shipping
devlore-test … let's go with what we committed, not what we might commit."

## Issue 956

`.github/workflows/docs-publish.yaml`, step "Update site content", copies `docs/cli/` whole into the
site's `src/content/cli/`. `devlore-docs` generates four trees, and #787 guarantees it does, so every
merge publishes devlore-test's eighteen reference pages for a program the release archive does not
carry. The site (devlore.noblefactor.com#457, lane 4) is being taught the committed products; the pages
it receives have to be those.

## Goals

1. The sync copies the reference for writ, lore and star, and nothing for devlore-test.
2. `devlore-docs` and #787's test are untouched: the repository keeps generating all four trees for its
   own use; publication is the one place the commitment is applied.
3. The guides copy is unchanged.

## Requirement

The step becomes:

```yaml
      - name: Update site content
        run: |
          # The site publishes the committed products (#956, ruled 2026-09-26): writ, lore and star.
          # devlore-docs generates devlore-test's tree too, for the repository's own use; it stays here.
          rm --recursive --force site/src/content/cli/
          mkdir --parents site/src/content/cli/
          for product in writ lore star; do
            cp "docs/cli/${product}.md" site/src/content/cli/
            cp --recursive "docs/cli/${product}/" site/src/content/cli/
          done
          rm --recursive --force site/src/content/guides/
          cp --recursive docs/guides/ site/src/content/guides/
```

Long options, as ruled 2026-09-26. The product list is written here rather than derived, because
this step is the statement of what the site publishes; a fourth product is one word.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The step

- [ ] The step as above; the workflow still parses as YAML and its shell passes `bash -n` and
      shellcheck when extracted

### Phase 3: Verify, then merge

- [ ] Dry run on this host: `make build` regenerates `docs/cli/`; the loop, run against a scratch
      directory, leaves `writ.md`, `lore.md`, `star.md`, `writ/`, `lore/`, `star/` and no
      `devlore-test*`; the file count equals the three trees' sum
- [ ] `star lint shell .`, `make vet-all`, `go test ./cmd/devlore-docs/` still pass (nothing in Go
      changes; the gate is run because it is the gate)
- [ ] PR script written, shown, and handed over
- [ ] After the merge: the run of `docs-publish.yaml` opens the site's PR with three trees and no
      `devlore-test`; that PR is the one lane 4's branch takes before its own build

## Out of Scope

- **Shipping devlore-test.** #955 gives every program the full surface; whether the archive and the
  installers carry devlore-test is the owner's ruling, later.
- **The site's schema** — lane 4, devlore.noblefactor.com#457.
