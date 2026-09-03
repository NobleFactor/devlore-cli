# Command Line Interface — One Convention, Every App — Status

**Document:** [10-command-line-interface.md](10-command-line-interface.md)
**Epic:** [#740](https://github.com/NobleFactor/devlore-cli/issues/740)
**Plan:** [740-cli-output-conventions.md](../plans/feature/740-cli-output-conventions.md)
**State:** Specified 2026-08-28 against `aws`, `az`, `docker`, `gcloud`, and `kubectl`; the formatter layer
and three of four programs landed by 2026-09-01. `devlore-test`, `lore` and `writ` register the common set;
`star` does not yet ([#743](https://github.com/NobleFactor/devlore-cli/issues/743), phase 3).

## Completion

- [x] **The rule stated in one line** — a result goes to stdout, narration to stderr, documents to the
  execution store; one shared flag set decides how each is rendered and where the store lives.
- [x] Scope settled: `devlore-test`, `lore`, `star`, and `writ`, each registering the full common set on
  its root as persistent flags -- every command of all four accepts every flag.
- [x] The reserved flag set settled: `--filter`, `--jq`, `--output` / `-o`, `--store`, with renderings
  `csv`, `json`, `list`, `none`, `table`, `template=<body>`, `value`, `yaml`. A `tsv` rendering was carried until
  2026-08-30 and dropped: quoting cannot rescue `cut` or `awk`, which have no quote awareness, so a quoted
  tab format served neither the shell nor the parser (§8).
- [x] The pipeline drawn: two stages, one flag each; a format needing an argument carries it as
  `NAME=ARGUMENT` rather than in a sidecar flag.
- [x] Prior art surveyed and recorded as a table, not an assertion — the `--output` versus `--format` split is
  3–2, and the case against the chosen name is written into the decision rather than omitted.
- [x] The current cost enumerated against the code: 1 of 46 commands calls `AddOutputFlags`; 12 hand-rolled
  output flags; 13 `fmt.Print` calls in `lore`; `devlore-test`'s three streams wired to the wrong artifacts.
  The `writ` figure was first recorded as "7 direct `os.Stdout` writes", counted by grepping that literal --
  which misses every `fmt.Print`, and those reach stdout just the same. The real figure is **30**.
- [x] Adoption measured as *decaying*, not merely lagging: `extract-output-package.md` recorded two call sites
  in March; `writ snapshot` was removed and took one with it, unrecorded.
- [x] `--store` implemented — `SinkOptions` gains a root, and `GraphsDir` / `TracesDir` resolve under it
  together, keeping checksum keying and the run index intact. It accepts a relative path: [OpenTree] demands
  an absolute one, so [SetStoreRoot] absolutizes at the seam. Found by running the binary, not by the tests,
  every one of which passed `t.TempDir()`.
- [x] `--format` renamed to `--output` / `-o`; the `none` renderer added to `pkg/result`.
- [x] `devlore-test` brought into agreement, which is where
  [#738](https://github.com/NobleFactor/devlore-cli/issues/738) is repaired rather than patched.
- [x] `writ`'s **flags** brought into agreement: the boolean `--json` retired by deletion on `status` and
  `verify`, `verify.Execute` returning `[]Report` for the command to emit, and the common set registered on
  the root.
- [x] The formatting rules written down and the code judged against them: the two stages, S1-S8, the
  per-shape matrix, and the divergences from PowerShell. Stage 1 is real -- `Pipeline.Emit` normalizes, so
  every rendering names a field by its `json:` tag, which is the name the Starlark surface shows.
- [x] `writ` consumes the set it registers. Measured 2026-08-30 as one of eight formats working, the value
  never validated, and `--store` read nowhere at all while `readback` folded runs from the default store --
  a flag registered on a root that no leaf consumes being worse than an absent one. Both were filed and
  fixed in PR #747: `--output` is validated in a `PersistentPreRunE`
  ([#754](https://github.com/NobleFactor/devlore-cli/issues/754)), and `--store` resolves the store root
  with a `PersistentPostRunE` restoring it ([#753](https://github.com/NobleFactor/devlore-cli/issues/753)).
  **Corrected 2026-09-01:** this box stayed unticked after both issues closed, so the document reported
  landed work as outstanding. That is the drift the revised process exists to prevent, and it is recorded
  here rather than quietly ticked.
- [x] `writ`'s **renderings** brought into agreement: all 30 stdout call sites, under #774 on 2026-09-01. The
  22 `fmt.Print` in `reconcile/report.go` are deleted (phase 2: the report is the result). The four dry-run
  `SerializeGraphs` dumps are returned as the plan and rendered by `-o` (phase 3). `migrate`'s own
  `--format` is retired and its session emits through the command's pipeline (phase 3). Two in `verify`
  went earlier with `presentReport`.
- [x] `lore`'s remaining commands brought into agreement, under #775 on 2026-09-01: the root registers the
  set; `runSearch` renders through the shared table, which is where #741's byte-count cut went away;
  `bundle` and `onboard` take their destination as a positional operand and `onboard`'s dead `--format` is
  gone; `onboard` emits its result; the thirteen `fmt.Print` are four rows of a result and nine lines of
  narration. Zero stdout writes remain.
- [x] `star` registers the common set and `cmd/star/cli` is deleted
  ([#743](https://github.com/NobleFactor/devlore-cli/issues/743)). The deletion landed in phase 2 -- the copy
  duplicated eighteen exported names and nothing imported it -- and the root moved onto `NewRootCmd` in
  phase 3, taking the three Starlark extension commands that bound `--output` and `--format` with it.
- [x] The shared root's commands are one set on the four programs with a program's additions attached
  beneath, `man` is the one route to man pages, and no usage text follows any error -- ruled 2026-09-02
  (§2, §9, §12; decisions 7-9). Landed with #743 phase 3, where `star` was the last program to join.
- [ ] The enforcement tests written — no direct `os.Stdout` write from a command package, and no command
  registering its own output flag. Both are red today.

## Document discrepancies

- ~~`extract-output-package.md` is marked complete while describing absent code.~~ **Corrected 2026-08-30**
  with a note at the head of that plan: no `internal/output` package exists, the code went to `pkg/result` +
  `pkg/sink`, and the plan is retained as history rather than as a map of where things live.
- ~~The generated CLI reference carries corrupted flag help.~~ **Fixed 2026-08-30.** Four sites, not three:
  the fourth was `lore inspect`'s long description, which read "Promise is JSON by default" in prose rather
  than in a flag string and so escaped a grep aimed at flag descriptions. Source corrected
  ([#739](https://github.com/NobleFactor/devlore-cli/issues/739)). `docs/cli` is gitignored here but
  published: `docs-publish.yaml` regenerates it on every push to `develop` and auto-merges it into
  `devlore.noblefactor.com`, so the wrong strings were live on the public site.

## Outstanding work

- Whether any command needs an exception to the json-always default. `lore list` defaults to `table` today.
- Whether `value` should require a projection, as `gcloud` requires one for `csv` and `value`. Applied to a
  whole nested struct it prints every field, pointer addresses included. §7 states the expectation; nothing
  enforces it.
