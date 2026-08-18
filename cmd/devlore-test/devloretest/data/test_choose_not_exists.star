# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_choose_not_exists.star — plan.choose routes to the default when no case matches.
#
# Check existence of a file that was never created (the when-body evaluates falsy). The decision tree's falsy edge
# lands on the default subgraph; its result is the choose result. Capture it in a downstream write_text to assert.

phantom = t.tmp("phantom.txt")
status  = t.tmp("choose_status.txt")

exists_inv = plan.file.exists(path=phantom)
choice     = plan.choose(plan.case(when=exists_inv, then=lambda: "found"), default=lambda: "missing")
status_inv = plan.file.write_text(destination_path=status, content=choice, mode=0o644)

graph = plan.assemble_definition([choice, status_inv])

t.expect_no_file(phantom)
t.expect_file(status, content="missing")
t.expect_unit_count(8)  # nodes: exists + then-call + default-call + status_write; subgraphs: choose + when + then + default

t.run(graph)
