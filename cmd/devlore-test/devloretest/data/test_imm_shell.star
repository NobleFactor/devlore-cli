# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_shell.star — Immediate shell execution, and the record it returns.
#
# Validates: shell.exec (immediate mode), and the automatic conversion of a provider's returned struct.
#
# shell.exec returns a *shell.Result, and every exported field is reachable from Starlark. Nothing declares
# that surface: the bridge resolves a receiver type by reflection when none is registered, so the fields
# appear with no directive, no registration, and no generated file. Go names are rendered in the config
# vocabulary, which is why ExitCode reads as exit_code.
#
# This file previously asserted `shell.exec(...) == "echo hello"` under the comment "returns the command
# string, not stdout". Both were wrong, and the assertion failed whenever anything ran it.

result = shell.exec(command="echo hello")

t.expect_equal(result.command, "echo hello")
t.expect_equal(result.stdout, "hello\n")
t.expect_equal(result.stderr, "")
t.expect_equal(result.exit_code, 0)

# The value is the record, not any one of its fields: it renders as its type, and str() of it is not stdout.
t.expect_equal(type(result), "Result")
t.expect_equal("%s" % result, "Result")

# stderr is captured separately from stdout rather than interleaved, and a command may write to it while
# still succeeding.
noisy = shell.exec(command="echo out; echo err >&2")

t.expect_equal(noisy.stdout, "out\n")
t.expect_equal(noisy.stderr, "err\n")
t.expect_equal(noisy.exit_code, 0)

# A non-zero exit is not observable here: shell.exec is a fallible action, so it reports failure as an error
# rather than returning a Result with a non-zero exit_code.

t.expect_unit_count(0)
