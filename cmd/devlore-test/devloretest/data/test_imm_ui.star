# test_imm_ui.star — Immediate UI output functions.
#
# Validates: ui.note, ui.warn, ui.succeed, ui.error
# ui.fail is excluded — it terminates script execution by design.

ui.note("test note")
ui.warn("test warning")
ui.succeed("test success")
ui.error("test error")

t.expect_unit_count(0)
