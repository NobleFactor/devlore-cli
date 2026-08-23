# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_runtime_escape_refusal.star — judgment scenario: the runtime grammar's escape refusal
# (docs/plans/resource-construction.md, explicit-conversion suite item 9, the star-portable direction).
#
# A run-computed path that escapes the run's root refuses at the discover node with the grammar's
# verdict — the same confinement the plan-space grammar enforces for authored paths, applied to the
# runtime dialect. The path arrives through a promise, exactly as a tool would deliver it. The
# machine-absolute directions (under-root rebase; outside-root refusal; volume spellings) are pinned in
# Go (`NormalizeRuntimePath`'s table), which runs on every CI platform.

pathfile = t.tmp("emitted-path.txt")
dst = t.tmp("never.txt")

t.write(pathfile, "../outside.txt")

read = plan.file.read_text(resource=pathfile)
found = plan.file.discover(path=read)
copied = plan.file.copy(source=found, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([read, found, copied])

t.expect_error("escapes the run's root")
t.expect_no_file(dst)

t.run(graph)
