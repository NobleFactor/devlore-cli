# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui.star — Immediate UI output functions.
#
# Validates: error, note, print, succeed, warn
#
# print is here because root placement made it ours: it resolves to ui.Print rather than starlark's
# builtin. fail is excluded — it terminates script execution by design, so it has its own fixture.

note("test note")
print("test print")
warn("test warning")
succeed("test success")
error("test error")

t.expect_unit_count(0)
