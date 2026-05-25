# test_writ_adopt_precedence.star — Variable binding: source precedence.
#
# Supplies the same parameter name from multiple sources to assert the resolver's precedence chain:
# Override > Flag > Env > Config > Default. The override value should win.
#
# 13.0(n) Phase 1: contract documentation only — Phase 2 (real resolver) implements precedence; Phase 4
# wires the Go entry point.

t.set_overrides({"layer": "override-value"})
t.set_flags({"layer": "flag-value"})
t.set_env_prefix("DEVLORE_TEST")  # DEVLORE_TEST_LAYER would also be set externally
t.set_config({"layer": "config-value"})

_ = plan.variable("layer", default_value="default-value")  # declare the parameter; reference unused except for binding

graph = plan.assemble([
    plan.file.mkdir(path=t.tmp("precedence-dest"), chmod=0o755),
])

# Phase 4+ assertion:
#   t.expect_variable("layer", value="override-value", origin="override:layer")

t.run(graph)
