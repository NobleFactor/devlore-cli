---
issue: https://github.com/NobleFactor/devlore-cli/issues/721
title: "Nothing checks Starlark, and dead API calls survive for months"
status: draft
proof_run: TBD — defined by the charter's own decision (see Exit criteria)
created: 2026-08-27
updated: 2026-08-27
---

# Charter — `LintStarlark`

**Status:** `charter` — chartered 2026-08-27 from the docker package rewrite
([docker.devlore-package.md](docker.devlore-package.md)), where every phase script in the shipped
package turned out to call an API that no longer exists.

**No solution is assumed.** This charter states the problem, the evidence, and what has to be
discovered. The approaches at the end are candidates to evaluate.

## The observed failure

`devlore-registry/packages/docker/` shipped 28 `.star` files. Reading them against the provider tree
on 2026-08-26 found that the deploy path could not execute on any platform:

| Written in the package | Reality |
| --- | --- |
| `plan.package.install("docker-ce", "docker-ce-cli")` | No `package` namespace — it is `pkg`, and it takes a list, not varargs |
| `plan.verify(name, check=…, optional=True)` | No `Verify` method on any provider. **All four `verify.star` files are entirely these calls** |
| `plan.file.write(path=…, content=…)` | It is `plan.file.write_text(destination_path=…, content=…, mode=…)` |
| `plan.user.add_to_group(user, "docker")` | No `user` provider |
| `plan.download(url, dest)` | `appnet.download(url)` returns bytes; there is no dest form |
| `plan.notify(…)` | Does not exist |
| `Upgrade/install.star` | `UpgradePhaseOrder` is `{prepare, upgrade, migrate, verify}` — `install` is not an upgrade phase |

The registry's own CI validates knowledge schemas and package structure. None of it reads a `.star`
file's contents, so all of the above passed continuously.

## Why a syntax checker would not have helped

Every one of those lines is **syntactically valid Starlark**. `buildifier` — already installed at
`/opt/homebrew/bin/buildifier` — parses all of them without complaint, and always would have. The
defects are *resolution* errors: wrong namespace, absent method, wrong arity, wrong parameter name.
A general-purpose Starlark formatter knows nothing about devlore's provider surface and structurally
cannot see them.

This is the load-bearing observation. The cheap check is available today and would have caught
nothing. The check that matters requires knowing what `plan.*` actually offers.

## Why the cost is low

The machinery is already in the tree:

1. `go.starlark.net` is a direct dependency in `go.mod`; `go.starlark.net/syntax` is imported by ten
   files.
2. `cmd/star/provider/starindex/provider.go` **already parses Starlark** — `syntax.FileOptions{}`,
   walking `DefStmt`, `LoadStmt`, and `AssignStmt`. Its package doc is "AST-based indexing of
   Starlark source files."
3. The resolution ground truth is **generated**: `action_names.gen.go` per provider enumerates every
   valid `<namespace>.<method>`, and the `ParameterNames` tables in each `gen/provider.gen.go`
   enumerate every valid keyword argument. A checker that reads these cannot drift from codegen —
   which is the same discipline `Knowledge/extract.star` already applies to Go.
4. There is a home and a conspicuous hole: the star extension family carries `LintCopyright`,
   `LintGo`, `LintGoStyle`, `LintMarkdown`, `LintShell`, and `LintTools` — and no `LintStarlark`,
   across 162 `.star` files.

## Rules worth considering beyond dead references

Discovered while writing the Darwin package; each needs its own justification before adoption.

- **Lifecycle verbs in phase scripts.** `cmd/lore/lore/builder.go:31` denies `plan.assemble`,
  `plan.clear`, `plan.load`, `plan.run`, and `plan.save` to package scripts at runtime. A checker
  could reject them at author time instead of at execution.
- **Lambdas in package graphs.** A Starlark lambda is archived as a content-addressed
  `function.Resource`, and a graph carrying one currently fails receipt writing with
  `mmap …/.devlore/function/resource/sha256/…: no such file or directory`. Since receipts are what
  decommission compensates from, a package with a lambda cannot be removed. This is a separate
  defect that must be fixed on its own merits; whether the linter should also flag it depends on
  whether the fix lands first.
- **Phase entry point.** Every phase script must define `def <phase>(package, phase)` matching its
  filename and the phase order for its action directory.

## What has to be discovered

- [ ] How much of `plan.<namespace>.<method>(…)` resolution is tractable statically? Starlark is
      dynamic and `plan` sub-namespaces are attribute lookups. Handling the literal call shape
      likely covers every real package script, but the coverage should be measured, not assumed.
- [ ] Does the checker live as a star extension (`LintStarlark`, joining `make check` through
      `LintAll`), as a Go analyzer, or as a `star` subcommand? The extension family is the obvious
      home, but `star` currently fails to load its own extensions.
- [ ] Does it run against `devlore-registry` as well as `devlore-cli`? The failure that motivated
      this charter is in the registry, a separate repository, whose CI would need the checker
      available as a built artifact.
- [ ] Should `buildifier` be adopted alongside it as a parse gate, and if so with what warning set?
      `-module-docstring` and `-unused-variable` are both wrong for devlore — the latter fires on
      the mandated `(package, phase)` signature of every phase script ever written. Note also that
      buildifier's formatter rewrites `kwarg=value` to `kwarg = value`, which no file in the corpus
      uses and which buildifier cannot be configured to skip.

## Candidate approaches

Not a shortlist to choose from — each needs evaluating against the discovery items above.

1. **Star extension reading generated tables.** Parse with `go.starlark.net/syntax`, resolve calls
   against `action_names.gen.go` and `ParameterNames`. Joins `make check` via `LintAll`.
2. **Go analyzer in `cmd/star/provider/`.** A sibling of `starindex`, reusing its walker, surfaced
   as a `lint.*` action.
3. **Extend `starindex` itself.** It already extracts functions, loads, and globals; call-site
   resolution is an addition rather than a new component.
4. **buildifier only.** Rejected as sufficient by the evidence above, but retained here as the
   do-nothing-more baseline any other option must beat.

## Exit criteria

- [ ] Running the checker over `devlore-registry/packages/docker/` **as it stood on 2026-08-26**
      reports every row in the table at the top of this charter. That corpus is the regression
      fixture, and it is why the discarded scripts should be preserved somewhere before deletion.
- [ ] The checker reads the generated tables rather than a hand-maintained surface list, proven by
      adding a provider method and observing the checker accept it with no edit.
- [ ] Zero false positives across the 162 `.star` files in `devlore-cli`.

## Related

- [Docker devlore package](docker.devlore-package.md) — the rewrite that surfaced this
- `cmd/star/provider/starindex/provider.go` — the existing Starlark AST walker
- `cmd/lore/lore/builder.go:31` — the runtime denial this could enforce at author time
