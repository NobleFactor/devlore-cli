# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui.star — Immediate UI output functions.
#
# Validates: note, warn, succeed, error
# fail is excluded — it terminates script execution by design.

note("test note")
warn("test warning")
succeed("test success")
error("test error")

t.expect_unit_count(0)
