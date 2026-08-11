---
title: "Writ Secret List"
status: proposed
created: 2026-08-10
updated: 2026-08-10
---

# Plan: Writ Secret List

## Summary

`writ secret list [<layer>...]` — inventory with a **recipient-drift audit**, chartered
2026-08-10. The reason it exists is the audit, not the inventory: a SOPS document's
recipient set is public metadata, readable with no credentials and no unwrap, so list
can show for every document *who can open it* — and mark every document whose actual
recipients differ from what the governing `.sops.yaml` would mint today. Drift is a
document encrypted before a rule change, a file a rekey sweep missed, or a foreign
recipient that should not be there. Delivered **with [rekey](writ-secret-rekey.md)**
(ruled): the drift audit is the sweep's completion proof — one work item, two commands.
Pure inventory alone would be `find`-replaceable and would not justify the surface.

## Design

1. **Surface**: `writ secret list [<layer>...]` — registered-layer names, all registered
   layers when none are named (the grammar init and rekey's sweep share); layer-scoped
   like the other authoring commands.
2. **Per document**: path (layer-relative), format, recipient set (key URLs, age
   recipients), and the drift marker from comparing the document's key groups against
   the resolution the governing `.sops.yaml` produces for that path today.
3. **Output** (ruled 2026-08-10): binds the shared result seam (`cli.AddOutputFlags`)
   unchanged — `--format` json (v1 default) | yaml | csv | template, with `template`
   retained as the user-authored escape hatch and `--filter`/`--jq` composing ahead of
   the formatter. **`text` is chartered, not built**
   ([result-text-formatter](result-text-formatter.md)): a generic tabular production
   over the same header inference csv already owns; when it lands, list's default
   flips from json to text — recorded here so the flip is deliberate, not drift. csv
   is **one row per document**: recipients joined with `"; "` in a single column,
   drift and drift-reason as their own columns — spreadsheets are csv's audience;
   machines take json plus `--jq`.
4. **Pipeline-free**: an effectless metadata read — no graph, no trace, no credentials,
   no network.
5. **The check logic is reusable, the command is presentation** — the drift comparison
   is the seed of a future `writ secret verify` and must not be welded to the table
   renderer.
6. Migration bonus: the cutover verification gains a writ-native "every document carries
   exactly the BYOK recipient" check.

## Verification

1. Fixtures with matching and mismatched recipient sets (drift detected, clean tree
   silent); containment refusal; multi-layer enumeration.
2. `make test`; dual-GOOS lint recount at zero.

## Open questions

None — the output-format rulings (2026-08-10) closed the format question; the text
default arrives with the chartered formatter
([result-text-formatter](result-text-formatter.md)).
