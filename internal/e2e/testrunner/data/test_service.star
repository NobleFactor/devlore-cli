# test_service.star — Dry-run: service actions create graph nodes.
#
# Validates: plan.service.start, plan.service.stop, plan.service.restart,
#            plan.service.enable, plan.service.disable,
#            plan.service.exists, plan.service.running, plan.service.enabled
#            (registration + node creation)
#
# Each action uses a distinct service name to avoid resource URI conflicts.

plan.service.start(name="svc-start")
plan.service.stop(name="svc-stop")
plan.service.restart(name="svc-restart")
plan.service.enable(name="svc-enable")
plan.service.disable(name="svc-disable")
plan.service.exists(name="svc-exists")
plan.service.running(name="svc-running")
plan.service.enabled(name="svc-enabled")
t.expect_node_count(8)
