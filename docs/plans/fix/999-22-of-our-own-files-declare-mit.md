---
title: "The shipped extension.yaml files declare Apache-2.0, as devlore-cli ships"
issue: https://github.com/NobleFactor/devlore-cli/issues/999
status: active
created: 2026-11-06
updated: 2026-11-06
---

# Plan: the shipped extension.yaml files declare Apache-2.0

## Summary

devlore-cli ships under Apache-2.0, and 21 of the `extension.yaml` files that ship with star declared MIT on their
first line: the 17 under `cmd/star/extensions`, embedded in the star binary, and the 4 under `star/extensions`, which
the release archive installs to `share/devlore/star/extensions`. This plan changes those 21 lines to Apache-2.0. It is
the first part of #999; the rest of #999 follows on this branch.

## Rulings

- 2026-11-06: devlore-cli ships under Apache-2.0, not MIT.
- 2026-11-06: files that release are product, and the unit of distribution is the file.
- 2026-11-06: the extensions that ship with star are embedded in the binary.
- 2026-11-06, on the owner's order: the shipped `extension.yaml` files are fixed now, ahead of the rest of #999.
- 2026-11-06: `.goreleaser.yaml` and the `Makefile`, which do not ship, carry Apache-2.0, the same as everything
  else.
- 2026-11-06: the lint provider's test fixture, a verbatim copy of devlore-registry's `packages/docker`, keeps its
  MIT headers, as devlore-registry's files; the registry's own CI checks its live packages
  (NobleFactor/devlore-registry#96).

## Implementation Phases

### Phase 1: the 21 shipped extension.yaml files

- [x] Line 1 of each changed from `# SPDX-License-Identifier: MIT` to `# SPDX-License-Identifier: Apache-2.0`, and
      nothing else changed; no file under either extension tree declares MIT; `make quality-gate` green

### Phase 2: the rest of #999

- [x] `.goreleaser.yaml` and the `Makefile`: line 1 of each changed to Apache-2.0, as ruled, and nothing else
      changed; `make quality-gate` green
- [ ] `star lint copyright` reads `.yaml` files, so that a shipped file declaring the wrong license fails the gate
- [ ] The pull request

## Open Questions

- [x] `.goreleaser.yaml` and the `Makefile` declare MIT and do not ship. Which license do they carry? Ruled
      2026-11-06: Apache-2.0, the same as everything else.
- [x] The 42 files of `cmd/star/provider/lint/testdata/docker-package/` declare MIT on line 1. They are a copy of
      devlore-registry's Docker package, and devlore-registry is licensed MIT. Do they keep MIT, as the registry's
      files, or carry Apache-2.0, as devlore-cli's test data? Ruled 2026-11-06: they keep MIT, and the fixture's
      README says why, so that no reader or license scanner is confused.
