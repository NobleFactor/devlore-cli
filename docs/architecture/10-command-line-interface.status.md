# Command Line Interface — One Convention, Every App — Status

**Document:** [10-command-line-interface.md](10-command-line-interface.md)
**Epic:** [#740](https://github.com/NobleFactor/devlore-cli/issues/740)
**Plan:** [cli-output-conventions.md](../plans/cli-output-conventions.md)
**State:** Specified 2026-08-28 against `aws`, `az`, `docker`, `gcloud`, and `kubectl`; the formatter layer
and two of four programs landed 2026-08-30. `devlore-test` and `writ` register the common set; `lore` and
`star` do not yet.

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
