---
title: "writ's templates end in .tmpl: .template is left to other tools, and New-LocationConfig finds its envsubst template"
issue: https://github.com/NobleFactor/devlore-cli/issues/974
status: approved
created: 2026-09-29
updated: 2026-09-29
---

# Plan: writ's templates end in .tmpl

Lane 11 of #949 (amended 2026-09-29). Feature #463: writ's command surface. One pull request resolves this
issue and David-Noble-at-work/personal#237.

## Issue 974

Ruled 2026-09-29: "Let's do as you propose. .tmpl for writ templates."

writ claims every file ending in `.template` (`cmd/writ/writ/tree/node.go:52-53`). It renders the file as a
Go template and deploys a copy with the suffix stripped. `.template` is a generic word that other tools use
for their own templates. `.tmpl` is what chezmoi, the dotfile manager closest to writ, uses for exactly
writ's behavior, and it's the common suffix for Go templates. The official nginx image's `envsubst` over
`*.template` is the precedent for leaving `.template` to other tools.

The writ guide already says `.tmpl` (`docs/guides/writ/index.md:72`), and the code and
`docs/guides/writ/repositories.md:124` say `.template`. This change makes the code agree with the ruling,
and both guides with the code.

## Issue 237

personal's `New-LocationConfig` fills `templates/certificate-request.conf.template` with `envsubst`. Its
placeholders are `${VAR}`, and it has no `{{ }}`. writ renders the file into `certificate-request.conf`, a
copy, so the script never finds the name it opens. Once writ stops claiming `.template`, the file deploys as
a link under its own name, with no change in personal.

## Requirements

### Requirement 1: The suffix

- `cmd/writ/writ/tree/node.go`: the rule at `:52-53` tests `.tmpl`, and the doc comment's examples at
  `:30-32` become `foo.tmpl` and `foo.tmpl.sops`.
- `pkg/sops/decrypt.go:80-82`: `detectFormat` strips `.tmpl`, so `config.yaml.tmpl.sops` is read as YAML.
- A file still ending in `.template` is an ordinary file, and writ links it like any other.

### Requirement 2: The tests say `.tmpl`

- `cmd/writ/writ/tree/tree_test.go`: the pipeline table's `.template` rows become `.tmpl`. A new row pins
  `foo.template` → `foo.template`, `["file.link"]`, the behavior this change creates.
- `pkg/sops/decrypt_test.go`: `config.yaml.tmpl.sops`.
- The integration tests' fixture names: deploy (two), sops, readback, reconcile, upgrade and
  decommission.
- `git mv cmd/writ/testdata/personal-repo/Home/noblefactor.Unix/.config/scenario/writ.conf.template …/writ.conf.tmpl`.
  The scenario asserts the rendered `writ.conf`, which is unchanged
  (`cmd/writ/scenario_integration_test.go:485`).

### Requirement 3: The guides describe the code

- `docs/guides/writ/repositories.md:124`: "A file named `<name>.tmpl` renders …".
- `docs/guides/writ/index.md` § Templates (`:70-82`): the suffix is already `.tmpl`. Its example uses
  `{{.UserName}}` and `{{.UserEmail}}`, which writ doesn't provide, and it says the values come from
  `writ config set`. The section is rewritten to what `cmd/writ/writ/deploy/templatedata.go:75-100`
  provides: `OS`, `ARCH`, `Hostname`, `Home`, `Username`, `Segments`, and the XDG homes. It also covers
  the user's own variables, `writ.vars`. Per the configuration spec (`docs/architecture/configuration.md`
  § Resolution, § Variables), which the guide links rather than restates, a variable set in the
  configuration can also come from the environment or the command line, the command line winning. The owner, 2026-09-29: "Putting something into configuration means that
  it can also come out of the environment or the command line." Today writ reads only the file; per agent
  rule 2, the guide states the design and names that interim in one sentence, citing #975.
- `docs/guides/writ/index.md` § Secrets (`:86-94`): the section says `.age`, which is outdated and not
  supported. It now describes `.sops`, decrypted and deployed as a copy (`node.go:47-50`), including
  `.tmpl.sops`, decrypted and then rendered.

### Requirement 4: No migration

No layer holds a writ template. On 2026-09-29, the only `.template` in personal, noblefactor-ops or
devlore-cli's `Home` trees was personal's certificate request, which isn't one. devlore-cli's other
`.template` files are star's code-generation templates and the MacPorts `Portfile`, which are out of
scope because other tools read them. So nothing renames in any layer.

### Requirement 5: Converge (agent rules 1 and 5), after the merge, on DANOBLE-UD24-1

1. The writ on PATH is develop's build with this change, installed and checked by `writ --version`.
2. `writ deploy`: `~/.local/bin/templates/certificate-request.conf.template` is a link into personal.
3. `~/.local/bin/templates/certificate-request.conf`, the copy rendered from the old rule, is gone. writ
   drops it if its record allows; otherwise it's removed by hand (#960's class of leftover).
4. The deployed `New-LocationConfig`, run from a scratch directory with every option given, writes
   `ssl/certificate-request.conf` with every `${…}` filled.
5. Rule 5's dangling-link `find` is empty, and `writ reconcile` is read and reported.

Each Mac that has deployed personal carries the same rendered copy. Converging it is per machine, as noted
on personal#239.

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, before the change

### Phase 2: The change

- [ ] Requirement 1, the suffix
- [ ] Requirement 2, the tests and the fixture
- [ ] Requirement 3, the guides

### Phase 3: Verify

- [ ] `gofmt -l` over the changed Go files is empty
- [ ] The tree, sops and writ integration tests pass through the Makefile's test target, as CI runs
      them
- [ ] A live check with this branch's writ in scratch XDG directories and a scratch layer:
      `a.conf.tmpl` holding `{{ .OS }}` deploys as a rendered copy `a.conf`, and `b.conf.template`
      holding `{{ .OS }}` and `${VAR}` deploys as a link `b.conf.template`, byte for byte. The host's
      `~/.config/devlore` is diffed before and after, to prove it untouched.
- [ ] CI's gate: `make vet-all`, `make lint-all`, `./build/star lint go ./...` and
      `./build/star lint shell .`

### Phase 4: Merge and converge

- [ ] PR script written, shown, and handed over. The PR resolves #974 and David-Noble-at-work/personal#237
- [ ] After the merge and the pre-release: Requirement 5

## Files to Create/Modify

| File | Action |
| --- | --- |
| `cmd/writ/writ/tree/node.go` | Modify: the rule and its doc comment |
| `pkg/sops/decrypt.go` | Modify: `detectFormat` |
| `cmd/writ/writ/tree/tree_test.go`, `pkg/sops/decrypt_test.go` | Modify: `.tmpl`, plus the `.template` row |
| the six writ integration tests named in Requirement 2 | Modify: fixture names |
| `cmd/writ/testdata/personal-repo/Home/noblefactor.Unix/.config/scenario/writ.conf.template` | Rename to `writ.conf.tmpl` |
| `docs/guides/writ/repositories.md`, `docs/guides/writ/index.md` | Modify |

## Out of Scope

- **star's code-generation templates** (`star/extensions/com.noblefactor.devlore.Actions/templates/*.go.template`)
  and **`packaging/macports/Portfile.template`**: Go templates, but star and the release read them, not writ.
- **A warning for a `.template` file that holds `{{ }}`.** No layer has one, and a warning would fire
  on legitimate files for other tools.
- **Template variables from the environment and the command line** (#975): the guide states them, and
  the code that delivers them is #975's.

## Open Questions

- [x] **`docs/guides/writ/index.md` § Secrets (`:86-94`) says files ending in `.age` are decrypted; writ
      decrypts `.sops` (`node.go:47-50`).** Ruled 2026-09-29: "We do not support the .age extension. That
      is outdated. We use .sops. PERIOD." Corrected here, because Requirement 3 edits that file (see
      Requirement 3).
