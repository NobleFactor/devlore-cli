---
title: "Layer-journey fixtures"
description: "Three layers and the move of 2026-09-07, for the writ layer-journey scenario (devlore-cli#855)"
---

# Layer-journey fixtures

`base/` is noblefactor-ops's `Home/common` as of the commit in `base/PIN`: `Declare-BashScript`, its man
page and completions, plus `git-scenario`, a git-supporting command that sources the sibling. `team/` is
content only — no packages, per #762. `personal-a/` is the personal layer before the move of 2026-09-07:
its own copy of the helper, consumers under `common.<selector>/local/bin` that source it, a `noblefactor`
project, the context projects `thenobles` and `microsoft`, a `Home/devlore-cli` project (#850's own example
of a layer's overrides for working with another repository), and `common/.config/scenario/personal.conf`
as its own common marker.

`personal-b`, the layer after the move, is not checked in: the scenario derives it from `personal-a`
(`applyMove` in `scenario_layer_journey_test.go`) so the two cannot drift. The bridge symlink
`Home/common/local/bin/Declare-BashScript` is created by the harness for the same reason a symlink is not
checked in: a Windows checkout would flatten it.

Every script is the standard shape and passes `star lint shell`.
