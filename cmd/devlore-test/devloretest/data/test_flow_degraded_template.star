# test_flow_degraded_template.star — Verify degraded with promise arg.
# The write_text result flows into the degraded message via a promise.
# Graph should still complete successfully.
# TODO: restore kwargs (path=written) after Phase 1.50 adds **kwargs bridge support.

written = plan.file.write_text(destination=t.tmp("d.txt"), content="ok", mode=0o644)
plan.degraded("wrote %s", written)

t.expect_node_count(2)
