# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_preflight_fail_fast.star — judgment scenario: pre-flight fail-fast (docs/plans/resource-construction.md).
#
# A graph claims an existing file as pending intent; the file is deleted AFTER planning, BEFORE the run. The
# prediction, authored from the ruled design (Q1, 2026-08-20): pre-flight's verification pass finds the
# pending file resource missing relative to the run's fsroot — unmet intent — and FAILS THE RUN before any
# unit dispatches; the error is the catalog's verdict ("verify existence"), not a rediscovered copy error,
# and the destination is never created. Phase 3's scoped pre-flight delivered the
# consequence (2026-08-22); this scenario un-skipped as the phase's acceptance evidence.

src = t.tmp("vanishes.txt")
dst = t.tmp("never.txt")
doc = t.tmp("graph.json")

t.write(src, "gone before the run")

copied = plan.file.copy(source=src, destination_path=dst, mode=0o600)
graph = plan.assemble_definition([copied])
plan.save_definition(graph, doc)

# The claim is intent: exactly one pending entry for the source.
document = json.decode(data=file.read_text(resource=doc))
entries = document["resources"]
t.expect_equal(len([e for e in entries if "vanishes.txt" in str(e)]), 1)
t.expect_equal(len([e for e in entries if "state" in e]), 0)  # stateless rows: presence IS the pending claim

# Intent breaks before the run starts.
file.remove(target=src, prune=False, boundary="")

t.expect_error("verify existence")
t.expect_no_file(dst)

t.run(graph)
