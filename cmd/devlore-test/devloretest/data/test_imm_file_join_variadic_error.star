# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_file_join_variadic_error.star — Reject ambiguous variadic call.
#
# Validates: file.join rejects both positional and keyword args for the
# variadic param. Starlark itself raises the refusal, so the message is its
# wording, not ours.

t.expect_error('multiple values for argument "parts"')
file.join("a", "b", parts=["c", "d"])
