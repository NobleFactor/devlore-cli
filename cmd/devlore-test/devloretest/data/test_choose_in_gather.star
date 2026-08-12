# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_choose_in_gather.star — plan.choose nested in a gather body, slots projected from the iteration item
# (phase-8 step 45): the when-predicate and both branches resolve plan.item(...) through the inherited
# per-iteration frame (the A1 frame-inheritance rule; no choose-specific machinery).

probe_hit = t.tmp("probe_present.txt")
probe_miss = t.tmp("probe_missing.txt")

pre = plan.file.write_text(destination_path=probe_hit, content="here", chmod=0o644)

items = [
    {"probe": probe_hit, "hit": t.tmp("cig0_hit.txt"), "miss": t.tmp("cig0_miss.txt")},
    {"probe": probe_miss, "hit": t.tmp("cig1_hit.txt"), "miss": t.tmp("cig1_miss.txt")},
]

c = plan.choose(
    plan.case(
        when=plan.file.exists(path=plan.item("probe")),
        then=plan.file.write_text(destination_path=plan.item("hit"), content="hit", chmod=0o644),
    ),
    default=plan.file.write_text(destination_path=plan.item("miss"), content="miss", chmod=0o644),
)

g = plan.gather(items=items, limit=1, body=[c])

graph = plan.assemble_definition([pre, g])

t.expect_file(t.tmp("cig0_hit.txt"), content="hit")
t.expect_no_file(t.tmp("cig0_miss.txt"))
t.expect_no_file(t.tmp("cig1_hit.txt"))
t.expect_file(t.tmp("cig1_miss.txt"), content="miss")

t.run(graph)
