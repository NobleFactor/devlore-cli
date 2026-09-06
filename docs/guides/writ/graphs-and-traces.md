---
title: "Graphs and Traces"
description: "Audit writ runs, detect drift, and verify documents through the execution store"
tool: "writ"
category: "reference"
order: 5
---

# Graphs and Traces

Every `writ` run persists two documents. The **graph** is the immutable plan — what the run
set out to do. The **trace** records one execution of that graph — what actually happened,
dispatch by dispatch. One graph accumulates a trace per execution, and everything writ
reports about deployed state is derived from these documents, never from guesswork.

They serve four purposes:

- **Auditing**: see exactly what ran, when, and with what outcome
- **Drift detection**: classify the live filesystem against what the run recorded
- **Rollback**: each trace carries the per-action undo records (receipts) a rollback replays
- **Verification**: prove a document is intact and who published it

> **Terminology**: a *receipt* is the per-action undo record riding inside a trace — the
> evidence a rollback needs. The persisted documents themselves are graphs and traces.

## The Execution Store

Documents live in the devlore state directory:

```
${XDG_STATE_HOME:-~/.local/state}/devlore/
├── graphs/
│   └── sha256-<hex>.yaml              # the plan, written once, keyed by its checksum
├── traces/
│   └── sha256-<hex>/                  # one subdirectory per graph
│       ├── 20260807T163000Z.yaml      # one trace per execution, UTC-timestamped
│       └── latest.yaml -> 20260807T163000Z.yaml
└── index.ndjson                        # the run index: one line per persisted document
```

A graph persists once — re-running the same plan reuses it. Each run appends a timestamped
trace under the graph's subdirectory and repoints `latest.yaml`. The run index is how
`writ reconcile` finds everything; a missing index is a hard error, never a silent rescan.

## What a Trace Records

- Every dispatch's outcome, with per-attempt history and the action's receipt
- The run's terminal status (completed, degraded, failed, paused)
- The as-deployed content identity of each file the run produced — what drift
  classification compares against

## Auditing and Drift: `writ reconcile`

```bash
writ reconcile                    # Report everything writ has deployed
writ reconcile noblefactor        # Report one project
writ reconcile -o json            # Machine-readable report
```

Status is report-only and store-derived. Each entry classifies the live filesystem against
the run's recorded content identity, and every finding names the command that repairs it:

| Indicator | Meaning | Repair |
|-----------|---------|--------|
| ✓ Linked / Copied | Present and as deployed | — |
| ✗ Missing | Deployed target is gone | `writ deploy` |
| ⚠ Conflict | Something else occupies the target | — |
| ? Orphan | Target's source no longer exists | `writ decommission` |
| ↑ Stale | Source changed since the run | `writ upgrade` |
| M Modified | Target edited out-of-band | `writ upgrade --force` |
| M Modified-or-stale | Differs, but the run predates recorded content identity — attribution indeterminate | `writ upgrade` |

## Verification: `writ verify`

Integrity is automatic: every document carries a checksum, and any load — status, upgrade,
resume — refuses a document whose checksum is missing or wrong. There is no unverified
read path.

Authenticity is `writ verify`. Documents are signed automatically at persist time when a
signing key resolves (your SSH key, `~/.ssh/id_ed25519`, or a generated local key);
verification checks the ssh-ed25519 signature and resolves the publisher's key against
your `allowed_signers` trust list:

```bash
writ verify ~/.local/state/devlore/graphs/*.yaml
writ verify --signing-policy=reject_external ~/Downloads/shared-plan.yaml
```

The `--signing-policy` ladder decides what a verification outcome does to the exit status:

| Policy | Behavior |
|--------|----------|
| `ignore` | No verification at all |
| `report` | (default) Report every outcome; never fail |
| `reject_external` | Reject unsigned/invalid/untrusted documents from outside this machine's own store |
| `reject` | Reject anything that is not valid |

Trust is yours to declare: the `allowed_signers` file (default
`<config>/devlore/allowed_signers`, override with `--allowed-signers`) lists the publisher
keys you accept. A valid signature from an unlisted key reports as untrusted.

## Troubleshooting

### "checksum mismatch"

The document changed after it was written — manual editing, corruption, or tampering. The
load is refused. Re-run the deployment to persist a fresh graph and trace.

### Unsigned documents

Signing is best-effort: with no resolvable key, documents persist unsigned and
`writ verify` reports the fact. Under the default `report` policy this never fails a run.

### "run index" errors from `writ reconcile`

Status refuses to report from silence. If the index is missing or incomplete, its store
health section names what is missing; re-running the affected deployments regenerates the
documents and index entries.
