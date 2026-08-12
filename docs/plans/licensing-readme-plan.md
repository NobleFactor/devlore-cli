---
notice: "© 2026 Noble Factor LLC. Confidential."
type: Plan
title: Licensing and README Remediation Plan
audience: Founder, Engineering
purpose: Concrete licence assignments and README rewrites for devlore-cli and devlore-registry
status: Draft
version: "0.2"
date: 2026-08-13
---

# Licensing and README Remediation Plan

**Version:** 0.2
**Date:** 2026-08-13
**Related:** [04-strategy-revision-plan.md](04-strategy-revision-plan.md) §2.4–2.6, [ADR-022](../design/adr/022-licensing-strategy.md), 05-licensing-model-research.md (evidence base)

---

## 1. Scope

Two repositories, four distinct asset classes, and a currently inconsistent state
in `devlore-cli`. This document assigns a licence to each asset class, explains
the reasoning, and specifies the README rewrites.

It does **not** decide the registry *server* licence — that remains deliberately
deferred per 04-strategy-revision-plan §2.4. This document covers the content the
registry holds.

---

## 2. The Four Asset Classes

The word "registry" currently covers assets with opposite licensing needs. They
must be separated before anything is filed.

```text
┌─ devlore-cli ─────────────────────────────────────────────────────┐
│  Class 1: CLI source (devlore, writ, star, shared packages)       │
│           → Apache-2.0                                            │
└───────────────────────────────────────────────────────────────────┘
┌─ devlore-registry ────────────────────────────────────────────────┐
│  Class 2: DevLore packages (Starlark plans, manifests, schema)    │
│           → Apache-2.0                                            │
│                                                                   │
│  Class 3a: Consumption CAG (shipped to customers' own LLMs)       │
│            → CC BY 4.0 for the public tier                        │
│            → Proprietary subscription terms for the curated tier  │
│                                                                   │
│  Class 3b: Authoring CAG (steers pack generation from vendor docs)│
│            → TRADE SECRET. Never published, under any licence.    │
└───────────────────────────────────────────────────────────────────┘
```

### 2.1. Class 1 — CLI source: Apache-2.0

Decided. Reasoning in 04-strategy-revision-plan §2.4. Not MIT: Apache adds the
express patent grant with retaliation clause, §5 inbound-equals-outbound
contribution terms, and a NOTICE mechanism, and it is the unmarked default in Go
and cloud-native infrastructure.

### 2.2. Class 2 — DevLore packages: Apache-2.0

**Packs are code, not data.** A pack is Starlark with plan methods plus a
manifest. Creative Commons explicitly advises against CC licences for software,
and a CC-licensed pack sitting next to Apache-2.0 CLI source would create a
mixed-licence surface that scanners flag and legal teams stall on.

**Restrictive licensing here is self-defeating.** Adoption fills the catalog; the
catalog is the moat. A licence that makes an enterprise's scanner reject a pack
prevents the only thing that makes the catalog valuable. Additionally, copyright
over a sequence of install commands is thin — procedures and facts are largely
uncopyrightable; only expression is protected.

**The moat is freshness and verification, not the static artifact.** A pack
verified against last week's upstream release is worth paying for. The same pack,
copied and left to rot, is worth nothing in three months. That asymmetry is the
business, and it survives permissive licensing intact. Chainguard's images are
free; the SLA is not.

**Consistency with Class 1 matters practically.** Same licence across CLI and
packs means one scanner result, one NOTICE convention, one answer in procurement.

### 2.3. Class 3a — Consumption CAG: CC BY 4.0 (public tier)

These are the assets shipped to customers so their own model can work with
DevLore: the Starlark API surface, pack format, plan-method semantics,
troubleshooting context.

**These must be distributable or the product does not work.** The stated product
requirement is that CAG assets be usable in any LLM, running on each customer's
preferred model, never a DevLore-hosted one. An asset the customer is not
licensed to copy into their environment, load into a model context, and carry
into an air-gapped network fails that requirement on day one.

**CC BY 4.0** is the right instrument for the public tier: it is curated text
rather than code, CC BY is the most legible content licence to a reviewer, and
attribution is the only condition worth imposing. **CDLA-Permissive-2.0** (Linux
Foundation, purpose-built for data sharing, very short) is a reasonable
alternative if a data-specific licence reads better in context.

**Do not use ShareAlike or NonCommercial.** SA would attempt to reach the
customer's own derived context, which is both unenforceable in practice and
hostile to the BYOM promise. NC blocks the commercial users who are the target
market.

**The curated tier is not open-licensed at all.** The premium corpus — the
maintained, verified, continuously refreshed body of knowledge — is delivered
under subscription terms, not a public licence. It does not live in a public
repository. This is the "sell the packs, not the plumbing" line from
04-strategy-revision-plan §2.6, and it is where the revenue actually sits.

### 2.4. Class 3b — Authoring CAG: trade secret

The assets that steer a model when generating packs from vendor documentation
are the pipeline's differentiator. Strategy §10.4 already recommends keeping them
closed, and that recommendation survives every other revision in this document.

**They are not licensed. They are not published. They are not in a public repo
under any terms.** Protection is trade secret, which means access control and
confidentiality obligations, not a LICENSE file.

**Audit result (2026-08-13): the authoring CAG is public** — the full
`knowledge/` tree (prompts, exemplars, schemas) and `AUTHORING.md` have been in
the public MIT devlore-registry since ~January 2026, published continuously by
the `knowledge-extract.yaml` workflow. **Decision taken (Option A,
05-licensing-model-research §6.1):** the baseline authoring assets stay public
as an adoption feature; Class 3b is redefined around the assets that are
actually unpublished and defensible — the verification harness, test corpus,
freshness pipeline, and the curated premium corpus. Those are built in a
private repository from day one and never land here or in devlore-registry.

---

## 3. The Derived-Content Problem

**We cannot grant Apache-2.0 over content we do not own.**

Packs generated from vendor documentation may carry expression originating with
the vendor. Command sequences and procedures are largely uncopyrightable fact,
but prose descriptions, error text, and explanatory comments may not be. A blanket
"this file is Apache-2.0" is an overclaim on any pack containing vendor prose.

### 3.1. Mitigations

| Mitigation | Detail |
| ---------- | ------ |
| **Structural generation** | Packs should emit commands, parameters, and structured plan steps — not narrative prose copied or lightly reworded from source documentation. This is a pipeline requirement, not a licensing one, and it reduces exposure at the source. |
| **Scoped grant language** | The LICENSE grant should cover *Noble Factor's contribution* to each pack, with a NOTICE convention for third-party material, rather than asserting ownership of the whole file. |
| **Provenance metadata** | Per-artifact `license` and `source` fields in the manifest schema — not just at registry level. An enterprise scanner consuming a mixed-provenance catalog must be able to determine what it pulled. Adding this now is a schema field; adding it after the catalog populates is a migration plus an uncompletable backfill. |
| **Embedded third-party material** | Vendor-supplied scripts, checksums, and URLs embedded in packs need a NOTICE convention and should be referenced rather than copied wherever possible. |

### 3.2. Terms of service, separately

Systematic automated ingestion of vendor documentation may breach site terms of
service. That is contract, not copyright, and is a separate exposure that
licensing decisions do not address. Requires professional advice before the
pipeline runs at volume. (Cross-reference 04-strategy-revision-plan §5.)

---

## 4. Repository Structure

### 4.1. Recommendation: split by licence

Mixed licences inside one repository are read by SCA scanners as the most
restrictive licence found, which defeats the purpose of permissive assignment.
The cleanest structure:

| Repository | Contents | Licence |
| ---------- | -------- | ------- |
| `devlore-cli` | CLI source | Apache-2.0 |
| `devlore-registry` | Packs, schema, public consumption CAG | Apache-2.0 for packs/schema; CC BY 4.0 for CAG, per-directory |
| *(private)* | Authoring CAG, curated CAG corpus | Unlicensed, access-controlled |

Note that the Go module boundary matters independently: a Go module has one
licence as far as pkg.go.dev and most scanners are concerned. If any registry
code shares a module with the CLI, split it.

### 4.2. If the split is not practical

Use the **REUSE specification** (reuse.software) with SPDX headers per file and a
`LICENSES/` directory. It is designed exactly for mixed-licence repositories, has
a linter that runs in CI, and produces machine-readable output that scanners
handle correctly. A root LICENSE plus per-directory LICENSE files is the minimum
acceptable fallback.

### 4.3. Contribution terms

| Repo | Mechanism | Reasoning |
| ---- | --------- | --------- |
| `devlore-cli` | **DCO sign-off** | Apache-2.0 §5 already makes contributions arrive under the same terms. No CLA needed. Lower friction in exactly the repo where contributions are wanted. |
| `devlore-registry` (packs) | **CLA** | Required. If a third party submits a pack and we later sublicense the catalog to a self-hosting customer, those redistribution rights must be secured up front. Retrofitting across a populated catalog is not feasible. Also required for any future dual-licensing of the curated corpus. |

The CLA must be in place **before** the catalog populates, not after.

---

## 5. devlore-cli Remediation

### 5.1. Current state

Four conflicting assertions (audit 2026-08-12/13, see
[05-licensing-model-research.md](../../../noblefactor/devlore/business/05-licensing-model-research.md) §2.1):

```text
┌─────────────────────────┬──────────────────────────────────────────────┐
│ LICENSE file            │ SSPL-1.0  (MongoDB text, verbatim)           │
│ README body text        │ MIT                                          │
│ go.mod header comment   │ SPDX MIT + "All rights reserved."            │
│ Source SPDX headers     │ ~620 files SSPL-1.0, 59 MIT, 2 none          │
│ Intent                  │ Apache-2.0                                   │
└─────────────────────────┴──────────────────────────────────────────────┘
```

Published tags are cached immutably on proxy.golang.org — **~215 SSPL-era
versions are cached as of 2026-08-13**, and `release.yaml` cuts a new release
on every push to main/develop, so the paper freeze is not holding. Retraction
removes versions from selection but not from the cache. **Every additional tag
mints another immutable entry under the wrong licence.**

### 5.2. Licence actions (do first, in this order)

0. **Disable the Release workflow** (`gh workflow disable Release`) so no
   further SSPL-era tags are minted while this lands. Re-enable in step 6.
1. Replace `LICENSE` with the Apache-2.0 text; add `NOTICE` with the Noble
   Factor copyright line.
2. In the same commit: correct the README licence statement, fix the `go.mod`
   header comment (Apache-2.0 SPDX, drop "All rights reserved"), and sweep all
   ~680 source-file SPDX headers to `Apache-2.0` — including the
   `.github/workflows/*.yaml` headers (currently a mix of SSPL and MIT).
3. Same commit or same PR: locate and fix whatever stamps headers on new files
   (the count grew 616→621 in one day; `New-OpInventory` emits headerless
   generated files) so new files get Apache headers.
4. Same PR: remove `draft-llm-cache-augmented-generation.md` and
   `draft-llm-long-context-prompting.md` from the repo root (they disclose IP
   strategy; removal does not un-publish history — treat content as disclosed).
5. Add a `retract` directive in `go.mod` covering the full SSPL-era range
   `[v0.1.0-dev.20260127185200, <last-SSPL-tag>]`.
6. Re-enable the Release workflow. The next tag is the first Apache-2.0 tag.

### 5.3. README rewrite

The current README predates the rename, names only two CLIs, and states the wrong
licence. Rewrite around:

- **One product.** `devlore` as the primary binary; `writ`; `star` folded in as
  `devlore star`. No "lore" anywhere.
- **What it does, in two sentences**, leading with onboarding and roles — not with
  package management. The reader should understand "pick a role, get a configured
  machine" before anything else.
- **The iceberg example.** The Docker sequence (conflicts removed, repo added,
  packages installed, group membership, rootless, completions, hello-world
  verified) is the strongest single demonstration in the corpus. It belongs above
  the fold.
- **Install.** `go install`, the install scripts, Homebrew and MacPorts when they
  land. Note that `install.sh` and `install.ps1` already exist.
- **Cross-platform statement.** macOS, Linux, Windows — genuinely, including the
  PowerShell provider. This is the differentiator against every containerised
  competitor and it is currently understated.
- **Relationship to devlore-registry**, in one paragraph, with a link.
- **Licence: Apache-2.0**, stated once, correctly.
- **Links to devlore.org**, not devlore.noblefactor.com.
- **Contributing:** DCO, with a link to CONTRIBUTING.md.

Also: clean the repo root (`commit_msg.txt`, `TEST_BREAKS.md`, stack-comparison
drafts, `GITHUB-ISSUES.md`) in the same pass. It is the first thing a visitor sees.

---

## 6. devlore-registry Remediation

### 6.1. Licence actions

1. Decide the repository split per §4 before filing anything.
2. `LICENSE` — Apache-2.0 for packs and schema.
3. `LICENSES/CC-BY-4.0.txt` plus per-directory LICENSE for public consumption CAG,
   or adopt REUSE with SPDX headers throughout.
4. `NOTICE` with the third-party material convention from §3.1.
5. Audit for any authoring CAG material in a publishable location (§2.4).
6. Add `license` and `source` fields to the pack manifest schema.
7. CLA in place before accepting external pack contributions.

### 6.2. README rewrite

This README does more work than the CLI's, because it is where contributors and
enterprise reviewers both land.

- **What the registry is:** the curated catalog of DevLore packages and CAG
  assets. State plainly that content is currently served from GitHub and that OCI
  is the planned distribution mechanism.
- **The licence split, as a table.** Packs Apache-2.0; public CAG CC BY 4.0;
  curated corpus under subscription. A reviewer should not have to infer this.
- **Provenance and verification.** What "verified" means, how packs are signed,
  what the trust root is. This is the credibility section and it should be
  specific.
- **How to contribute a pack:** the review process, the CLA, the provenance
  requirements, what gets rejected. Curation as a quality gate is a feature —
  describe it as one.
- **What is *not* here:** authoring CAG, the curated premium corpus. Saying so
  explicitly is better than leaving a reviewer to wonder.
- **Self-hosting and air-gap:** even as a stub. This is what the enterprise
  reader is looking for.

---

## 7. Trademark and Trust Root

Neither is a licence, and both do more enforcement work than any licence
available:

- **Trademark** — file "DevLore" with USPTO; EUIPO search first given the Dutch
  agency at devlore.nl. A permissive code licence plus a firm trademark policy
  (Mozilla/Apache model: allow fair use, prevent confusion) gets most of what
  restrictive licensing was reaching for, at a fraction of the adoption cost.
  Publish TRADEMARK.md.
- **Trust root** — the client's default signing root is the practical control
  over what appears in the default experience. Technical, not legal, and it works
  regardless of how permissively the code is licensed. Align with cosign/sigstore
  conventions rather than a bespoke chain.

---

## 8. Task Sequence

**This week**

1. devlore-cli: disable Release workflow (step 0) — first action, before any
   other merge
2. devlore-cli: the §5.2 remediation PR (LICENSE, NOTICE, README, go.mod
   header, ~680-header sweep, header-template fix, draft-llm removal, retract)
3. devlore-cli: re-enable Release workflow after merge
4. ~~Audit for authoring CAG in publishable locations~~ **Done — result and
   decision recorded in §2.4**

**This month**

5. Decide the devlore-registry repository split (§4)
6. File licences in devlore-registry per §6.1
7. Add `license` and `source` fields to the manifest schema
8. Draft the CLA; draft CONTRIBUTING.md for both repos
9. Rewrite both READMEs per §5.3 and §6.2
10. SPDX headers / REUSE adoption with CI linting

**Before the catalog populates**

11. CLA operational
12. TRADEMARK.md published; USPTO and EUIPO filings underway
13. Legal review of §3 — scoped grant language and the derived-content position

---

## 9. Open Items

1. Registry **server** licence — deliberately deferred (04-strategy-revision-plan §2.4)
2. Whether the curated CAG corpus is delivered as files under subscription terms or
   gated by registry credentials — affects whether an EULA or a service agreement
   is the governing document
3. Catalog mirroring rights for self-hosting licensees
4. Whether CC BY 4.0 or CDLA-Permissive-2.0 reads better to enterprise reviewers
   for public CAG

---

## 10. Document History

| Date | Version | Change |
| ---- | ------- | ------ |
| 2026-08-13 | 0.2 | Amended per 05-licensing-model-research v0.2: four-way conflict recorded, step 0 (pause Release workflow), header sweep + template fix + draft-llm removal folded into §5.2, retract range specified, Class 3b audit closed with Option A decision. |
| 2026-08-11 | 0.1 | Initial. Four asset classes, derived-content handling, repository structure, README rewrites for both repos. |
