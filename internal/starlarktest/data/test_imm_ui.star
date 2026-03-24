# test_imm_ui.star — Immediate UI output functions.
#
# Validates: ui.note, ui.warn, ui.success, ui.error
# ui.fail is excluded — it terminates script execution by design.

ui.note("test note")
ui.warn("test warning")
ui.success("test success")
ui.error("test error")

t.expect_node_count(0)
