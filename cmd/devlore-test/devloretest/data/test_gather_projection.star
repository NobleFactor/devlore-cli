# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_gather_projection.star — field projection over record-shaped gather items (phase-8 step 45).
#
# Items are records; body slots project fields — including a multi-invocation body writing two distinct
# per-iteration paths, the shape the A4 single-value limitation previously forbade. plan.variable("item",
# field=...) is the primitive; plan.item(field) is sugar over it (one invocation uses each surface).

items = [
    {"path_a": t.tmp("proj0_a.txt"), "path_b": t.tmp("proj0_b.txt"), "content": "alpha"},
    {"path_a": t.tmp("proj1_a.txt"), "path_b": t.tmp("proj1_b.txt"), "content": "beta"},
]

write_a = plan.file.write_text(
    destination_path=plan.item("path_a"),
    content=plan.item("content"),
    chmod=0o644,
)
write_b = plan.file.write_text(
    destination_path=plan.variable("item", field="path_b"),
    content=plan.variable("item", field="content"),
    chmod=0o644,
)

g = plan.gather(items=items, limit=2, body=[write_a, write_b])

graph = plan.assemble_definition([g])

t.expect_file(t.tmp("proj0_a.txt"), content="alpha")
t.expect_file(t.tmp("proj0_b.txt"), content="alpha")
t.expect_file(t.tmp("proj1_a.txt"), content="beta")
t.expect_file(t.tmp("proj1_b.txt"), content="beta")

t.run(graph)
