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
[`cmd/internal/cli/output.go`](../../cmd/internal/cli/output.go) today binds `--filter`, `--format`, and
`--jq`, and composes a `result.Pipeline` of filter, formatter, and sink. What it has never had is a
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
| `--output` | `-o` | string | How the result is rendered; `NAME` or `NAME=ARGUMENT`. §7, §8 |
| `--store` | | string | The execution store's root. §6 |

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

`--output` / `-o` selects the rendering. Its value is a format name, or `NAME=ARGUMENT` for a format that
needs one (§8). The set, alphabetically: `csv`, `json`, `list`, `none`, `table`, `template=<body>`, `value`,
`yaml`.

Reshaping is not a rendering. Selecting fields, mapping, and interpolating happen in the filter stage
(`--filter`, `--jq`), which composes with every format. `aws`, `az`, and `gcloud` all take this shape: a
query language reshapes, a format renders, and neither borrows the other's job.

**JSON is the default**, following `az`. A command's result is data first, and the common case — a script or a
pipe — should need no flag at all.

**A command does not switch format based on whether stdout is a TTY.** A pipeline that behaves differently
when observed is unreproducible. A human wanting a table asks for one: `-o table`.

**Adding a formatter** is a change to `pkg/result`, never to a command. A command needing a shape the set
does not have reshapes with `--jq` and renders with `value`.

**`value` expects a projection.** It renders whatever it is handed, so a whole nested structure comes out as
one line with its nested members as compact JSON. That is readable, and it is not what a pipe wants. It is
the same expectation `gcloud` states by requiring a projection for its `csv` and `value` formats. The
intended use is `--jq` first:

```
writ status --jq '.entries[] | "\(.target) is \(.state)"' -o value
```

**One table formatter, no exceptions.** `table` is a general rendering and belongs in `pkg/result` like the
others. No command owns its own. The current hand-rolled table in `lore`'s `runSearch`
(`cmd/lore/lore/commands.go:525-556`) is not an argument for a second one: it writes through `fmt.Printf`
rather than the sink, hard-codes column widths, measures truncation in bytes, and folds a boolean into a
name column as a `*` suffix. A shared formatter fixes all four, and `installed` stays a field that
`--output json` can emit.

A **domain** rendering is a different question. `lore list` registers `--format manifest`, which means
something only to lore. Under this convention that is a lore-specific flag outside the common set -- never a
value added to the shared formatter list.

## 8. The pipeline: two stages, one flag each

Everything a command emits as a result passes through two stages, and each stage is owned by exactly one
flag:

```
                   FILTER STAGE                    FORMAT STAGE
                (value ──► value)               (value ──► bytes)

              ┌──────────┐  ┌────────┐        ┌──────────────────────┐
 result ─────►│ --filter │─►│  --jq  │───────►│      --output        │──► sink ──► stdout
  value       │ field=v  │  │  gojq  │        │                      │
              └──────────┘  └────────┘        │  csv                 │
                                              │  json                │
                                              │  list                │
               reshape: select, map,          │  none                │
               project, interpolate           │  table               │
               composable, any order          │  value               │
                                              │  yaml                │
                                              │  template=<body>     │
                                              └──────────────────────┘
                                                 pick exactly one
```

**Reshaping is not rendering.** Choosing which fields appear, mapping over a list, and building a string are
the filter stage's work, and they compose with every format. `aws`, `az`, and `gcloud` all take this shape --
a query language reshapes, a format renders, and neither borrows the other's job.

### From a Go value to a presentation

The pipeline has two stages, and the first is total: **every result becomes JSON before anything renders
it.** JSON is the native format, and `table`, `csv`, `list`, and `value` are presentations of that JSON --
not of the Go value behind it.

```
  Go value            JSON                    presentation
     |                  |                          |
     v                  v                          v
  +--------+       +---------+   filter      +-----------+
  | struct |------>| object  |-->--filter--->|  --output |--> stdout
  | slice  | stage | array   |   --jq        |           |
  | map    |   1   | scalar  |               |  json     |  serialize
  | scalar |       | null    |               |  yaml     |  serialize
  +--------+       +---------+               |  table    |  }
                                             |  csv      |  } one key derivation,
                        stage 2 ------------>|  list     |  } four presentations
                                             |  value    |  }
                                             |  template |  applied to the value
                                             |  none     |  discards
                                             +-----------+
```

Stage 1 runs once, at the head of [Pipeline.Emit], so the filter and the formatter see the same data. A
formatter called directly renders what it is handed; a *command* never does that, which is what makes the
rule below true of everything a user sees.

Normalizing first is what keeps the presenters honest, and the cost of skipping it is measurable -- see
"What PowerShell gets wrong, and why" below. It also decides field NAMES: a struct field carries its
`json:` tag through every rendering, and those are the names the Starlark surface shows a customer. Before
stage 1 ran here only the jq filter normalized, so `-o list` named a field `UnitCount` while
`--jq . -o list` named the same field `unit_count`.

**Numbers stay `json.Number`.** Decoding to `float64` rounds any integer past 2^53 -- the defect
[#712](https://github.com/NobleFactor/devlore-cli/issues/712) records against the document codec -- so a
presenter renders the literal digits it was given. `gojq` needs int64 and float64 and gets them inside the
jq filter, where the conversion is gojq's requirement rather than this pipeline's.

#### The shapes a presenter must answer for

| | Shape | Example |
| --- | --- | --- |
| S1 | scalar | `"active"`, `3`, `true`, `null` |
| S2 | array of scalars | `["a","b"]` |
| S3 | flat object | `{"name":"x","state":"active"}` |
| S4 | nested object | `{"name":"x","health":{"runs":3}}` |
| S5 | array of flat objects | `[{...},{...}]` -- **the table** |
| S6 | array of objects, differing keys | union, with holes |
| S7 | array of arrays | positional rows |
| S8 | empty array, or null | |

`json`, `yaml`, and `none` are shape-independent: the first two serialize anything, the third discards.
`template=BODY` hands the value to a template. The rules below govern the four that lay data out.

#### Keys are derived once; presentations differ

One derivation serves all four. A `csv:"name"` tag renames a field, `-` omits it, and a [HasHeaders]
implementation overrides inference entirely. Absent those: a struct contributes its exported fields in
declaration order, and a map contributes its keys sorted alphabetically.

What differs is whether records share a schema.

- `table`, `csv`, and `value` derive **one** column set -- the union of keys across every record -- and
  every record fills it, leaving holes where a key is absent.
- `list` gives each record **its own** keys. That is what makes it right for S6.

| Shape | `table` / `csv` / `value` | `list` |
| --- | --- | --- |
| S1 | one row, one column, no header | the value alone, no key |
| S2 | one column, one row per element | one value per line |
| S3 | one row | `key : value` per field |
| S4 | one row; nested values as compact JSON | as S3, nested values as compact JSON |
| S5 | one row per element; columns = union | one block per element, blank line between |
| S6 | as S5; absent keys render empty | as S5 -- each block shows only its own keys |
| S7 | one row per inner array, positional, no header | one block per inner array, values unkeyed |
| S8 | nothing, exit 0 | nothing, exit 0 |

**A non-scalar cell renders as compact JSON.** `{"runs":3}`, `["a","b","c"]`, at any depth, never
truncated. Three reasons over the alternatives:

1. Columns stay shallow and predictable, which is the property that makes `table` and `csv` usable at all.
   Flattening to `health.runs` makes column count a function of data depth.
2. The cell holds the native format, so it pipes back into `jq`.
3. It loses nothing. Refusing a nested value would be defensible, but it makes a command author's shaping
   mistake into a user's error.

Flattening remains available where it belongs -- in the filter stage, chosen explicitly:
`--jq '{name, runs: .health.runs}'`.

#### What PowerShell gets wrong, and why

PowerShell has the richest formatter set in this survey, and its inline rendering of a nested value is the
choice adopted above. Three of its behaviors are rejected, and all three have one cause: **it formats .NET
objects by their properties, without normalizing first.** Measured against pwsh 7.5.4.

| Behavior | Output | Cause |
| --- | --- | --- |
| Depth truncates at the third level | `@{c=}` -- the value of `d` silently gone | the property walk stops |
| Type names leak | `System.Collections.Hashtable`, where an object gives `@{k=v}` | different .NET types |
| Arrays are reflected over | `Length LongLength Rank SyncRoot IsReadOnly ...` | an array is an object with properties |

Stage 1 makes all three impossible here. By the time a presenter sees the value it is JSON: no
Hashtable-versus-object distinction exists, an array is an array, and nothing has properties to reflect
over. That is the argument for normalizing before presenting, stated as a consequence rather than a taste.

**Rejected: automatic table/list switching.** PowerShell renders four properties or fewer as a table and
five or more as a list. The same command changes shape because someone added a field. §10 rejects TTY
adaptation because a pipeline that behaves differently when observed is unreproducible; switching on
property count is that defect with a different trigger. Neither `aws`, `gcloud`, nor `kubectl` does it.

**Divergence: S6 unions keys rather than sampling the first record.** `Format-Table` takes its columns from
the first object and blanks every key the later ones introduce:

```
$het = @(
  [pscustomobject]@{ name="x"; state="active" }
  [pscustomobject]@{ name="y"; runs=3; findings=@("a","b") }
  [pscustomobject]@{ kind="package"; action="install" }
)
$het | Format-Table

name state
---- -----
x    active
y

        <- runs, findings, kind, and action are gone, and nothing says so
```

A sparse table is a worse presentation than a dense one. Silently dropping columns is a worse *answer*. We
take the sparse table, and `list` exists so heterogeneity has a rendering that is not sparse.

#### `list`: one field per line

`list` is the rendering for a result that is wide, heterogeneous, or both -- where `table` is unreadable and
`json` is punctuation a reader has to see past.

```
name  : x
state : active

name     : y
runs     : 3
findings : ["a","b"]
```

Keys are padded within a record, not across the stream, so a heterogeneous stream does not pay for its
widest key everywhere. Records are separated by a blank line. The separator is `` : `` with the colon
aligned, deliberately not `key: value`, which would read as YAML and invite the belief that it is -- `-o
yaml` is one flag away and means something different.

`aws` has no equivalent; `gcloud` ships `list` and `flattened` as separate formats. One suffices here
because the compact-JSON cell rule already handles the nesting that `flattened` exists to spread out.

### A format that needs an argument carries it

`--output template=<body>` is one flag with one value. The alternative -- a sidecar `--template` flag -- gives
the format stage two inputs and needs a rule about how they interact:

```
                                              ┌──────────────────────┐
                                     ────────►│      --output        │
                                              │                      │──► sink
                                     ────────►│      --template      │
                                              └──────────────────────┘
                                                mutually exclusive --
                                                a rule to document,
                                                enforce, and get wrong
```

The `NAME=ARGUMENT` form deletes that rule rather than stating it: conflict is impossible by construction, not
prevented by validation. `kubectl` ships exactly this -- `-o go-template=...`, `-o jsonpath=...`,
`-o custom-columns=...` -- and gcloud's `NAME[ATTRIBUTES](PROJECTION)` is the same idea with richer syntax.
A sidecar flag is docker's shape, and docker has it only because `--format` *is* the template.

Parsing splits on the **first** `=`, so an argument containing `=` is unaffected.

**Presets stay named.** `csv` and `value` are two names, not one name plus an argument, because two values a
user can guess beat one value with an argument they must look up. gcloud ships the same two names for the
same reason.

**The delimited pair splits by consumer, not by separator.** `csv` quotes, so a parser round-trips it;
`value` does not, so a shell reads exactly what was composed. A quoted tab format -- `tsv` -- sat between
them until 2026-08-30 and served neither. `cut` and `awk -F'\t'` have no quote awareness, so quoting a field
that contains a tab does not rescue them: the row still splits and they get `"a` and `b"` rather than `a` and
`b`. Quoting helps only a caller running a real parser, and that caller is better served by `csv`, which
every standard library reads. gcloud and aws each ship a raw delimited format (`value`, `text`) and no
`tsv`, and the naming is the reason: `tsv` promises round-tripping, and a format that does not quote cannot
keep that promise.

## 9. Errors and exit codes

- `0` — success.
- `1` — the command ran and the answer is failure (a verification failed, a package is missing).
- `2` — the command could not run as asked (bad flag, unreadable input, unknown subcommand).

An error message names what failed, what was expected, and what the user can do. It goes to stderr. Technical
errors are rewritten at the boundary rather than surfaced raw; a Go error string is a diagnostic, not a
message.

## 10. Interactivity and TTY

- Prompt only when stdin is a TTY. A non-interactive invocation that would prompt fails instead, naming the
  flag that would have supplied the answer.
- Color and progress indicators only when stderr is a TTY, and never in the result stream.
- `--silent` suppresses narration. It never suppresses the result, and never changes the exit code.

## 11. Configuration precedence

Highest to lowest: **flags**, then environment variables, then project configuration, then user, then system.
This document owns only the first rung; [`configuration.md`](configuration.md) owns the rest and is
authoritative where the two meet.

A flag always wins. A command must not read configuration in a way that overrides an explicitly passed flag,
including when the flag's value equals its default.

## 12. Help and generated documentation

Help text is the specification a user reads. It states what the command does, what its flags mean, and what
its output is — including, for a multi-artifact command, the names of the artifacts it writes.

`docs/cli/**` is generated from the command tree by `devlore-docs`. It is **gitignored in this repository and
published from it**: `.github/workflows/docs-publish.yaml` runs `make docs` on every push to `develop`,
`main`, or `release/*`, copies `docs/cli/` and `docs/guides/` into `NobleFactor/devlore.noblefactor.com`, and
**opens and merges** the site PR unattended.

Two consequences follow, and the second is the one that bites.

A wrong word in the reference is a wrong word in a command's help string, and it is fixed at the source. The
generated files are never hand-edited -- there is nowhere to hand-edit them, since this repository does not
keep them.

**A help string reaches the public site on the next merge to `develop`, with no human in the path.** That is
how "Promise bundle path" was published
([#739](https://github.com/NobleFactor/devlore-cli/issues/739)). Being gitignored makes it *less* visible in
review, not less published: a flag description gets no diff here and no approval there.

## 13. Stability

Greenfield, per the repository's governing principle. There are no released CLI contracts to preserve.

**A flag retires by deletion.** It is not aliased, not hidden, not accepted-with-a-warning. An alias is the
backward-compatibility shim the governing principle forbids, and it doubles the surface every future change
must consider.

## 14. Conformance and enforcement

Each rule below is greppable, and each has a test. These are the reason the document is worth writing.

| # | Invariant | Enforced by |
| --- | --- | --- |
| 1 | No command package writes to `os.Stdout` directly | a test over `cmd/**` |
| 2 | No command registers `--output`, `--store`, or `--json` itself | a test over cobra flag registration |
| 3 | All four in-scope roots register the full common set | a test over the command tree |
| 4 | `--store` relocates both subdirectories and the run index together | a store round-trip test |
| 5 | Narration is absent from stdout under every format | a test capturing both streams |
| 6 | Help strings read as published prose; they ship unreviewed | `make docs` and read it, in the flag-changing work |

Invariants 1 and 2 are the ones that prevent regression, because both are mechanical and both are red today.

## 15. Per-app conformance

Current state, 2026-08-30. Two of the four in-scope programs register the common set on their root.

| App | Root registers the set | Remaining deviations |
| --- | --- | --- |
| `devlore-test` | **yes** | none |
| `writ` | **yes** | `status`'s 22 `fmt.Print` calls; four dry-run dumps; `migrate`'s `--format` |
| `lore` | no | hand-rolled flags on three commands; 13 `fmt.Print`; `runSearch` (#741) |
| `star` | no | a **second `cli` package** of its own -- see below |
| `devlore-docs` | not in scope | — |

`writ status` is bridged rather than converted: `cfg.JSONOutput` reads `outputOptions.Format == "json"` while
its report still renders itself. The consequence is worth stating -- `writ status -o yaml` silently produces
the human report, not yaml -- and it holds only until that report goes through the pipeline.

**`star` does not lack the convention -- it has a second copy of it.** `cmd/star/cli` duplicates eighteen
exported names from `cmd/internal/cli`, including all ten exit codes and `AddOutputFlags`, and at 387 lines
against 196 the copy has grown rather than gone stale. The two now disagree: one binds `--filter`, `--jq`,
`--output`/`-o`, and `--store` on `PersistentFlags`, the other binds `--format` and `--filter`
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

   **Cost, accepted:** "output" names the stream, not the rendering. That precision is traded for the flag
   users already type.

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
   human-readable text. Every deviation in this repository is a defect, tracked in §15.

5. **A format needing an argument carries it in the value: `NAME=ARGUMENT`.** The alternative, a sidecar
   flag, gives the format stage two inputs and a mutual-exclusion rule to enforce. `--output template=<body>`
   makes the conflict impossible by construction instead. `kubectl` ships this exact form
   (`-o go-template=`, `-o jsonpath=`, `-o custom-columns=`); gcloud's `NAME[ATTRIBUTES](PROJECTION)` is the
   same idea. Parsing splits on the first `=`.

   Presets stay named rather than becoming arguments: `csv` and `value` are two values a user can guess,
   where `csv=tab` is one value with an argument to look up. gcloud ships the same two names for that
   reason.

6. **Rejected: `--artifacts`, `--document-dir`, `--documents`.** Each was a new word for a concept the code
   already names. [`cmd/internal/cli/store.go`](../../cmd/internal/cli/store.go) has called it the execution
   store since it was written, and `writ secret`'s help already says so to users. A fifth synonym for one
   concept is how `graph` and `receipt` came to be inverted in `devlore-test`
   ([#738](https://github.com/NobleFactor/devlore-cli/issues/738)).

## 16. Divergences from clig.dev

**Machine-readable output.** clig.dev recommends a `--json` flag; this suite uses `--output json`. A boolean
cannot express yaml, csv, or template. Five formats behind five booleans is five flags and an ambiguity the
moment two are passed.

**Plain output.** clig.dev recommends `--plain`; here that is `--output csv` or `--output value`, by the
same argument. Plain is a rendering, so it is a format value.

**TTY-adaptive output.** clig.dev encourages adapting output to a terminal. This document **rejects** that for
the result stream: a pipeline whose data changes when observed is unreproducible. Narration adapts to a TTY
(§10); results never do.

**A query language rather than a template engine.** clig.dev does not take a position; the prior art splits
cleanly. `aws` and `az` ship JMESPath as `--query`, `gcloud` ships projections and transforms, and none of the
three has a template engine. `kubectl` and `docker` have templates and no query language -- docker's
`--format` *is* the template. Nobody ships both, because they solve one problem.

This suite uses `gojq`, which is strictly more capable than JMESPath and more widely known than gcloud's
projection syntax. `template=<body>` exists for text layout a query cannot express, and is deliberately the
only argument-taking format.

**A third stream.** clig.dev describes two streams. This suite has three, because a workflow engine produces
durable artifacts that are neither the answer to a question nor progress narration. Documents go to the store
(§6), and no clig.dev guidance is contradicted by their existence — it simply does not cover them.

Everything else follows clig.dev, and where this document is silent, clig.dev is the answer.
