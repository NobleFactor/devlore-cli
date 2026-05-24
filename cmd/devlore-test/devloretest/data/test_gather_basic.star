# test_gather_basic.star — input / body-shape / ordering coverage for plan.gather.
#
# Matrix rows (basic surface — PowerShell ForEach-Object analogues):
#   B1: single item — body runs exactly once.
#   B2: multi-item   — body runs once per item.
#   B3: many items   — items count exceeds limit, all dispatched eventually.
#   B4: empty items  — body runs zero times; no error.
#   B5: item binding — plan.variable("item") resolves per iteration.
#   B6: zero-child body — body=[] is a no-op iteration.
#
# Each iteration writes a per-iteration file using plan.variable("item") so each row
# can be verified by file existence on the path it minted. The body=[...] list adopts
# each invocation's subgraph as a child of the gather subgraph; per-iteration dispatch
# walks those children with a per-iteration frame containing the current `item` binding.

# region B1: single-item gather — body runs exactly once

b1_out = t.tmp("b1.txt")
b1_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="alpha", chmod=0o644)
b1 = plan.gather(items=[b1_out], limit=1, body=[b1_inv])

# endregion

# region B2: multi-item gather — body runs once per item

b2_a = t.tmp("b2_a.txt")
b2_b = t.tmp("b2_b.txt")
b2_c = t.tmp("b2_c.txt")
b2_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="bravo", chmod=0o644)
b2 = plan.gather(items=[b2_a, b2_b, b2_c], limit=2, body=[b2_inv])

# endregion

# region B3: many-items gather — items > limit, all dispatched eventually

b3_paths = [t.tmp("b3_%d.txt" % i) for i in range(5)]
b3_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="charlie", chmod=0o644)
b3 = plan.gather(items=b3_paths, limit=3, body=[b3_inv])

# endregion

# region B4: empty items — body runs zero times, no error

b4_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="never", chmod=0o644)
b4 = plan.gather(items=[], limit=4, body=[b4_inv])
b4_canary = t.tmp("b4_canary.txt")
b4_canary_inv = plan.file.write_text(destination_path=b4_canary, content="reached", chmod=0o644)

# endregion

# region B5: item binding — plan.variable("item") resolves per iteration

b5_paths = [t.tmp("b5_%s.txt" % name) for name in ["alpha", "bravo", "charlie"]]
b5_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="delta", chmod=0o644)
b5 = plan.gather(items=b5_paths, limit=2, body=[b5_inv])

# endregion

# region B6: zero-child body — body=[] is a no-op iteration

b6 = plan.gather(items=["ignored-1", "ignored-2"], limit=2, body=[])
b6_canary = t.tmp("b6_canary.txt")
b6_canary_inv = plan.file.write_text(destination_path=b6_canary, content="reached", chmod=0o644)

# endregion

graph = plan.assemble([b1, b2, b3, b4, b4_canary_inv, b5, b6, b6_canary_inv])

t.expect_file(b1_out, content="alpha")

t.expect_file(b2_a, content="bravo")
t.expect_file(b2_b, content="bravo")
t.expect_file(b2_c, content="bravo")

for p in b3_paths:
    t.expect_file(p, content="charlie")

t.expect_file(b4_canary, content="reached")

for p in b5_paths:
    t.expect_file(p, content="delta")

t.expect_file(b6_canary, content="reached")
