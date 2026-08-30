# Command Line Interface — One Convention, Every App — Status

**Document:** [10-command-line-interface.md](10-command-line-interface.md)
**Epic:** [#740](https://github.com/NobleFactor/devlore-cli/issues/740)
**Plan:** [cli-output-conventions.md](../plans/cli-output-conventions.md)
**State:** Design draft (2026-08-28), written after measuring the suite against `aws`, `az`, `docker`,
`gcloud`, and `kubectl`. The mechanism exists; adoption is one command in forty-six. No implementation.

## Completion

- [x] **The rule stated in one line** — a result goes to stdout, narration to stderr, documents to the
  execution store; one shared flag set decides how each is rendered and where the store lives.
- [x] Scope settled: `devlore-test`, `lore`, `star`, and `writ`, each registering the full common set on
  its root as persistent flags -- every command of all four accepts every flag.
- [x] The reserved flag set settled: `--filter`, `--jq`, `--output` / `-o`, `--store`, with renderings
  `csv`, `json`, `none`, `table`, `template=<body>`, `tsv`, `value`, `yaml`.
- [x] The pipeline drawn: two stages, one flag each; a format needing an argument carries it as
  `NAME=ARGUMENT` rather than in a sidecar flag.
- [x] Prior art surveyed and recorded as a table, not an assertion — the `--output` versus `--format` split is
  3–2, and the case against the chosen name is written into the decision rather than omitted.
- [x] The current cost enumerated against the code: 1 of 46 commands calls `AddOutputFlags`; 12 hand-rolled
  output flags; 13 `fmt.Print` calls in `lore`; 7 direct `os.Stdout` writes in `writ`; `devlore-test`'s three
  streams wired to the wrong artifacts.
- [x] Adoption measured as *decaying*, not merely lagging: `extract-output-package.md` recorded two call sites
  in March; `writ snapshot` was removed and took one with it, unrecorded.
- [ ] `--store` implemented — `SinkOptions` gains a root, and `GraphsDir` / `TracesDir` resolve under it
  together, keeping checksum keying and the run index intact.
- [ ] `--format` renamed to `--output` / `-o`; the `none` renderer added to `pkg/result`.
- [ ] `devlore-test` brought into agreement, which is where
  [#738](https://github.com/NobleFactor/devlore-cli/issues/738) is repaired rather than patched.
- [ ] `writ` brought into agreement: `--json` retired, direct stdout writes routed through the sink.
- [ ] `lore`'s remaining commands brought into agreement; the `fmt.Print` calls triaged.
- [ ] The enforcement tests written — no direct `os.Stdout` write from a command package, and no command
  registering its own output flag. Both are red today.

## Document discrepancies

- **`extract-output-package.md` is marked complete while describing absent code.** Its stated target was an
  `internal/output` package with a single `Render(w, data, options)`. No such package exists; the code went to
  `pkg/result` + `pkg/sink` with `BuildPipeline`. Correcting it is a Phase 5 task.
- **The generated CLI reference carries corrupted flag help.** Three pages under `docs/cli/lore/` read
  "Promise bundle path" and similar, from an `Output` → `Promise` replace that caught flag descriptions
  ([#739](https://github.com/NobleFactor/devlore-cli/issues/739)). The generated files are not hand-edited;
  the fix is in the flag strings.

## Outstanding work

- Whether any command needs an exception to the json-always default. `lore list` defaults to `table` today.
- `jq` versus the query languages the prior art uses — `aws` has JMESPath `--query`, `gcloud` has projections.
  Ours is defensible and more widely known, but the divergence is not yet written into §15.
