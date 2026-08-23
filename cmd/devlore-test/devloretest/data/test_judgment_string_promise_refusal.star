# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_string_promise_refusal.star — judgment scenario: the plan-time mirror (docs/plans/
# resource-construction.md, explicit-conversion suite item 3).
#
# A producer whose DECLARED result is a string cannot fill a resource-typed slot: at graph dispatch a
# string is a key, never a constructor, and a run-computed string is never a recorded identity — so the
# binding refuses at plan time (checkPromiseTypes' graph-context narrowing), not deep into a run.
# Undeclared producers meet the dispatch refusal instead (suite item 2b).

src = t.tmp("input.txt")
dst = t.tmp("never.txt")

t.write(src, "some content")

t.expect_error("returns a string, but the slot is resource-typed")

read = plan.file.read_text(resource=src)
bad = plan.file.copy(source=read, destination_path=dst, mode=0o600)

plan.assemble_definition([read, bad])
