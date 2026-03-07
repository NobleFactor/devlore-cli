# test_imm_file_join_variadic_error.star — Reject ambiguous variadic call.
#
# Validates: file.join rejects both positional and keyword args for the
# variadic param.

t.expect_error("positional and keyword")
file.join("a", "b", parts=["c", "d"])
