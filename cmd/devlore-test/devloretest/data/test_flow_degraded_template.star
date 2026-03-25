# test_flow_degraded_template.star — Verify degraded with template kwargs.
# The write_text result flows into the degraded template via a promise.
# Graph should still complete successfully.

written = plan.file.write_text(destination=t.tmp("d.txt"), content="ok", mode=0o644)
plan.degraded("wrote {{ .path }}", path=written)

t.expect_node_count(2)
