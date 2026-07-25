# test_gather_concurrency.star — limit semantics + iteration-isolation coverage for plan.gather.
#
# Matrix rows (PowerShell ForEach-Object -ThrottleLimit analogues):
#   C1: limit=1                 — serial execution; every item still completes.
#   C2: limit=N (N>1)           — parallel up to N; every item still completes.
#   C3: limit > items count     — caps at items.len, no excess goroutines, all complete.
#   C4: limit=0                 — falls through to Platform.DefaultConcurrency(); all complete.
#   C5: items > limit × 2       — multiple parallel batches; ordering not assumed, all complete.
#   C6: per-iteration isolation — each iteration writes its own `item` value (no inter-iteration aliasing).
#
# Each row asserts via per-item file existence: per-iteration isolation is verified by every output
# file matching the path derived from its source item. A race-aliased iteration would clobber another
# item's destination_path, leaving at least one expected file missing.

# region C1: limit=1 — serial execution

c1_paths = [t.tmp("c1_%d.txt" % i) for i in range(3)]
c1_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="serial", chmod=0o644)
c1 = plan.gather(items=c1_paths, limit=1, body=[c1_inv])

# endregion

# region C2: limit=N (N>1) — parallel up to N

c2_paths = [t.tmp("c2_%d.txt" % i) for i in range(4)]
c2_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="par_n", chmod=0o644)
c2 = plan.gather(items=c2_paths, limit=2, body=[c2_inv])

# endregion

# region C3: limit > items count — caps at items.len

c3_paths = [t.tmp("c3_%d.txt" % i) for i in range(2)]
c3_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="over_lim", chmod=0o644)
c3 = plan.gather(items=c3_paths, limit=100, body=[c3_inv])

# endregion

# region C4: limit=0 — falls through to platform default

c4_paths = [t.tmp("c4_%d.txt" % i) for i in range(3)]
c4_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="dflt", chmod=0o644)
c4 = plan.gather(items=c4_paths, limit=0, body=[c4_inv])

# endregion

# region C5: items > limit × 2 — multiple parallel batches

c5_paths = [t.tmp("c5_%d.txt" % i) for i in range(10)]
c5_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="batches", chmod=0o644)
c5 = plan.gather(items=c5_paths, limit=3, body=[c5_inv])

# endregion

# region C6: per-iteration isolation — each iteration writes its own item value

c6_paths = [t.tmp("c6_%d.txt" % i) for i in range(6)]
c6_inv = plan.file.write_text(destination_path=plan.variable("item", default_value=None), content="isolated", chmod=0o644)
c6 = plan.gather(items=c6_paths, limit=4, body=[c6_inv])

# endregion

graph = plan.assemble_definition([c1, c2, c3, c4, c5, c6])

for p in c1_paths:
    t.expect_file(p, content="serial")

for p in c2_paths:
    t.expect_file(p, content="par_n")

for p in c3_paths:
    t.expect_file(p, content="over_lim")

for p in c4_paths:
    t.expect_file(p, content="dflt")

for p in c5_paths:
    t.expect_file(p, content="batches")

for p in c6_paths:
    t.expect_file(p, content="isolated")

t.run(graph)
