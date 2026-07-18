# test_choose_exists.star — plan.choose routes to the matched case's then-subgraph.
#
# Write a file, check its existence inside the case's when-body (truthy), and use plan.choose to route between the
# then-body ("found") and the default body ("missing"). The choose result is the executed leaf's result; capture it in
# a downstream write_text to assert the value flowed through.
#
# Decision-tree surface (phase-8 step 10): when= is a body — here a singleton invocation — adopted as the when-subgraph;
# then= / default= are lambda bodies desugared to function.call leaves.

dest   = t.tmp("choose_target.txt")
status = t.tmp("choose_status.txt")

written    = plan.file.write_text(destination_path=dest, content="here", chmod=0o644)
exists_inv = plan.file.exists(path=dest)
choice     = plan.choose(plan.case(when=exists_inv, then=lambda: "found"), default=lambda: "missing")
status_inv = plan.file.write_text(destination_path=status, content=choice, chmod=0o644)

graph = plan.assemble_definition([written, choice, status_inv])

t.expect_file(status, content="found")
t.expect_unit_count(9)  # nodes: written + exists + then-call + default-call + status_write; subgraphs: choose + when + then + default

t.run(graph)
