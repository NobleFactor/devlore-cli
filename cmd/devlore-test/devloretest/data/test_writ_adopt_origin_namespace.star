# test_writ_adopt_origin_namespace.star — Variable binding: origin namespace assertion.
#
# Supplies a parameter via t.set_flags so the resolver records NamespaceFlag for that binding. Asserts the
# namespace alone (loose form) via t.expect_variable_namespace.
#
# 13.0(n) Phase 1: contract documentation only — Phase 2 (real resolver) populates origins; Phase 4 wires
# the Go entry point.

dest_dir = t.tmp("adopt-dest-origin-ns")

t.set_flags({"dest_dir": dest_dir})

graph = plan.assemble_definition([
    plan.file.mkdir(path=plan.variable("dest_dir"), chmod=0o755),
])

# Phase 4+ assertions:
#   t.expect_variable_namespace("dest_dir", "flag")
#   t.expect_file(dest_dir + "/.keep") or similar positive side effect (not yet meaningful — Phase 1)

t.run(graph)
