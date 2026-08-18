# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_gather_projection_missing_field.star — plan-time record validation (phase-8 step 45): a body projection
# of a field absent from an immediate item is a plan error at plan.gather, never a nil at dispatch.
# Plan-time validation only: the script never calls t.run.

t.expect_error("items\\[0\\] is missing field \"content\"")

items = [{"path": t.tmp("never.txt")}]

inv = plan.file.write_text(
    destination_path=plan.item("path"),
    content=plan.item("content"),
    mode=0o644,
)

plan.gather(items=items, limit=1, body=[inv])
