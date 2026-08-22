# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_scoped_claims.star — judgment scenario: scoped claims (docs/plans/resource-construction.md).
#
# A choose case claims a file that never exists — and the case is never reached, because its when-predicate
# is falsy. The prediction, authored from the ruled design (2026-08-22): claims verification is per subgraph
# executor, and a choose case is a subgraph, so its claims are judged only when the case is hit. The run
# routes to the default and SUCCEEDS; the unreached case's missing claim is inconsequential — its existence
# was never this run's business. (The reached-direction consequence is pinned by the pre-flight fail-fast
# scenario: a required claim in a scope that DOES start fails that scope with the catalog's verdict.)

phantom = t.tmp("phantom.txt")     # never created — the when-predicate probes it (a path query, no claim)
unclaimed = t.tmp("never_made.txt")  # never created — the unreached case CLAIMS it
status = t.tmp("choose_status.txt")

exists_inv = plan.file.exists(path=phantom)
choice = plan.choose(
    plan.case(when=exists_inv, then=plan.file.read_text(resource=unclaimed)),
    default=lambda: "case not taken",
)
status_inv = plan.file.write_text(destination_path=status, content=choice, mode=0o644)

graph = plan.assemble_definition([choice, status_inv])

# The claim IS in the stored catalog — intent is declared regardless of reachability — as a stateless row.
doc = t.tmp("graph.json")
plan.save_definition(graph, doc)
document = json.decode(data=file.read_text(resource=doc))
entries = document["resources"]
t.expect_equal(len([e for e in entries if "never_made.txt" in str(e)]), 1)

# The run succeeds: the scope that claimed the missing file never started, so the claim was never judged.
t.expect_file(status, content="case not taken")
t.expect_no_file(unclaimed)

t.run(graph)
