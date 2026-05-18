# test_round_trip_writ_adopt.star — Wire-format: graph save / load round-trip.
#
# Wraps the mkdir → move → link sequence in plan.subgraph(...) and uses plan.assemble to materialize the
# Graph, then plan.save / plan.load to round-trip it through YAML on disk. Asserts structural equivalence
# via t.expect_graph_equal. Serialization-only — no actions execute.
#
# 13.0(n) Phase 1: contract documentation only. The body builds the in-bridge invocation registry today.
# Phase 5 lands plan.assemble / plan.save / plan.load and the harness gains t.expect_graph_equal; at that
# point the assertions at the bottom of this file go live.

src_dir   = t.tmp("rt-src")
dest_dir  = t.tmp("rt-dest")
src_path  = t.tmp("rt-src/file.txt")
dest_path = t.tmp("rt-dest/file.txt")

t.mkdir(src_dir)
t.write(src_path, "round-trip content")

t.set_flags({
    "dest_dir":    dest_dir,
    "source_path": src_path,
    "dest_path":   dest_path,
})

plan.subgraph(body=[
    plan.file.mkdir(path=plan.variable("dest_dir"), chmod=0o755),
    plan.file.move(source=plan.variable("source_path"), destination_path=plan.variable("dest_path")),
    plan.file.link(source=plan.variable("dest_path"), target_path=plan.variable("source_path")),
])

# Phase 5+ assertions (when plan.assemble / plan.save / plan.load / t.expect_graph_equal land):
#   graph_path = t.tmp("graph.yaml")
#   original = plan.assemble()
#   plan.save(original, graph_path)
#   loaded = plan.load(graph_path)
#   t.expect_graph_equal(original, loaded)
