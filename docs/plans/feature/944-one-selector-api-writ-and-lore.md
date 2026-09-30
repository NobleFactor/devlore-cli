---
title: "One selector API: writ and lore detect the host and match platform selectors through the same code, along os-release's lineage"
issue: https://github.com/NobleFactor/devlore-cli/issues/944
status: active
created: 2026-09-30
updated: 2026-09-30
---

# Plan: one selector API for writ and lore

Lane 19 of #949, alone, straight after #959 (lane 18): "we'll do lane 18 followed by lane 22. two prs and at the end
we'll be done with selectors. we'll have common selectors for writ and lore." (ruled 2026-09-30; #944 was lane 22 when
the ruling was made). It absorbs #860, the lineage chain. The rulings below also make it resolve #369, the suffix
grammar's enforcement, and #970, the segments that `writ upgrade` and `writ reconcile` don't see.

## Issue 944

writ and lore each detect the host and match platform selectors with their own code, and neither follows a
distribution's lineage. The rulings are on the issue: the chain `Unix → Linux → <ID_LIKE reversed> → <ID>`; a selector
word names one link; dots separate independent dimensions; the most specific link wins; lore's `Linux.Debian` and
`Linux.Fedora` become `Debian` and `Fedora`; detection falls back along `ID_LIKE`.

Ruled 2026-09-30, in the design talk on how selectors match:

- **Order is significant.** "order is important to understanding at a glance what I've got." A name reads project,
  then OS, then architecture, then the extra segments in their configured order.
- **The OS part is one word.** "Linux matches all linuxes. We match using common reference patterns. When I say I'm on
  Ubuntu, everyone knows I mean Ubuntu Linux. It's implied." So `common.Linux.Debian` is no longer a valid name; that
  replaces 2026-09-28's "still matches but is redundant".
- **The API** takes a list of directory names and answers which to include, and in which order: least specific to
  most specific, starting with the name that has no OS word.
- **Extras are declared in configuration,** each with its allowed values: "they must be defined in config. Otherwise, we
  have no validation and a simple misspelling gives us grief." Their values come from the command line
  (`--segment NAME=value`), the environment (`WRIT_SEGMENT_<NAME>`) or configuration, "as the user wishes as any other
  argument can". The declaration is an ordered list, because the extras' order is the grammar's:

  ```yaml
  writ:
    segments:
      - name: ROLE
        values: [desktop, server]
        value: desktop
      - name: SITE
        values: [aws, home]
  ```

- **The ranking, the attribution of words to segments, and grammar errors** are Q3, Q20 and Q19 under Decisions.

Today, at `ea5cebd2`, on this Ubuntu host (`ID=ubuntu`, `ID_LIKE=debian`), re-read by four readers on 2026-09-30:

| | `Unix` | `Linux` | `Debian` | `Ubuntu` |
| --- | --- | --- | --- | --- |
| writ | ✔ | ✔ | ✘: nothing reads `ID_LIKE` | ✔ since #959 |
| lore | ✔ | ✔ | ✔ only as `Linux.Debian`, from a hard-coded family list | ✘: no Ubuntu selector exists |

- **Two os-release readers,** writ's `cmd/writ/writ/segment/detect.go:128-155` and
  `pkg/platform/detect_linux.go:85-119`. Neither reads `ID_LIKE`. `pkg/platform` refuses any ID outside its ten
  (`defaults.go:31-42`), so Pop!_OS fails detection outright.
- **Three spellings and more:** writ's suffix `Debian`, lore's token `Linux.Debian` (`pkg/platform/token.go:21-40`),
  Starlark's raw `ubuntu` (`pkg/op/provider/platform/provider.go:47-58`), and lore's `linux/fedora` flags.
- **Matching is an unordered AND, ranked by suffix count** (`segment.go:84-121`, `tree/builder.go:238`, `:325`). Ties
  fall to the order `os.ReadDir` walks, so `common.Unix` beats `common.Ubuntu`, and on macOS `common.Unix` beats
  `common.Darwin`. By count, a two-word name beats any one-word name: with `--segment LIKE=Debian` making `Debian`
  match, `common.Linux.Debian` beats `common.Ubuntu`. A probe at `ea5cebd2` confirmed all three.
- **lore picks its native package manager by the token's prefix** (`cmd/internal/lorepackage/package.go:200-213`):
  bare `Linux` means apt, so CentOS Stream, AlmaLinux, Arch and Manjaro are labeled apt.
- **The registry has no `Linux.*` directories left.** devlore-registry's develop and main dropped them in its #81; only
  hand-written text still names the form (29 references in 9 files).
- **The owner's three layers need no rename.** Every project name in base, team and personal is valid under the ruled
  grammar. Personal's `Home/common.Debian` is the only distribution-suffixed project, and no path is shared between
  chain links that match together, so the ranking change moves no deployed file.

## Requirements

Where a requirement rests on a decision, it names it (**Q*n***).

### Requirement 1: One package detects the host, and selects and orders directory names

Its home is `pkg/selector` (**Q1**).

- **Detection has no side effects and never fails,** because writ calls it on every run. It reads `/etc/os-release`,
  falling back to `/usr/lib/os-release`: `ID`, `ID_LIKE`, `VERSION_ID` and `VARIANT_ID`, with double quotes, single
  quotes and escapes. A missing file means "Linux, distribution unknown": the chain stops at `Linux`. It takes a path or
  a reader, so every test drives it with a synthetic os-release.
- **The chain,** most general first: Linux `Unix → Linux → <ID_LIKE reversed> → <ID>`, and the other operating systems
  per **Q6**. Each link is spelled through one word table (**Q2**); a link that repeats is kept at its most specific
  position.
- **A closed vocabulary** (**Q21**): every OS word, every distribution word the table spells, the host's own chain,
  every architecture in both spellings (`arm64` and `aarch64`, `amd64` and `x86_64`), and the declared extras' values.
  A word outside it is a misspelling.
- **The grammar:** `[<project>][.<os>][.<arch>][.<extra>...]`, in that order. The caller says whether its names carry a
  project: writ's do, lore's don't (**Q24**). Words are attributed to their slots left to right, each slot taking at
  most one word (**Q20**). A name that breaks the grammar is out of order, has two OS words, or holds a word outside
  the vocabulary.
- **Selection** takes a list of directory names and returns two things: the names this machine includes, in the order
  to apply them, total and stable so no result depends on walk order: projects in the caller's order (**Q27**), and
  within a project the platform ranking (**Q3**); and every grammar error, each naming the
  directory and the rule it breaks (**Q19**). A well-formed name for another machine, `common.Darwin` on Ubuntu, is
  excluded silently.
- It replaces writ's `readDistro`, `capitalizeDistro`, `capitalizeOS` and `OSFamily`, `pkg/platform`'s `readOSRelease`,
  and `token.go`'s `Token` and `DetectToken`.

### Requirement 2: writ selects through the one package

- `segment.DetectSegments` takes OS, the chain and ARCH from the one package. DISTRO stays the ID (**Q5**), so
  templates' `.Segments.OS`, `.Segments.DISTRO` and `.Segments.ARCH` keep their meaning.
- Within each layer, the tree builder applies the directories selection returns, in its order; layers are processed
  left to right (**Q4**). Manifests contribute in the same order.
- Projects are ordered as the command line reads (**Q27**): the implicit projects first (`common`, then the one named
  after each layer repository, as writ lists them today), then the named projects left to right. Naming an implicit
  project on the command line is a command-line error, `writ deploy common` included: the bare form is `writ deploy`.
- `writ deploy` collects the grammar errors of every layer before planning, and refuses the run with all of them
  listed (**Q19**). `writ upgrade` and `writ reconcile` read the deployed inventory, not the layer trees, so they
  select no directory and have no name to judge. (Corrected 2026-09-30: this named them too.)
- `writ adopt --platform` validates `<project>.<platform>` through the grammar and against this machine (**Q13**). Its
  help, examples and doc comments say `Debian`, not `Linux.Debian`.
- The collision record and its narration carry the winning directory's rank, and name the matched directory rather
  than the file's parent (`tree/output.go:46-48`).
- The dead selector code goes (**Q14**).

### Requirement 3: The extra segments are declared, validated, and seen by every command

- **Declared in configuration.** `writ.segments` becomes the ordered list ruled on 2026-09-30: each entry a `name`, its
  `values`, and optionally this machine's `value`. It replaces the list of names at
  `cmd/internal/config/writ.go:9-11`, which nothing reads; the name-to-value object in
  `schema/devlore-config.json:241-257`, which `writ config validate` and `writ config set` check against; the map in
  `schema/defaults/writ.yaml:7-15`; and the map at `docs/architecture/10-command-line-interface.md:896-906`.
  `cmd/internal/config/config_test.go:145`, `:205` and `:237` follow.
- **Validated when the configuration loads.** Names are unique and none is a built-in (OS, DISTRO, ARCH). Values are
  unique across every segment, the OS and architecture words included (**Q20**). A `value` is one of its `values`.
- **Set as any argument is.** A value comes from `--segment NAME=value`, `WRIT_SEGMENT_<NAME>` or configuration, the
  flag beating the environment beating configuration. For an extra, `--segment` and `WRIT_SEGMENT_<NAME>` refuse a
  name the configuration doesn't declare and a value its segment doesn't declare. The built-ins take them without a
  declaration (**Q22**).
- **Seen by every command that selects:** `writ deploy`, `writ upgrade`, `writ reconcile` and `writ adopt --platform`,
  the same way, each taking `--segment`. That resolves #970. Reconcile reads the deployed inventory from the store, not
  the layer trees, so it selects nothing and its `Config.Segments` stays unread; it resolves segments all the same, so
  a bad value is refused there as everywhere. #765 keeps its scopes; its segments half is this PR's (**Q26**).
- **A declared segment with no value** matches no directory name.

### Requirement 4: pkg/platform detects through the one package and falls back along ID_LIKE

- `detectHost` reads os-release through the one package and takes its managers from the ID or, failing that, from the
  closest `ID_LIKE` ancestor it lists. Pop!_OS takes Ubuntu's.
- An ID with no listed ancestor (Alpine, Void) is still refused here; turning the nil `Platform` that follows into a
  named error is #968's (**Q9**). Selection still works on such a host: its chain names its own ID.
- A host that falls back reports the ancestor as its `Platform.Distro()`, the distro whose managers it takes.
- `resolveLinuxDistro` is the resolution, pure, and tested from synthetic os-release fields. The tests that pin the
  ten, the aliases and `New`'s refusal of an unlisted distro (`detect_linux_test.go`, `detect_test.go`,
  `constructors_test.go`, `spec_test.go`, `defaults_test.go`) still hold, checked against the change: the ten and the
  aliases are unchanged, and `New` still refuses Alpine. `token_test.go` goes with `Token` (Requirement 5).

### Requirement 5: lore selects its phase scripts through the one package

- **lore has no project.** A package's directories are named by selectors alone (`docker/Darwin/Deploy/`). `Common` is
  the name with no selector words, the part writ's bare project directory plays, and comes first (**Q7**). So lore's
  names read `Common` or `<os>[.<arch>]`, and an architecture alone (`arm64/`) is a valid name (**Q24**).
- lore lists the package's directories and selects through the one package (**Q8**). On Ubuntu:
  `Common → Unix → Linux → Debian → Ubuntu`. A grammar error refuses the run, so a leftover `Linux.Debian/`, with two
  OS words, is reported (**Q19**).
- `resolveNative` takes the native manager from the detected `Platform`'s default, not a token prefix (**Q11**): deb is
  apt, rpm dnf, and alpm a new pacman source, so Arch and Manjaro stop being labeled apt. A host `pkg/platform` can't
  detect gets an error naming it rather than a guess. `Registry.Resolve` and `ResolveWithConfidence` lose their token
  parameter.
- The token parameter threaded through `builder.go`, `commands.go`, `package.go`, `lifecycle.go`, `search.go` and
  `origin.go` becomes the chain. The graph origin's `platform` annotation records the chain (**Q12**).
- Starlark's `platform.distro` stays pkg/platform's distro (**Q23**).
- A grammar error in any package refuses the run: `lore deploy` plans every package before any deploys, and the
  refusal lists every malformed directory of every package; nothing deploys.
- Dead code goes: `GetPhaseScript`, `HasPhase` and `DiscoverAllPhases` (no callers), the never-read
  `ScriptAction.Platform`, `BuildFromManifest` and `BuildFromPackages` (no callers), and `pkg/platform`'s `Token` and
  `DetectToken` with `token_test.go`. The Starlark accessor's doc says what `platform.distro` returns: pkg/platform's
  distro, lowercase.

### Requirement 6: Tests

- **One per row of the issue's table,** each from a synthetic os-release: Debian, Ubuntu, Linux Mint, Fedora, RHEL,
  CentOS Stream, Rocky, and no os-release. Also Pop!_OS (the fallback), Darwin and Windows.
- **The vocabulary:** on a synthetic Ubuntu host, `common.Darwin`, `common.Windows` and `microsoft.Windows` (the shape
  of the owner's personal layer) are excluded silently, with no error.
- **The ranking:** Q3's order of application on a synthetic Ubuntu arm64 host, for one file, in single- and
  multi-source builds; `Darwin > Unix`; manifests' contribution order.
- **Projects:** `.bashrc` in `common/`, `noblefactor/` and `thenobles/`, with `writ deploy noblefactor thenobles`,
  resolves to `thenobles/.bashrc`, and `common.Ubuntu/.bashrc` loses to `noblefactor/.bashrc`;
  `writ deploy noblefactor thenobles common` is refused as a command-line error.
- **Grammar errors:** `common.arm64.Debian`, `common.Linux.Debian` and `common.Debain` are reported together, in one
  refusal before anything changes, in a multi-layer writ build and in a lore package with a `Linux.Debian/` directory;
  `common.Fedora` on Ubuntu is excluded silently.
- **Extras:** a declaration that repeats a value, reuses an OS or architecture word, or names a built-in is refused when
  the configuration loads; `--segment` with an undeclared name or value is refused.
- **adopt:** each link of a synthetic Ubuntu chain is a valid `--platform`; `Linux.Debian` and `arm64.Debian` are
  refused.
- **The pinning tests corrected:** writ's `segment_test.go` is rewritten against the one package, so its cases that
  pinned two OS words (`:124-125`), Debian refused on Ubuntu (`:127`) and the suffix count
  (`TestMatchResultSpecificity`) went with the functions they tested; the tree tests assert collisions by directory;
  lore's `builder_test.go:39`, `:42`, `:92` and `:112-113` and
  `package_test.go:105`, which pass the `Linux.Debian` token, onto a synthetic chain.
- **lore:** a lifecycle test that selects over `Common/`, `Unix/`, `Linux/`, `Debian/` and `Ubuntu/` with a synthetic
  Ubuntu chain. None exists today.

### Requirement 7: The scenarios

- The layer-journey scenario's `issueLineageChain = 860` goes. Step 2.3b asserts that `common.Debian` deploys on
  `ubuntu-latest` and `ubuntu-24.04-arm`, and `consumersAtA` stops treating lineage as optional. Its own os-release
  reader stays as an independent oracle, limited to the IDs CI hosts carry (**Q10**). The doubled `runtime.GOOS` in its
  skip message is fixed.
- The first scenario asserts its `common.Linux` and `common.Debian` fixtures.

### Requirement 8: The documents

- **One place for selectors** (**Q15**): `docs/guides/selectors.md`, a guide the site publishes. It carries the chain,
  the grammar and its order, the vocabulary, the ranking, the extra segments and their declaration, the grammar errors,
  and the issue's design input: the os-release facts, the owner's rule, the chain table and the fallback.
- **writ refers to it.** `docs/guides/writ/platform-awareness.md` drops its own precedence rules and its `uname` and
  "matched in order" claims, points to the one place for the ruled order, and renames its `noblefactor.Linux.Debian`
  examples to `noblefactor.Debian`. `manage-environments.md` and `index.md` follow.
- **lore refers to it.** `docs/guides/lore/create-manifests.md` points to the one place; it and `pipeline.md` rename
  `Linux.Debian` and `Linux.Fedora`.
- **The architecture documents refer to it:** 2.5 restates lore's cascade as the host's chain with `Common` first, and
  its status document (`2.5-lifecycle-pipeline-construction.status.md:23`) names `Debian`; 3.4 loses its false "by
  descent" claim; the command-line document's segments section takes the ordered list and stops saying DISTRO
  resolves on every platform.
- **The plans:** #855's, #931's and #762's are brought into line; #369's (`docs/plans/segment-grammar-enforcement.md`)
  is set `abandoned`, noting this plan supersedes it (the process has no superseded status), and its `all` → `common`
  sweep goes to a follow-up issue (**Q25**). Plans that describe the retired token or `Linux.Debian` as current get a
  dated note.
- `docs/package-reference.md` and `docs/package-hierarchy.md` name the one package.

### Requirement 9: #959's owed boxes

`docs/plans/fix/959-writ-never-detects-the-linux.md`:
- **The convergence box is ticked.** DANOBLE-UD24-1 converged on `v0.1.0-dev.20260930024252`, and its graph records
  `DISTRO: Ubuntu`, with all 233 writ links unchanged.
- **The other-session box is ticked** once the owner confirms that the notice of 2026-09-30 reached the other
  session.
- **The plan is set `complete`.**

## Implementation Phases

### Phase 1: Commit the plan

- [x] This plan, approved, before the change. #944's table links it, and its Design row names
      `docs/guides/selectors.md`; #949's lane 19 names #369 and #970

### Phase 2: The one package, and pkg/platform on it (Requirements 1, 4, 6)

- [x] The package, with its tests first, failing: the per-row chains, the vocabulary, the grammar and its errors
- [x] `pkg/platform` detects through it and falls back along `ID_LIKE`; its pinning tests checked

### Phase 3: writ (Requirements 2, 3, 6)

- [x] Detection and selection; the builder; the refusal on grammar errors; adopt; the collision record
- [x] The extra segments: the configuration, its schema and defaults, validation, and every command taking them
- [x] The ranking, grammar-error, extras and adopt tests; the pinning tests corrected; the dead code removed

### Phase 4: lore (Requirements 5, 6)

- [x] Selection with `Common` first; the refusal; `resolveNative`; the chain threaded through; the dead code removed
- [x] The lifecycle test; lore's pinning tests corrected

### Phase 5: The scenarios and the documents (Requirements 7, 8)

- [x] The scenarios
- [x] `docs/guides/selectors.md`; the guides, the architecture documents, the plans and the package maps

### Phase 6: Verify

- [x] `gofmt`; `make test`; CI's quality gate; `make test-scenario`
- [x] #959's owed boxes (Requirement 9)

### Phase 7: Merge

- [x] PR script written, shown, and handed over. The PR resolves #944, #369 and #970
- [ ] After the merge, DANOBLE-UD24-1 converges on the official pre-release: `Home/common.Debian`'s four files deploy on
      this Ubuntu host, and no other link changes
- [ ] The follow-up issues filed: the registry's text (**Q16**), a non-Ubuntu CI leg (**Q18**) and #369's `all` →
      `common` sweep (**Q25**); comments left on #849 and #765
- [ ] The bare form's follow-up issues filed (**Q27**), one in noblefactor-ops and one in personal: their open boxes
      that say `writ deploy common` say `writ deploy`, and noblefactor-ops's README drops "the binary still spells these
      two lines `writ repo add` and `writ deploy common`"

## Out of Scope

- **The nil `Platform` for an ID with no listed ancestor:** #968.
- **lore's declared-but-unread platform surfaces** (`platforms:`, `--platform`, the schema's enum): #969.
- **Cross-platform planning** (a graph built on macOS for Ubuntu): #282.
- **#765's scopes.**

## Decisions

No question is open. Q3, Q4, Q15, Q19, Q20, Q21 and Q27 are the owner's rulings. The others follow from the rulings or
are engineering choices, decided here, and the owner's review of this plan can overrule any of them.

- **Q1. Where does the one package live?** A new leaf package, `pkg/selector`. writ's `segment`, lore's `lifecycle`
  and `pkg/platform`'s `detectHost` call it. `pkg/platform.Detect` doesn't fit writ: it spawns `systemctl`, `sw_vers`
  and `cmd`, refuses unknown hosts, and uses Docker architecture names (`arm/v7`) that can't be a directory suffix.
- **Q2. How is an ID spelled as a selector word?** One table (`ubuntu → Ubuntu`, `rhel → RHEL`, `centos → CentOS`,
  `linuxmint → Mint`, `almalinux → AlmaLinux`, the openSUSE IDs → `OpenSUSE`), holding every ID `pkg/platform` lists;
  any other ID gets its first letter uppercased, as today.
- **Q3. How do the parts rank against each other?** In the order a name reads: the OS link's depth first, then the
  architecture, then each extra in configured order, each part narrowing the one to its left; then the directory
  name. On this Ubuntu arm64 host the order of application is `common`, `common.arm64`, `common.Unix`,
  `common.Unix.arm64`, `common.Linux`, `common.Linux.arm64`, `common.Debian`, `common.Debian.arm64`,
  `common.Ubuntu`, `common.Ubuntu.arm64`, and the last applied wins: `common.Ubuntu` beats `common.Debian.arm64`.
  Ruled 2026-09-30: "architecture beats no architecture", and of the two orders, "the second".
- **Q4. Layers.** Directories are selected and ordered within a layer, and layers are processed left to right, base,
  team, personal. Ruled 2026-09-30: "layers work as layers work."
- **Q5. What do DISTRO and its overrides mean?** DISTRO stays the ID. The ancestors are selected on but get no
  template variable. `WRIT_SEGMENT_DISTRO` and `--segment DISTRO=` replace the ID and keep the detected ancestors.
- **Q6. What are the chains outside Linux?** Darwin `Unix → Darwin`; Windows `Windows`; FreeBSD, OpenBSD and NetBSD
  `Unix → <OS>`.
- **Q7. Where does lore's `Common` sit?** First: it is the name with no selector words.
- **Q8. Does lore select with the one package?** Yes. lore lists the package's directories and selects through it, so
  `Debian.arm64/` becomes possible.
- **Q9. What happens on a Linux ID with no listed ancestor?** Detection and selection work: the chain ends at its own
  ID. `pkg/platform` still refuses it, and #968 names the error. No supplementary lineage table for distributions
  whose os-release omits `ID_LIKE`: os-release is the source.
- **Q10. What does the layer-journey scenario do?** Step 2.3b becomes a hard assertion in this PR, and its oracle stays
  independent of the code under test.
- **Q11. How does lore choose the native package manager?** The detected `Platform`'s default manager, after the
  `ID_LIKE` fallback.
- **Q12. What does the graph origin's `platform` annotation record?** The chain, as a list.
- **Q13. What does `writ adopt --platform` accept?** A suffix that passes the grammar and matches this machine: each
  link of its chain, its architecture, its extras' values, in order. So `Debian` is accepted on Ubuntu; `Fedora`,
  `Linux.Debian` and `arm64.Debian` are refused.
- **Q14. Does the dead selector code go in this PR?** All of it, including what the readers found beyond the issue's
  list (`DetectSegmentsWithNames`, `Segments.Values`, `Config.SegmentMap`, `op.Collision`).
- **Q15. Where does the design live?** In one place, `docs/guides/selectors.md`, with references from writ's and
  lore's documentation (Requirement 8). Ruled 2026-09-30: "we want one location with references from two locations:
  lore and writ."
- **Q16. What happens to the registry's text?** A devlore-registry issue, opened after this PR merges. The registry has
  no directories to rename, and its knowledge files are hand-written (`generated: false`), which #92 governs.
- **Q17. What happens to the star lint fixture's `Linux.Debian` and `Linux.Fedora`?** They stay. The fixture is a
  verbatim copy of the registry at `cc87c4f0`, kept as #721's regression corpus, and the linter never reads the
  platform directory; selection never reads it either. Its README says why.
- **Q18. Does CI exercise a non-Ubuntu distribution?** Not in this PR. The synthetic os-release tests cover every row;
  a container leg is filed separately.
- **Q19. Is a name that breaks the grammar reported, or skipped?** Out of order (`common.arm64.Debian`), two OS words
  (`common.Linux.Debian`) or a word outside the vocabulary (`common.Debain`): reported, naming the directory and the
  rule it breaks, and the run refuses before anything changes, listing every violation at once. A well-formed name for
  another machine (`common.Fedora` on Ubuntu) is excluded silently. Ruled 2026-09-30: "yes, report them", and "refuse
  the run".
- **Q20. How is a word attributed to its segment?** A name carries values, never segment names, so each word's segment
  is inferred: left to right against the slots in order (OS, architecture, then each extra in configured order), each
  word taken by the first remaining slot whose vocabulary holds it, each slot at most once. A word no remaining slot
  holds is a grammar error (**Q19**). Segment values are unique: a declaration that gives two segments one value, an
  extra's value included against the OS and architecture words, is refused when the configuration loads. Ruled
  2026-09-30: "the attribution works for me. It is, I think, reasonable to ask that segment values be unique."
- **Q21. What is the vocabulary a word is judged against?** A closed list: every OS word (Q6), every distribution word
  the Q2 table spells, the host's own chain (so an unlisted distribution names itself on its own host), every
  architecture in both spellings, and the declared extras' values. That is what lets Q19 tell `common.Debain`, a
  misspelling, from `common.Fedora`, another machine's name. A distribution missing from the table must be added
  before a layer on another host can name it. Ruled 2026-09-30: "i agree 100% with the closed vocabulary. we should
  complain if we encounter a host such as `common.Nixos` that we don't know."
- **Q22. Do the built-ins take overrides without a declaration?** Yes: OS, DISTRO and ARCH take `--segment` and
  `WRIT_SEGMENT_*` as today, with DISTRO's meaning per Q5; no declaration may use their names.
- **Q23. What does Starlark's `platform.distro` report?** pkg/platform's distro, as before: the ID in its vocabulary
  (`linuxmint` is `mint`, `centos` is `centos-stream`), lowercase, and `macos` or `windows` off Linux. On a distribution
  it doesn't list, that's now the closest listed `ID_LIKE` ancestor (Requirement 4). No lineage accessor in this PR:
  the directories carry the lineage. (Corrected 2026-09-30: this said "the raw ID", which it never was.)
- **Q24. How do lore's names read?** Without a project: `Common`, or `<os>[.<arch>]`, and an architecture alone
  (`arm64/`) is valid. The one package takes the project part as optional, and each caller says whether its names
  carry one.
- **Q25. What happens to #369's plan?** It is set `abandoned`, with a note that this plan supersedes it, delivering
  its order, its slots, a total order and its errors. Its `all` → `common` sweep, which isn't selector work, goes to a
  follow-up issue.
- **Q26. What happens to #765?** Its segments half is this PR's (Requirement 3); it keeps its scopes, and gets a
  comment that links this plan.
- **Q27. How are projects ordered?** As the command line reads: the implicit projects first (`common`, then the one
  named after each layer repository), then the named projects left to right; within each project, the platform
  ranking (Q3). So with `writ deploy noblefactor thenobles`, `thenobles/.bashrc` beats `noblefactor/.bashrc`, which
  beats `common.Ubuntu/.bashrc`. Naming an implicit project (`writ deploy noblefactor thenobles common`) is a
  command-line error: `common` is always first. Ruled 2026-09-30: "The evaluate left to right in the order specified on
  the command line", and of refusing an explicit `common`, "agreed. it's a command line error." Ruled again the same
  day, of #850's bare form: "`writ deploy common` is an error", and "the bare form is `writ deploy`". This supersedes
  #850's 2026-09-26 ruling, and #850's and #918's plans carry dated notes.
- **Q28. Where does a named project go when it is also recorded?** In its command-line place: the recorded projects not
  named apply after the implicit ones, and the named ones after them, in command-line order, as Q27 rules. Naming a
  project twice is refused as a command-line error, because its order would be ambiguous.
