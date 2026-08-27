# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui_print_replaces_builtin.star — print() is the ui provider's, not starlark's builtin.
#
# This is the property root placement exists for, and it is invisible at the call site: print("x")
# reads identically whichever implementation answers it, and only the DESTINATION differs — the
# builtin writes straight to stderr through starlark-go, escaping --silent and the narrator.
#
# The arity is what tells them apart. starlark's builtin is variadic — print("a", "b") prints "a b".
# ui.Print takes one string. So a two-argument call MUST fail, and if it ever succeeds the builtin
# is back and every script kept working while its output quietly returned to stderr.

# The message names ui.print, which is a symbol no script can use -- root placement removed the ui
# global. That is #710, and it is pre-existing: flow has reported flow.failed for plan.failed calls
# since root placement existed. Pinned as-is so the fix to #710 has to come back through here.
t.expect_error(r"ui\.print: got 2 arguments, want at most 1")

print("a", "b")
