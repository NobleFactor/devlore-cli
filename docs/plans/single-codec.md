---
title: "Single Codec"
status: draft
created: 2026-08-04
updated: 2026-08-04
---

# Plan: Single Codec

> **Authoring status**: this plan is being authored by the project owner. This file currently
> holds the plan's collected requirements and the current-state inventory feeding that
> authoring; it is not the design. Referenced by
> [fix-trace-format-leak.md](fix-trace-format-leak.md), which defers its unlegislated
> canonical-form corners here.

## Summary

One canonical model for op documents. YAML, JSON, and protobuf are renderings of that model;
two documents with the same checksum are the same logical document regardless of rendering.
The raw-bytes canonical from PR #298 is interim until this plan lands.

## Requirements

### Requirement 1 — the framework owns both directions of graph-document handling (ruled 2026-08-04)

`pkg/op` must own graph-document handling completely, in both directions, including all
aspects of serialization. No serialization decision — encoder selection, indentation,
document-stream framing, close-error handling — may live in a CLI.

**Current state.** The framework already owns most of this:

| Surface | Location | Direction |
| --- | --- | --- |
| `op.LoadGraph` (YAML/JSON decode) | [pkg/op/graph.go:243](../../pkg/op/graph.go) | read |
| `Graph.Serialize` (via `op.Encoder`) | [pkg/op/graph.go:729](../../pkg/op/graph.go) | write |
| `yaml.v3` import | [pkg/op/graph.go:28](../../pkg/op/graph.go) | both |

**The gap (closed 2026-08-04).** The dry-run rendering of a graph sequence — one YAML
stream, indent 2, encoder close-error folded into the return — previously lived outside the
framework as three byte-identical `emitGraphs` copies (writ deploy, upgrade, decommission).
`op.SerializeGraphs(w io.Writer, graphs []*op.Graph) error` now owns that rendering beside
`op.LoadGraph` in [pkg/op/graph.go](../../pkg/op/graph.go); each command reduced to
`return op.SerializeGraphs(os.Stdout, graphs)`, the destination writer the only caller
choice. No `pkg/op` sub-package yet: where serialization surfaces consolidate is this
plan's decision, and a single free function does not preempt it.
