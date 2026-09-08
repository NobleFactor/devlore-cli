---
title: "pkg accepts the canonical purl form it emits"
issue: https://github.com/NobleFactor/devlore-cli/issues/813
status: draft
created: 2026-09-08
updated: 2026-09-08
---

# Plan: pkg accepts the canonical purl form it emits

## Summary

`pkg.Resource` writes canonical purls and cannot read them. `buildCandidate` cuts a package string at its first
colon and hands the prefix to the platform's type resolver, so `pkg:winget/Vim/Vim?scope=machine` is refused as an
unknown package manager named `pkg` -- while the same function *emits* `purl.String()` as the resource's URI. This
plan makes the provider read the form it writes, and carries the namespace and qualifiers a purl exists to express
all the way to the driver.

## Issue 813

A high-priority bug under [#459](https://github.com/NobleFactor/devlore-cli/issues/459), found 2026-09-04 on the
Windows VM, blocking the Windows deploy of the personal layer. Its acceptance criteria settle the ruling the issue
records as pending: **option 1**, the provider accepts the canonical form through `platform.ParsePURL`. This plan
proceeds on that and does not reopen it. Closed by this plan's pull request.

## Goals

- [ ] `pkg:winget/Vim/Vim?scope=machine`, `pkg:brew/jq@1.7`, `brew:jq` and `jq` all construct.
- [ ] A namespace and qualifiers survive from the manifest to the package manager driver.
- [ ] The constructor round-trips its own URI.
- [ ] A malformed purl is refused with the parser's own message.
- [ ] The manifest schema documents every form the constructor accepts.

## Current State

Measured 2026-09-08 in the worktree, against develop at c0b6c517.

| Component | Status | Notes |
| --- | --- | --- |
| `platform.ParsePURL` | ✅ complete | the full grammar: type, namespace, name, `@version`, `?qualifiers`, `#subpath` |
| `platform.PURL.String()` | ✅ complete | emits namespace and qualifiers |
| The winget driver | ✅ **already correct** | `windows_managers.go:97` joins `Namespace + "." + Name` -- it has simply never been handed a namespace |
| `buildCandidate` | ❌ the defect | cuts at the first `:`; a purl's prefix is `pkg`, which is not a manager |
| `resource` | ❌ lossy | holds `name`, `typ`, `version` only; a namespace or qualifier has nowhere to live |
| `toPURL` and four siblings | ❌ lossy | five sites rebuild a `platform.PURL` from the resource, each dropping namespace and qualifiers |
| `schema/packages-manifest.json` | ❌ behind | `packages[].name` documented as a bare name, examples `gh`, `jq`, `ripgrep` |

The driver being ready is the load-bearing fact: nothing in `pkg/platform` changes.

## Requirements

### Requirement 1: The constructor reads the three forms

`buildCandidate` branches on the `pkg:` scheme before it looks for a manager prefix:

| Input | Parsed as |
| --- | --- |
| `pkg:winget/Vim/Vim?scope=machine` | `ParsePURL`; type `winget` resolved through `ResolvePurlType`, namespace `Vim`, name `Vim`, qualifier `scope=machine` |
| `pkg:brew/jq@1.7` | `ParsePURL`; type `brew`, name `jq`, version `1.7` |
| `brew:jq` | as today: prefix resolved as a manager |
| `jq` | as today: the platform's default type |

A `pkg:` string that `ParsePURL` rejects is refused **with the parser's message**, not re-read as a manager prefix:
once a string declares the scheme, it is a purl, and a fallback would turn a typo into a package named after it. A
purl whose type is not a manager this platform knows is refused as the manager prefix is today, naming the type.

### Requirement 2: The resource carries what a purl expresses

`resource` gains `namespace string` and `qualifiers map[string]string`. Both are identity and enter the URI, which
`String()` already renders; the **version stays out**, as it is today, so `jq` and `jq@1.7` remain one catalog entry.
Qualifiers are identity because `scope=machine` and `scope=user` are different installations, not one package
requested twice.

### Requirement 3: One projection, not five

`toPURL` becomes the single place a `platform.PURL` is built from a resource, returning every field, and the four
other sites that hand-build one -- two in `Compensate`, one in the pre-flight query, one in `Digest` -- call it.
**This is where the bug is actually fixed for the driver**: parsing a namespace changes nothing while every dispatch
path rebuilds a PURL without it, so winget would still be asked for `Vim` rather than `Vim.Vim`.

### Requirement 4: The schema says what is accepted

`schema/packages-manifest.json` documents `packages[].name` as the three forms with an example of each, so a manifest
author reads the accepted grammar rather than inferring it from three bare examples.

### Requirement 5: The reversal

The workaround in force is a commit on the Personal repository: the Windows manifest set aside or respelled
`winget:Vim.Vim`, which drops `scope=machine`. When this lands, the manifest returns to
`pkg:winget/Vim/Vim?scope=machine`. That verification needs the VM and is **the user's**, not a box this plan can
tick; it is listed as such.

## Design

**Why the scheme is checked before the colon cut.** `strings.Cut(raw, ":")` cannot tell a scheme from a manager
prefix, and `pkg` is a legal-looking prefix. Testing `strings.HasPrefix(raw, "pkg:")` first is what makes the two
grammars unambiguous, and it matches what `ParsePURL` itself requires.

**Why qualifiers are a map on the resource and not flags.** They are the driver's business, not the provider's; the
provider's job is to carry them intact. `windows_managers.go` reads `Namespace` today and can read qualifiers when a
driver needs one, without the provider learning any manager's vocabulary.

**What is deliberately not in scope.** No new Starlark surface: `Namespace()` and `Qualifiers()` accessors are not
added to the sealed `Resource` interface, because every consumer is inside the provider package and can read the
fields directly. If a workflow ever needs to read a namespace, that is a surface request under the pkg provider's
feature, not a bug fix. `pkg/platform` is not touched.

## Implementation Phases

### Phase 1: The failing tests

- [ ] A unit test per form of Requirement 1's table, red before the fix.
- [ ] A round-trip test: `DiscoverResource(env, r.URI())` yields the same catalog entry for a purl carrying a
      namespace and a qualifier.
- [ ] A projection test: the PURL handed to the driver for `pkg:winget/Vim/Vim?scope=machine` carries the namespace
      and the qualifier -- the assertion that fails today even with parsing in place.
- [ ] A refusal test: a malformed `pkg:` string reports the parser's message.

### Phase 2: The constructor and the resource

- [ ] `buildCandidate` branches on the scheme and parses through `platform.ParsePURL`.
- [ ] `resource` gains `namespace` and `qualifiers`; the URI carries both.
- [ ] The doc comment on `discoverResource` names the three forms it now really accepts.

### Phase 3: The projection

- [ ] `toPURL` returns every field; the four hand-built sites call it.

### Phase 4: The schema and closure

- [ ] `schema/packages-manifest.json` documents the three forms.
- [ ] A `devlore-test` fixture plans a purl-named package.
- [ ] The plan's status and the issue's acceptance boxes.

## Test Plan

| # | What it proves | Level | Fails when |
| --- | --- | --- | --- |
| 1 | `pkg:winget/Vim/Vim?scope=machine` constructs | unit | the scheme branch is removed |
| 2 | `pkg:brew/jq@1.7` constructs with version `1.7` | unit | the purl version is dropped |
| 3 | `brew:jq` and `jq` still construct | unit | the manager-prefix path regresses |
| 4 | A malformed `pkg:` string reports the parser's message | unit | a fallback to the prefix path returns |
| 5 | The URI round-trips through the constructor | unit | namespace or qualifiers leave the URI |
| 6 | The driver receives namespace and qualifiers | unit | any site rebuilds a PURL by hand |
| 7 | A purl-named package plans end to end | devlore-test | the provider path breaks |
| 8 | `writ deploy noblefactor --dry-run` passes on Windows with the canonical manifest | manual, **the user's** | the fix is incomplete on the real manifest |

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/provider/pkg/resource.go` | Modify | the scheme branch; `namespace` and `qualifiers`; the doc comment |
| `pkg/op/provider/pkg/helpers.go` | Modify | `toPURL` returns every field |
| `pkg/op/provider/pkg/provider.go` | Modify | the four hand-built PURLs call `toPURL` |
| `pkg/op/provider/pkg/resource_test.go` | Modify/Create | the form, round-trip, projection and refusal tests |
| `schema/packages-manifest.json` | Modify | the three accepted forms |
| `cmd/devlore-test/devloretest/data/` | Create | a fixture planning a purl-named package |

## Related Documents

- [#813](https://github.com/NobleFactor/devlore-cli/issues/813) -- this plan
- [#814](https://github.com/NobleFactor/devlore-cli/issues/814) -- manifests collide instead of unioning; the reason this purl was the only package the planner saw
- [#868](https://github.com/NobleFactor/devlore-cli/issues/868) -- the pkg provider has no design document; this plan's grammar belongs in it when it is written
- `pkg/platform/purl.go` -- the parser this plan calls
