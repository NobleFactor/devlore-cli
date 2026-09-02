---
title: "Writ Repo Command"
status: complete
created: 2026-08-09
updated: 2026-08-09
---

# Plan: Writ Repo Command

## Summary

The fresh-user packaging command chartered from the writ-deploy scenario's Q2 re-ruling:
layer registration had no first-class surface (a hand-made symlink), and the repositories
guide documented a `writ config set writ.repos.*` mechanism nothing reads. `writ repo`
closes both: `add <layer> <path>`, `remove <layer>` (alias `rm`), `list` (alias `ls`), and
bare `writ repo` acting as list (ruled 2026-08-09; git-remote's idiom, docker's aliases).
Registration remains what it always was — a symlink under `XDG_DATA_HOME/devlore/writ/layers`
— packaging, never configuration, so the future config API owes this nothing.

## Delivered

1. `cmd/writ/writ/repo_cmd.go` + unit tests (round-trip, bare invocation, aliases,
   unknown-layer / missing-path / double-add / remove-unregistered errors, broken-link
   marker in list).
2. The scenario harness dogfoods `writ repo add` for its layer registration — the
   fresh-user path proven on ubuntu, macos, and windows by every scenario run.
3. `docs/guides/writ/repositories.md` rewritten onto reality: `writ repo` registration
   (the config-set fiction removed), the corrected `Home/<project>[.<Segment>]` structure
   (was upside-down `<project>/Home/`), `writ reconcile` in place of the retired
   `writ inspect`, and the fictional "Configuration storage" section replaced by the
   layers-directory truth.

Landed as [PR #352](https://github.com/NobleFactor/devlore-cli/pull/352); squash-merged to
`develop` 2026-08-09.
