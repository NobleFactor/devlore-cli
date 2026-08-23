# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_discover_kind_verdict.star — judgment scenario: kind assertion sharpens the verdict
# (docs/plans/resource-construction.md, explicit-conversion suite item 5).
#
# discover(kind="regular") over a directory fails AT THE DISCOVER NODE with the kind-mismatch verdict —
# kinds are lstat-strict, and the assertion is opt-in strictness whose verdict lands at the asserting
# action's own node. The consumer never dispatches.

dir = t.tmp("a-directory")
dst = t.tmp("never.txt")

t.mkdir(dir)

found = plan.file.discover(path=dir, kind="regular")
copied = plan.file.copy(source=found, destination_path=dst, mode=0o600)

graph = plan.assemble_definition([found, copied])

t.expect_error("does not satisfy the regular assertion")
t.expect_no_file(dst)

t.run(graph)
