---
step: 55
title: "Curate internal/ before it moves to cmd/internal/"
status: charter — chartered 2026-08-14; gates the internal→cmd/internal sweep
proof_run: TBD — per-package decision recorded, then the sweep moves only what survives
parent: ../../phase-8.md
---

# Step 55 — Curate `internal/` before it moves

**Status:** `charter` — chartered 2026-08-14 while sizing the `internal/` → `cmd/internal/` sweep.

**A move is the moment to stop carrying things.** Rewriting 97 import lines is the cheapest opportunity
this repository will get to ask, per package, whether it should exist at all. Moving first and curating
later means every deletion pays the import-rewrite tax twice, and in practice means the curation never
happens.

**No dispositions are assumed here.** What follows is the evidence gathered while sizing the sweep, and the
question each package has to answer. The answers are the step's work.

## Method

Every package gets one of three verdicts, recorded with its reason:

- **Keep** — moves to `cmd/internal/<name>` as-is.
- **Replace** — a maintained library or an existing package in this repository already does the job.
- **Delete** — nothing uses it, or its job stopped existing.

"Nothing imports it" is evidence, not a verdict: test harnesses are legitimately importer-less. The test is
whether removing it changes anything a user can observe.

## The inventory, with what is already known

| Package | Files | Importers | Question |
| --- | --- | --- | --- |
| `pwsh` | 2 | **none — not even tests** | **DELETED 2026-08-17.** Nothing imported it; it was a second process-execution path beside `pkg/process`, which declares itself the single bridge to `os/exec`; the one-shot execution it wrapped is `powershell.exec` ([3.5.9](../../../../architecture/3.5.9-powershell-provider.md)); and its own package-doc example called a constructor that does not exist (`NewExecuting`). The only argument for keeping it was [6-execution-topology.md](../../../../architecture/6-execution-topology.md)'s citation as prior art for the elevation worker — but that section keeps the *pattern* and explicitly discards the stdin/marker protocol, which was the only thing the code held. Section rewritten to stand without it. |
| `console` | 5 | 3 files in `cmd/writ/…/migrate` | **Keep or replace?** A Bubble Tea-style console (themes, styles, `MockSession`, `Model`) used by exactly one command's migrate flow, while the rest of the repository narrates through `pkg/status` + `pkg/sink`. Two presentation layers, one of them with a single consumer. |
| `config` | 6 | cmd, internal | **Keep or replace?** Exports `Config`, `Path`, `Load`, `Save` — a simple YAML round-trip — while `pkg/devconfig` owns sections, layering and set-by tracking. Whether the simple one is a legitimate front end or a survivor of the older design needs answering. |
| `credentials` | 4 | internal | **Keep or replace?** Keystore access is a solved problem with maintained libraries; whether ours does something they do not is the question. |
| `document` | 2 | cmd, internal | **Keep.** Moves to `cmd/internal/document`. Its `pkg/op` dependency dissolved when `op` took ownership of trace persistence (step 54 discussion), so nothing outside `cmd/` needs it. |
| `model` | 9 | cmd, internal | **Keep or replace?** The LLM provider abstraction. Same question as `credentials`: is this ours to maintain? |
| `lorepackage` | 11 | cmd, internal | Keep, presumed — the lore package model is product logic. |
| `registry` | 2 | cmd, internal | Keep, presumed — OCI registry access is product logic. |
| `manifest` | 2 | cmd | Keep, presumed. |
| `e2e` | 3 | none | **Keep** — a test harness plus its two test files; importer-less by design. |
| `tools/docgen` | 3 | `cmd/devlore-docs` only | **Fold** into its only consumer, which becomes `cmd/devlore-doc`. |

## Sequencing

Curate, then move. A package deleted after the sweep costs a second import rewrite across the same files;
a package deleted before it costs nothing.

The one exception is `document`, whose disposition is already settled and which six other packages depend
on — it can move with the first wave regardless of how the rest resolve.

## Exit criteria

- [ ] Every package above carries a recorded verdict with its reason.
- [ ] Deletions and replacements land **before** the sweep, not after.
- [ ] `docs/package-reference.md` and `docs/package-hierarchy.md` match reality — both currently document
      `internal/pwsh` as though it were in use.
- [ ] The sweep moves only what survived.

## Related

- [step 54](54-xdg-anchors-on-windows.md) — the migration that made the sweep's ordering matter.
- [windows-native-permissions.md](../../../windows-native-permissions.md) — phase 7 is the sweep itself.
