# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui_print_replaces_builtin.star — print() is the ui provider's, not starlark's builtin.
#
# This is the property root placement exists for, and it is invisible at the call site: print("x")
# reads identically whichever implementation answers it, and only the DESTINATION differs — the
# builtin writes straight to stderr through starlark-go, escaping --silent and the narrator.
#
# The arity is what tells them apart. starlark's builtin is variadic; ui.Print takes one string. So a
# two-argument call MUST fail, and if it ever succeeds the builtin is back and every script kept
# working while its output quietly returned to stderr.
#
# The message names print, which is what the author typed. It said ui.print until #710 — a symbol no
# script can use, since promotion means ui is not a global at all, so an author following the error
# arrived at `undefined: ui`.
#
# The rule: where a call site exists, an error names what the author typed; where none exists, it
# names the action. This is script evaluation, so the call site is right here. A graph node's action
# name stays provider-qualified, because it is the node's identity in a serialized workflow.

t.expect_error(r"^print: got 2 arguments, want at most 1")

print("a", "b")
