# test_writ_adopt_subgraph.star — Variable binding: subgraph bubble-up surface.
#
# Wraps the mkdir → move → link sequence in plan.subgraph(...). The subgraph's Parameters() should expose
# the deduplicated union of its topological roots' parameter surfaces, and variables supplied at the root
# graph level should flow through to the nested nodes.
#
# 13.0(n) Phase 1: contract documentation only — Phase 3 implements ExecutableUnit.Parameters() bubble-up;
# Phase 4 wires the Go entry point.

src_dir   = t.tmp("adopt-src-sub")
dest_dir  = t.tmp("adopt-dest-sub")
src_path  = t.tmp("adopt-src-sub/file.txt")
dest_path = t.tmp("adopt-dest-sub/file.txt")

t.mkdir(src_dir)
t.write(src_path, "subgraph adopted content")

t.set_flags({
    "dest_dir":    dest_dir,
    "source_path": src_path,
    "dest_path":   dest_path,
})

# Phase 8 (writ adopt migration) will use this exact subgraph shape inside the writ-side helper. The
# Phase 1 plan-doc captures the intent here so the bubble-up contract is exercised at the .star level.
sg = plan.subgraph(body=[
    plan.file.mkdir(path=plan.variable("dest_dir"), chmod=0o755),
    plan.file.move(source=plan.variable("source_path"), destination_path=plan.variable("dest_path")),
    plan.file.link(source=plan.variable("dest_path"), target_path=plan.variable("source_path")),
])

graph = plan.assemble([sg])

# Phase 4+ assertions:
#   t.expect_variable_namespace("dest_dir", "flag")
#   t.expect_variable_namespace("source_path", "flag")
#   t.expect_variable_namespace("dest_path", "flag")
#   t.expect_file(dest_path, content="subgraph adopted content")
#   t.expect_no_file(src_path)

t.run(graph)
