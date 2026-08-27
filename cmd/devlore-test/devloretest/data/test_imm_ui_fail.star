# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui_fail.star — fail() is the ui provider's, and it still halts the script.
#
# Excluded from test_imm_ui.star precisely because it terminates execution, so it needs a fixture of
# its own. ui.Fail emits the fatal-form message and returns a non-nil error, which the bridge raises
# — the same observable behavior as starlark's builtin fail, now routed through the narrator.

t.expect_error("halt and catch fire")

fail("halt and catch fire")
