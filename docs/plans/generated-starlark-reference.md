---
title: "A generated Starlark reference for package authoring"
issue: https://github.com/NobleFactor/devlore-cli/issues/675
status: draft
created: 2026-08-25
updated: 2026-08-25
---

# Plan: a generated Starlark reference

## Goal

**Publish the surface an author actually writes, generated from the announcements, and make drift fail.**

AI will be asked to generate devlore packages (USER, 2026-08-25). What it needs is the Starlark reference
— command names, parameters, defaults, optionality, and which namespace a command is reachable from — not
the Go provider reference. An author never writes `activationRecord`, never sees a `*Receipt`, and never
types `destinationPath`.

## Why generated, not authored

Every hand-maintained surface in the tree is currently wrong, and all three report green:

- **`knowledge/package-authoring/`** lists eleven `file.*` operations. **Six do not exist** — `chmod`,
  `chown`, `configure`, `edit`, `sops`, `write` — and fifteen real commands are absent.
- **`star docs starlark`** is a 106-line tutorial teaching `fs.write(...)` and `fs.join(...)`. **There is
  no `fs` provider.**
- **`reference.yaml`** records provider blurbs and **zero commands**, so it changes only when a provider is
  added or removed.

A model given any of them emits calls that cannot resolve — the exact failure the knowledge base exists to
prevent. Three independent artefacts wrong simultaneously is not three mistakes; it is the absence of a
mechanism.

## The source already exists

Every `gen/provider.gen.go` carries the surface, machine-readable:

```go
"Copy":     {ParameterNames: []string{"source", "destination_path",
                                      "mode?={{ umask 0o755 }}", "user?=\"\"", "group?=\"\""}},
"Join":     {ParameterNames: []string{"*parts"}},
"Discover": {ParameterNames: []string{"path", "kind?=any", "after?"}},
```

Snake-case as authored, `?` optional, `=` default, `*` variadic. Alongside it `op.RoleModule|op.RoleAction`
decides whether a command is reachable immediately, only as `plan.<ns>.<command>`, or both — something a
generating model gets wrong constantly and cannot infer from a name. Method doc comments supply the prose,
already reachable through `goast.type_doc`.

This is generation from data the build already emits and the boot-discipline suite already validates. The
reference inherits that correctness rather than restating it.

## Steps

| # | Step | Done |
| --- | --- | --- |
| 1 | Emit a Starlark reference from the announcements: per command, its parameters and roles | ☐ |
| 2 | Include the method doc comment as each command's description | ☐ |
| 3 | Record the role so `plan.<ns>.<command>` versus immediate use is unambiguous | ☐ |
| 4 | Publish it to devlore-registry as the package-authoring context | ☐ |
| 5 | Retire the hand-maintained `file.*` list it replaces | ☐ |
| 6 | Regenerate or retire `star docs starlark` — a tutorial is fine, one naming absent providers is not | ☐ |
| 7 | **Make drift fail**: a check that breaks when the published surface and the announced surface disagree | ☐ |
| 8 | Pin it — a fixture asserting a known command's parameters, so a silent shape change is caught | ☐ |

## Order matters

**Step 7 is the point.** Steps 1–6 correct today's content; only 7 stops it recurring, and its absence is
why three artefacts are wrong at once. A reference that is generated but unchecked drifts the moment
someone edits the published copy by hand.

**Step 5 after 4.** Removing the hand-maintained list before its replacement is published leaves package
authoring with no vocabulary at all.

## Verification

Each step: `make check`, `gofmt -l`.

Whole-plan exit: the published reference lists every announced command and no absent one; `file.chmod` and
`fs.write` appear nowhere; and editing the published copy to disagree with the announcements fails a check.

## Related

- [#674](https://github.com/NobleFactor/devlore-cli/issues/674) — the defect this addresses
- [#670](https://github.com/NobleFactor/devlore-cli/issues/670) — CI as a strict superset; step 7 is the
  same principle applied to published artefacts rather than to checks
