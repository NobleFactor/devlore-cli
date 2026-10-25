---
title: "2.7 gains the channels, the wire protocol and the transport, hop by hop, and two open questions on SSH"
issue: https://github.com/NobleFactor/devlore-cli/issues/1174
status: approved
created: 2026-10-24
updated: 2026-10-24
---

# Plan: the channels, the wire protocol and the transport, by hop

## Summary

**The problem.** After #1173 merged, the execution-model session laid out, hop by hop, what channels a request uses, what the messages look like on the wire, and what carries them in each iteration. The owner asked that the whole of it, its tables and its code included, be kept in the design documents, and that the two gaps it showed be recorded as open before they are settled. None of it is in a document.

**The deliverable.** A section of 2.7, Channels, wire protocol and transport, by hop, carrying the account whole. Two questions added to 2.7 § Open and to the open-questions list of #1159's plan: one SSH connection per processor or per subgraph, and on the far side an executor process per SSH session or a daemon the sessions reach. 6.2 points to them. Documents only.

## Issue 1174

[2.7 gains the channels, the wire protocol and the transport, hop by hop](https://github.com/NobleFactor/devlore-cli/issues/1174). A chore under Epic [#1118](https://github.com/NobleFactor/devlore-cli/issues/1118), The DevLore Broker. Taken before lane 3 of schedule [#1127](https://github.com/NobleFactor/devlore-cli/issues/1127), on the owner's word of 2026-10-24.

## Requirements

1. **2.7 § Channels, wire protocol and transport, by hop**, placed before § Message reference: the logical channels with the console's table; the wire protocol of the requests, the console's Server-Sent Events frames with an example, the runner hop's frames and OTLP; the transport of each hop in each iteration as a table; and the SSH session's mapping of stdin, stdout, stderr, exit status and cancel.
2. **2.7 § Open 21 and 22**: a connection per processor or per subgraph, recommending Go's SSH package's sessions on one connection as the deciding factor of § Open 18; and an executor process per session or a daemon, recommending a process per session first.
3. **#1159's plan** lists the same two as outstanding questions 9 and 10, dated.
4. **6.2 § Open 14** points to 2.7 § Open 21 and 22.
5. **2.7's status companion** carries a dated line.

## Implementation Phases

- [x] The plan, approved on the owner's instruction of 2026-10-24, committed alone on `chore/1174-wire-by-hop`
- [x] Requirements 1 to 5, one commit -- 2026-10-24
- [ ] `make check` passes on the committed tree; the pull request, its description saying AI assisted and ending with `Signed-off-by:` and `Assisted-by: Claude Code`; the script is `~/Workspace/NobleFactor/devlore-cli.1174-wire-by-hop.bash`; status `complete` with it, since nothing deploys

## Files to Create/Modify

| File | Action | Purpose |
| --- | --- | --- |
| `docs/plans/chore/1174-wire-by-hop.md` | Create | This plan |
| `docs/architecture/2.7-control-plane.md`, `.status.md` | Modify | Requirements 1, 2, 5 |
| `docs/architecture/6.2-process-pool.md` | Modify | Requirement 4 |
| `docs/plans/chore/1159-stream-rulings.md` | Modify | Requirement 3 |
