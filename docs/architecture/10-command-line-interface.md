# Command Line Interface — One Convention, Every App

> **Status:** design (draft, 2026-08-28). Specifies the command-line surface every binary in the suite
> presents: the command grammar, the flag set, and above all where output goes. No implementation.
> Companion: [`10-command-line-interface.status.md`](10-command-line-interface.status.md).
> Epic: [#740](https://github.com/NobleFactor/devlore-cli/issues/740).
>
> **Relationship to the neighbours.** [`configuration.md`](configuration.md) owns how settings are declared,
> discovered, and rolled up; this document owns only the *flag* end of that precedence chain.
> [`6.3-command-execution.md`](6.3-command-execution.md) owns the form of a command devlore *runs*; this
> document owns the form of a command a *user* runs. [`2.8-eventing-infrastructure.md`](2.8-eventing-infrastructure.md)
> owns the narration/event/diagnostic stream split inside the engine; this document owns where those streams
> land on a terminal.

## Thesis

**A result goes to stdout. Narration goes to stderr. Documents go to the execution store. One shared flag
set decides how each is rendered and where the store lives, and no command invents its own.**

The suite already has the mechanism —
[`cmd/internal/cli/output.go`](../../cmd/internal/cli/output.go) today binds `--filter`, `--format`, `--jq`,
and `--template`, and composes a `result.Pipeline` of filter, formatter, and sink. What it has never had is a
statement that using it is mandatory, or a `--output` flag to say *where*. Both absences are why forty-five of
forty-six commands improvise.

## 1. Scope and principles

This document governs four binaries -- `devlore-test`, `lore`, `star`, and `writ`. They **will use the
same common flag set**, in full: every flag in §4 is available on every command of all four.

It follows [clig.dev](https://clig.dev) except where it says otherwise, and §15 states each divergence with
its reason.

Three principles, in priority order:

1. **Consistency across the suite beats local convenience.** A user who learns `--output` in `lore` must find
   it means the same thing in `writ`. A command with a good reason to differ still does not.
2. **Machine-readable by default, human-readable by choice.** Output is data first; the rendering is a flag.
3. **The convention is code, not prose.** A rule that lives only in a document drifts. Every rule in §5–§7 has
   a test in §13, and a rule that cannot be tested is a preference, not a rule.

## 2. The suite

| Binary | Audience | Owns | In scope |
| --- | --- | --- | --- |
| `devlore-test` | developer | the graph test harness: plan, execute, verify | **yes** |
| `lore` | user | package deployment, manifests, receipts | **yes** |
| `star` | user | Starlark tooling: lint, complexity, codegen | **yes** |
| `writ` | user | dotfile/layer deployment, secrets, adoption | **yes** |
| `devlore-docs` | developer | generates `docs/cli/**` from the command tree | not yet |
| `devlore-index` | developer | knowledge index maintenance | not yet |
| `devlore-inventory` | developer | provider inventory generation | not yet |

Developer tooling is held to the same convention. A harness whose output is inconsistent costs the same
confusion as a shipped command, and `devlore-test` is the proof: its three output streams were wired to the
wrong artifacts for months without anyone noticing ([#738](https://github.com/NobleFactor/devlore-cli/issues/738)).

## 3. Command grammar

Commands are `<binary> <noun> <verb>` or `<binary> <verb>` where the noun is implied by the binary. `lore
manifest create` is noun-verb; `lore deploy` is a verb whose noun is the binary's whole subject.

- A subcommand group is a noun. It takes no action of its own and prints help when invoked bare.
- A leaf is a verb. It does exactly one thing.
- Names are lowercase, hyphenated when compound, never abbreviated (`decommission`, not `decom`).

This is the same rule the Starlark surface follows for `devlore.<noun>.<verb>`; the two trees are separate
namespaces with one naming convention.

## 4. Arguments and flags

**Positional arguments** are the objects the verb acts on, and only that. A command takes positionals when the
list is open-ended (`lore deploy <package>...`) and flags otherwise.

**Persistent flags** are declared once on the root command and mean the same thing everywhere:
`--config`, `--silent`, `--verbose` / `-v`.

**The common set is persistent too.** All four in-scope programs register the whole set on their root
command, so every command of every one of them accepts every flag. This is how `aws` and `az` do it —
`--output` is global there, not per-command — and it is what makes the convention learnable: a user who
learns `-o yaml` on `lore` types it on `star` without checking.

**A flag with nothing to act on is inert, not an error.** For renderings this is established practice, and
was verified rather than assumed — both of these exit 0 and print output byte-identical to the same command
without the flag, because each prints a fixed human format that `--output` cannot touch:

```
aws configure list --output json
az --version --output json
```

For `--store` the same rule is **reasoned, not precedented**. None of the five surveyed tools has a document
store, so none can supply an example. The argument is internal: a command that writes no document leaves a
valid store untouched, which is a no-op and not a failure — the flag named a real place that this command
had no reason to write to.

Uniform availability is worth more than pruning flags per command, and pruning per command is what produced
twelve hand-rolled variants.

**The reserved set.** These names are owned by the shared convention. A command may not redefine, retype, or
repurpose them:

| Flag | Short | Type | Meaning |
| --- | --- | --- | --- |
| `--filter` | | string, repeatable | `field=value`, AND logic |
| `--jq` | | string | jq expression, applied after `--filter` |
| `--output` | `-o` | string | How the result is rendered. §7 |
| `--store` | | string | The execution store's root. §6 |
| `--template` | | string | Template body, with `--output template` |

`--output` / `-o` selects the **rendering**, matching `az` and `kubectl`. It is deliberately not a
destination: a user typing `-o json` must never be silently creating a file named `json`. See Design
decisions.

**No boolean format flags.** `--json` is not a flag; `--output json` is. A boolean cannot express a third
format, which is why two `writ` commands cannot emit YAML today.

## 5. The output model

**The split.** Everything a command emits is one of three things:

- A **result** — the answer the user asked for. Machine-readable, JSON by default. Always stdout.
- **Narration** — progress, warnings, errors, diagnostics. For a human. Always stderr.
- **Documents** — definitions and traces. Never stdout; they go to the execution store (§6).

A result is written to a file by shell redirection, not by a flag: `lore list > packages.json`. The suite
adds no `--output <file>`, because stdout already is one.

The consequence that makes this worth enforcing: `command -o json | jq` works, always, with narration
still visible on the terminal. Nothing a command says about its progress can corrupt what it returns.

**Narration goes through the status narrator** — `cli.Note`, `cli.Warn`, `cli.Error`, `cli.Success` — never
through `fmt.Print*` or `os.Stdout`. A direct write to `os.Stdout` from a command package is a defect, and §13
tests for it.

**Exit status is not output.** A command reports failure through its exit code (§8), not by printing to
stdout.

## 6. `--store`: the execution store

The third stream is not an output directory. It is a **store**, with a layout, a cardinality rule, and an
index, defined in [`cmd/internal/cli/store.go`](../../cmd/internal/cli/store.go):

- A **definition** (`op.Graph` today, `Definition` after the workflow rename) is the immutable plan. It
  persists **once**, under `GraphsDir()`, keyed by its checksum.
- A **trace** is one execution's serialized executor state. It persists **per run**, under `TracesDir()`, in a
  per-definition subdirectory, tied back through `Trace.GraphChecksum`.
- The cardinality is one definition to many traces, and a per-definition run index records them.

That structure is what makes a trace useful beyond the run that produced it: reconciliation, troubleshooting,
dependency analysis, and — the reason it must be durable rather than incidental — pausing a run and resuming
it later.

`--store <dir>` names the store's **root**. It relocates the whole store: both subdirectories together, with
the checksum-keyed layout and the run index intact. A command that writes loose files into `--store` has not
implemented this flag — it has broken the tie between a trace and the definition it ran.

The store is not new, and it is already user-visible: `writ secret`'s help says "graph and trace persist to
the execution store with receipts recorded." What has never existed is a way to point at a different one.

**Why not `--output`.** `az` and `kubectl` both use `--output` / `-o` for the *format*; `docker build` uses it
for a destination. The name is ambiguous across the very tools this suite borrowed from, and the meaning most
users carry — format — is the one we adopt for `--output` itself. A flag that means the opposite of what a user
expects is worse than a flag they have never seen. "Store" collides with nothing and names a thing rather
than a direction.

## 7. `--output`: how the result is rendered

`--output` / `-o` selects the rendering. The formatter set, alphabetically, is `csv`, `json`, `none`, `table`,
`template`, and `yaml`; `--template` supplies the body when `--output template`.

**JSON is the default**, following `az`. A command's result is data first, and the common case — a script or a
pipe — should need no flag at all.

**A command does not switch format based on whether stdout is a TTY.** A pipeline that behaves differently
when observed is unreproducible. A human wanting a table asks for one: `-o table`.

**Adding a formatter** is a change to `pkg/result`, never to a command. A command that needs a rendering the
set does not have uses `--output template`.

**One table formatter, no exceptions.** `table` is a general rendering and belongs in `pkg/result` like the
others. No command owns its own. The current hand-rolled table in `lore`'s `runSearch`
(`cmd/lore/lore/commands.go:525-556`) is not an argument for a second one: it writes through `fmt.Printf`
rather than the sink, hard-codes column widths, measures truncation in bytes, and folds a boolean into a
name column as a `*` suffix. A shared formatter fixes all four, and `installed` stays a field that
`--output json` can emit.

A **domain** rendering is a different question. `lore list` registers `--format manifest`, which means
something only to lore. Under this convention that is either `--output template` with a manifest template,
or a lore-specific flag that is not part of the common set — but it is never a value added to the shared
formatter list.

## 8. Errors and exit codes

- `0` — success.
- `1` — the command ran and the answer is failure (a verification failed, a package is missing).
- `2` — the command could not run as asked (bad flag, unreadable input, unknown subcommand).

An error message names what failed, what was expected, and what the user can do. It goes to stderr. Technical
errors are rewritten at the boundary rather than surfaced raw; a Go error string is a diagnostic, not a
message.

## 9. Interactivity and TTY

- Prompt only when stdin is a TTY. A non-interactive invocation that would prompt fails instead, naming the
  flag that would have supplied the answer.
- Color and progress indicators only when stderr is a TTY, and never in the result stream.
- `--silent` suppresses narration. It never suppresses the result, and never changes the exit code.

## 10. Configuration precedence

Highest to lowest: **flags**, then environment variables, then project configuration, then user, then system.
This document owns only the first rung; [`configuration.md`](configuration.md) owns the rest and is
authoritative where the two meet.

A flag always wins. A command must not read configuration in a way that overrides an explicitly passed flag,
including when the flag's value equals its default.

## 11. Help and generated documentation

Help text is the specification a user reads. It states what the command does, what its flags mean, and what
its output is — including, for a multi-artifact command, the names of the artifacts it writes.

`docs/cli/**` is generated from the command tree by `devlore-docs`. **The generated files are never
hand-edited.** A wrong word in the docs is a wrong word in the flag description, and it is fixed there —
which is exactly how "Promise bundle path" reached three published pages
([#739](https://github.com/NobleFactor/devlore-cli/issues/739)).

## 12. Stability

Greenfield, per the repository's governing principle. There are no released CLI contracts to preserve.

**A flag retires by deletion.** It is not aliased, not hidden, not accepted-with-a-warning. An alias is the
backward-compatibility shim the governing principle forbids, and it doubles the surface every future change
must consider.

## 13. Conformance and enforcement

Each rule below is greppable, and each has a test. These are the reason the document is worth writing.

| # | Invariant | Enforced by |
| --- | --- | --- |
| 1 | No command package writes to `os.Stdout` directly | a test over `cmd/**` |
| 2 | No command registers `--output`, `--store`, or `--json` itself | a test over cobra flag registration |
| 3 | All four in-scope roots register the full common set | a test over the command tree |
| 4 | `--store` relocates both subdirectories and the run index together | a store round-trip test |
| 5 | Narration is absent from stdout under every format | a test capturing both streams |
| 6 | Generated `docs/cli/**` matches the flag descriptions | the codegen determinism gate |

Invariants 1 and 2 are the ones that prevent regression, because both are mechanical and both are red today.

## 14. Per-app conformance

Current state, measured 2026-08-28. One command out of forty-six uses the convention, and no root
registers the common set.

| App | Commands | Conforming | Deviations |
| --- | --- | --- | --- |
| `lore` | 19 | 1 (`inspect`) | `bundle`, `onboard`, `list` hand-roll flags; 13 `fmt.Print` calls |
| `writ` | 13 | 0 | `--json` booleans on `status` and `verify`; 7 direct `os.Stdout` writes |
| `star` | 9 | 0 | a **second `cli` package** of its own -- see below |
| `devlore-docs` | 3 | 0 | — |
| `devlore-test` | 2 | 0 | `--output stream=dest` routing, `--receipt-format`, inverted artifacts |

**`star` does not lack the convention -- it has a second copy of it.** `cmd/star/cli` duplicates eighteen
exported names from `cmd/internal/cli`, including all ten exit codes and `AddOutputFlags`, and at 387 lines
against 196 the copy has grown rather than gone stale. The two now disagree: one binds `--filter`, `--jq`,
`--output`/`-o`, `--store`, and `--template` on `PersistentFlags`, the other binds `--format` and `--filter`
on `Flags`. A program cannot share a common set while binding a different one from a different package
([#743](https://github.com/NobleFactor/devlore-cli/issues/743)).

Its `renderTable` is the exception worth keeping: it is the suite's only working table renderer and the
candidate to promote into `pkg/result` in Phase 4, rather than a thing to delete.

**CLI code also lives outside `cmd/`.** Every package under the repository-root `internal/` is imported only
by `cmd/`, and `internal/console` is a Bubble Tea terminal UI. Root `internal/` is importable by the whole
module, so nothing prevents a `pkg/` package from importing CLI presentation
([#742](https://github.com/NobleFactor/devlore-cli/issues/742)).

**Adoption has gone backwards.** [`extract-output-package.md`](../plans/extract-output-package.md) recorded
`AddOutputFlags` as used at two call sites — `lore inspect` and `writ snapshot`. `writ snapshot` no longer
exists as a command, and nothing recorded that the convention lost a user with it. That is invariant 3's
justification: adoption that is not enforced decays silently.

No deviation is sanctioned. Every row above is work, tracked by the plan in
[`cli-output-conventions.md`](../plans/cli-output-conventions.md).


## Design decisions

**Settled 2026-08-28** (worked interactively, against `az`, `docker`, and `kubectl` as prior art):

1. **`--output` / `-o` selects the rendering** — not `--format`. Five tools were examined:

   | Tool | Rendering flag | Short | Values | "No output" value |
   | --- | --- | --- | --- | --- |
   | `aws` | `--output` | — | json, off, table, text, yaml, yaml-stream | `off` |
   | `az` | `--output` | `-o` | json, jsonc, none, table, tsv, yaml, yamlc | `none` |
   | `docker` | `--format` | — | json, or a Go template | — |
   | `gcloud` | `--format` | — | csv, json, list, none, table, text, value, yaml, + 7 more | `none` |
   | `kubectl` | `--output` | `-o` | custom-columns, go-template, json, jsonpath, name, wide, yaml | — |

   **The split is 3–2, not a rout.** `--output` has `aws`, `az`, and `kubectl`; `--format` has `docker` and
   `gcloud`. `-o` is narrower still — only `az` and `kubectl`.

   **The case for `--output`:** the plurality, and the two tools whose stdout-is-JSON model this suite copied
   most directly (`aws`, `az`) both use it. A user who types `-o json` by reflex is served.

   **The case against, recorded because it is strong:** `--format` is the honest word — it names the
   rendering, not the stream. `gcloud` is this suite's closest structural analogue, pairing `--format` with
   `--filter` and projections exactly as we do, and it has no `--output` at all.
   [`cmd/internal/cli/output.go`](../../cmd/internal/cli/output.go) already binds `--format`, so `--output` is
   churn against a near-tie. And the short-form argument is symmetric: binding `-o` strands `gcloud` and
   `docker` users typing `--format`, just as not binding it strands `az` and `kubectl` users typing `-o`.

   **Cost, accepted:** "output" names the stream, not the rendering. Docker's template case is still served;
   it hangs off `--output template` with `--template`.

2. **`--store` names the execution store's root**, and no `--output <file>` exists. An earlier draft of this
   document used `--output` for a *destination*, which would have meant the opposite of what `az` and
   `kubectl` users expect. Naming the store for what it is freed `--output` for decision 1. A result reaches
   a file by shell redirection, because stdout already is one.

3. **`none` joins the formatter set** — a rendering that suppresses stdout while errors still reach stderr,
   for CI that only checks an exit code. Three of the five ship one (`aws` spells it `off`; `az` and `gcloud`
   spell it `none`), and the majority spelling wins. It is a *value*, not a `--quiet` flag, for the same
   reason `--json` is not a flag: a rendering belongs in the rendering flag.

4. **Supersedes the 2026-08-20 devlore-test routing ruling.** That ruling said "results are files, narration
   is stderr, and stdout stays clean", and `devlore-test` routed all three of its payloads to files named for
   the script. It predates the three-stream split and treats one category where there are two: the summary is
   a **result** and belongs on stdout; the definition and trace are **documents** and belong in the store.
   `TestCLI_DefaultsToArtifactFiles` pinned the old behavior and is rewritten, not preserved.

   This is not a convention a program may opt out of. `aws`, `az`, `docker`, and `gcloud` differ on flag
   names, but not on this: the result goes to stdout as machine-readable data, narration goes to stderr as
   human-readable text. Every deviation in this repository is a defect, tracked in §14.

5. **Rejected: `--artifacts`, `--document-dir`, `--documents`.** Each was a new word for a concept the code
   already names. [`cmd/internal/cli/store.go`](../../cmd/internal/cli/store.go) has called it the execution
   store since it was written, and `writ secret`'s help already says so to users. A fifth synonym for one
   concept is how `graph` and `receipt` came to be inverted in `devlore-test`
   ([#738](https://github.com/NobleFactor/devlore-cli/issues/738)).

## 15. Divergences from clig.dev

**Machine-readable output.** clig.dev recommends a `--json` flag; this suite uses `--output json`. A boolean
cannot express yaml, csv, or template. Five formats behind five booleans is five flags and an ambiguity the
moment two are passed.

**Plain output.** clig.dev recommends `--plain`; here that is `--output csv` or `--output template`, by the
same argument. Plain is a rendering, so it is a format value.

**TTY-adaptive output.** clig.dev encourages adapting output to a terminal. This document **rejects** that for
the result stream: a pipeline whose data changes when observed is unreproducible. Narration adapts to a TTY
(§9); results never do.

**A third stream.** clig.dev describes two streams. This suite has three, because a workflow engine produces
durable artifacts that are neither the answer to a question nor progress narration. Documents go to the store
(§6), and no clig.dev guidance is contradicted by their existence — it simply does not cover them.

Everything else follows clig.dev, and where this document is silent, clig.dev is the answer.
