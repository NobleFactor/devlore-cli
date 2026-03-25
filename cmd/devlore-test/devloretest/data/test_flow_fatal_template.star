# test_flow_fatal_template.star — Verify fatal with promise arg.
# The write_text result flows into the fatal message via a promise.
# TODO: restore kwargs (service=svc) after Phase 1.50 adds **kwargs bridge support.

t.expect_error("fatal:.*startup failed")
svc = plan.file.write_text(destination=t.tmp("svc.txt"), content="myapp", mode=0o644)
plan.fatal("%s startup failed", svc)
