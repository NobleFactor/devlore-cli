---
title: "The Charm stack moves to v2 under charm.land: glamour, Lip Gloss, Bubble Tea and Bubbles together"
issue: https://github.com/NobleFactor/devlore-cli/issues/1101
status: active
created: 2026-10-20
updated: 2026-10-20
---

# Plan: the Charm stack moves to v2

## Summary

devlore-cli renders `--output terminal` with glamour and runs its interactive console (`writ migrate`) on Bubble Tea,
Bubbles and Lip Gloss, all at their v1 lines. Each has a v2 under new `charm.land` import paths, and the four move
together, so the build carries one Lip Gloss. Ruled 2026-10-20, asked whether to take glamour v2: "Whole stack to v2."
Then: "We should do that immediately."

## Goals

1. **One Charm stack, at its current major:** glamour, Lip Gloss, Bubble Tea and Bubbles at v2, under `charm.land`,
   and nothing of their v1 lines left in `go.mod`.
2. **`--output terminal` renders what it renders today:** the same attributes, colors, wrap and tables; `NO_COLOR`
   drops color and keeps attributes; nothing is probed.
3. **The console behaves as it does today:** the same keys, alternate screen for `Run` and none for `RunInline`, the
   same styles.

## Current State

At `65c8b40f`. Versions read 2026-10-20 from each repository's releases.

| Library | Pinned | Moves to | Import path |
| --- | --- | --- | --- |
| glamour | v0.10.0 | v2.0.1 (2026-06-12) | `charm.land/glamour/v2` |
| Lip Gloss | v1.1.1 snapshot (2025-04-04) | v2.0.6 (2026-08-11) | `charm.land/lipgloss/v2` |
| Bubble Tea | v1.3.10 | v2.1.0 (2026-10-08) | `charm.land/bubbletea/v2` |
| Bubbles | v1.0.0 | v2.2.1 (2026-08-24) | `charm.land/bubbles/v2` |
| termenv | v0.16.0, direct | dropped as a direct requirement | — |
| colorprofile | v0.4.2, indirect | v0.4.3 (2026-03-09), direct | `github.com/charmbracelet/colorprofile` |

glamour v2.0.1 parses with goldmark v1 (it requires ≥ v1.7.8; the build has v1.7.16). goldmark's own version is
#1100's, not this plan's.

Four files import the stack:

| File | Lines | Uses |
| --- | --- | --- |
| `pkg/result/terminal.go` | 230 | glamour, `glamour/ansi`, `glamour/styles`, termenv: `WithColorProfile(termenv.TrueColor or Ascii)` at :104 |
| `cmd/internal/console/console.go` | 150 | Bubble Tea: `tea.NewProgram` with `WithInput`, `WithOutput`, and `WithAltScreen` in `Run` (:57) only; Lip Gloss `Render` printed with `fmt.Fprint` |
| `cmd/internal/console/model.go` | 446 | Bubble Tea `Model`, `View() string`; `tea.KeyMsg` and `msg.Type` (`KeyCtrlC`, `KeyEsc`, `KeyEnter`, `KeySpace`, `KeyUp`, `KeyDown`); Bubbles `textinput`, `spinner`; glamour `WithAutoStyle()` at :51 and :426 |
| `cmd/internal/console/styles.go` | 160 | Lip Gloss: `Theme` fields typed `lipgloss.Color` |

The console's one caller is `writ migrate` (`cmd/writ/writ/migrate_cmd.go:165`).

## Requirements

### Requirement 1: Imports and modules

Every import moves to `charm.land/<library>/v2`. `go.mod` requires the four v2 modules and `colorprofile` directly, and
none of the four v1 modules or termenv; `go mod tidy` leaves it so.

### Requirement 2: `--output terminal` (`pkg/result/terminal.go`)

glamour v2 is pure: it always emits full color and leaves downsampling to the writer (its UPGRADE_GUIDE_V2.md, "Handle
Color Downsampling Explicitly"). `WithColorProfile` is removed.

- With color, the rendering is written as glamour emits it: TrueColor, as today. Nothing is probed.
- Under `NO_COLOR`, the rendering goes through a `colorprofile.Writer` whose `Profile` is set to `colorprofile.Ascii`,
  not detected: Ascii drops every color and keeps bold, italic and underline, which is no-color.org's rule and §10's.
- Code blocks: glamour v1 handed a block to chroma only when the profile was not Ascii. v2 always highlights; under
  `NO_COLOR` the writer strips chroma's colors. The doc comment on `terminalStyle` says so.
- `trimPadding`: glamour v2 still pads lines (`ansi/margin.go`, `NewPaddingWriter`). It stays if v2's output still ends
  lines in padding, measured in Phase 2; if v2 no longer pads, it and its test are deleted.

### Requirement 3: The console (`cmd/internal/console`)

By the Bubble Tea, Bubbles and Lip Gloss upgrade guides:

- `View()` returns `tea.View`. The alternate screen is a field of the view, not a program option: the model carries
  whether it runs on the alternate screen, `Run` sets it and `RunInline` does not, and `View` sets `AltScreen` from it.
- Keys: `tea.KeyMsg` becomes `tea.KeyPressMsg`; `msg.Type` becomes `msg.Code`; ctrl+c is matched by
  `msg.String() == "ctrl+c"`; space by its code, since `String()` now says `"space"`.
- `Theme`'s fields become `color.Color` (`image/color`), built with `lipgloss.Color("39")`; `lipgloss.Color` is now a
  function.
- glamour's `WithAutoStyle()` is removed. The console asks Bubble Tea for the terminal's background
  (`tea.RequestBackgroundColor`, answered by `tea.BackgroundColorMsg`) and renders with glamour's `dark` or `light`
  style by the answer; until it arrives, `dark`, glamour v2's default. This is the Bubbles guide's recommended route
  (section 4, "Query via Bubble Tea"), and it keeps today's behavior: v1's auto style read the background too.
- `Print`, `PrintStyled`, `Success`, `Warning`, `Error` and `Info` write outside Bubble Tea. Lip Gloss v2's `Render`
  always emits full color, so the console wraps its output in `colorprofile.NewWriter(output, os.Environ())`, which
  detects the terminal and honors `NO_COLOR`, as Lip Gloss v1's renderer did. Inside a program, Bubble Tea downsamples
  itself.

### Requirement 4: The specification

`docs/architecture/10-command-line-interface.md` §10 cites termenv's `termenv.go:100` and Lip Gloss v1's
`renderer.go:74` as how the console honors `NO_COLOR`, and says `--output terminal` fixes glamour's profile to
TrueColor. Both rows are restated for v2: colorprofile's `Detect` for the console, and the fixed Ascii writer under
`NO_COLOR` for `--output terminal`, cited at the versions this plan pins.

### Requirement 5: The markdown provider's design stub

Part of #1100, riding this branch (ruled 2026-10-20: "Stub in the design doc now, reference it in the issue"):
`docs/architecture/3.5.17-markdown-provider.md` and its status companion record what is decided (goldmark v1.8.6;
the terminal through glamour v2) and what is to design; `3.5-provider-catalog.md` lists `markdown` as proposed. #1100's
Design row names them. The pull request says `Part of #1100`; it does not resolve it.

## Implementation Phases

### Phase 1: The plan

- [x] The markdown provider's design stub (Requirement 5), committed with this plan
- [x] This plan committed on `chore/1101-charm-stack-moves-to-v2-under` and reviewed with the owner: approved 2026-10-20

### Phase 2: `--output terminal` (Requirements 1 and 2)

- [x] glamour v2 and colorprofile in `go.mod`; `pkg/result/terminal.go` on glamour v2. termenv is no longer
      imported; it stays an indirect requirement, through Lip Gloss v1, until Phase 3
- [x] Before and after, the same documents rendered and compared: every `pkg/result/terminal_test.go` test passes
      unchanged, and `make test` is green. Rendered 2026-10-20 with star built at `385c6a90` (v1) and with this
      change (v2), from a directory outside every repository:
      - `star docs starlark --output terminal`, with and without `NO_COLOR`: the visible text is identical; the
        escape codes differ.
      - The schedule report for #949, `--output terminal`, with and without `NO_COLOR`: two differences, both v2's.
        Lip Gloss v2 sizes table columns differently (`item_title` two columns wider, the last column narrower).
        glamour v2 writes each link in a table cell as an OSC 8 hyperlink (234 in the report), with an `id` that
        is an FNV hash of the URL, so two renderings are byte-identical and §7 holds; v2 has no option to turn
        them off. The links are still collected under the table, as before.
- [x] `trimPadding` kept: with it disabled, `make test` fails `TestTerminalFormatter_NoLineEndsInPadding` (every
      line ends in padding to the wrap width), so glamour v2 still pads

### Phase 3: The console (Requirements 1 and 3)

- [x] Bubble Tea, Bubbles and Lip Gloss v2; `console.go`, `model.go`, `styles.go` on the v2 APIs. `go.mod` holds no
      Charm v1 module and no termenv, not even indirectly
- [x] Tests for what changed: each key a step handles, by `tea.KeyPressMsg` (`TestModel_Keys`, `TestModel_Cancel`);
      `View().AltScreen` true under `Run` and false under `RunInline` (`TestConsole_AltScreen`); the style chosen from
      a `tea.BackgroundColorMsg` (`TestModel_StyleFollowsBackground`); a resize (`TestModel_Resizes`)
- [x] ~~`writ migrate` run in a scratch home, in a terminal~~. `writ migrate` refuses to run without an AI model
      provider, and none is local here, so a run sends the scratch directory's analysis to an outside service. Ruled
      2026-10-20, offered that or a test: "Program-level test." `TestConsole_RunsAProgram` runs a real Bubble Tea v2
      program through `RunInline` on the bytes a terminal sends (`y`, `abc`, return) and checks the session's
      responses; `TestConsole_CtrlCCancels` sends `\x03`. What a step renders stays `TestModelView`'s: v2 renders by
      frame, and scripted keys are read before the first. `make test` green twice and `make test-race` green

**Found along the way, and fixed here.** All three are in the console as committed at `65c8b40f`, not made by the
move; the program-level tests found them.

- An input step never took typing. `Update` sent every key to `handleKey`, and `handleInputKey` acted on enter
  alone, so no character reached the text input: `TestConsole_RunsAProgram` responded `""` where it typed `abc`.
  Every other key now goes to the text input.
- A message arriving after the session ended dereferenced a nil step (`m.step.Type` in `Update`): the program
  panicked. The step is checked first.
- `renderStep` returned a command that read `m.step` and `m.renderer` on Bubble Tea's command goroutine while the
  next update wrote them: `make test-race` reported the race. The step now renders in the update that asks for it,
  and the `stepRenderedMsg` round trip is gone.

### Phase 4: The specification (Requirement 4)

- [ ] §10's rows restated, with citations at the pinned versions

### Phase 5: Merge

- [ ] `gofmt -l` clean; `make check`, `make test` and `make test-scenario` green
- [ ] PR script written, shown, and handed over; the PR resolves #1101

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `go.mod`, `go.sum` | Modify | Requirement 1 |
| `pkg/result/terminal.go`, `pkg/result/terminal_test.go` | Modify | Requirement 2 |
| `cmd/internal/console/console.go`, `model.go`, `styles.go`, `console_test.go` | Modify | Requirement 3 |
| `docs/architecture/10-command-line-interface.md` | Modify | Requirement 4 |
| `docs/architecture/3.5.17-markdown-provider.md`, `3.5.17-markdown-provider.status.md` | Create | Requirement 5 |
| `docs/architecture/3.5-provider-catalog.md` | Modify | Requirement 5 |

## Related Documents

- #1101, this chore; #1019, keeping current; #1021, the monthly module update that first listed the Charm v2 set
- #1100, the markdown provider, which settles goldmark at v1.8.6
- glamour [UPGRADE_GUIDE_V2.md](https://github.com/charmbracelet/glamour/blob/main/UPGRADE_GUIDE_V2.md), and the
  Bubble Tea, Bubbles and Lip Gloss `UPGRADE_GUIDE_V2.md` in their repositories

## Out of Scope

- **goldmark's version.** #1100 settles it at v1.8.6; this plan leaves the build on what glamour v2 and go.mod give.
- **New v2 features** (Bubble Tea's cursor, progress bar and keyboard enhancements). The move keeps behavior.

## Decisions

- **D1. No probe in `--output terminal`.** The `NO_COLOR` writer's profile is set, not detected, as today's termenv
  profile is: §7's rule that a rendering does not change when it is observed.
- **D2. The console detects.** It is interactive and already did, through Lip Gloss v1's renderer and glamour's auto
  style; v2 keeps that through colorprofile and Bubble Tea's background query.

## Open Questions

None.
