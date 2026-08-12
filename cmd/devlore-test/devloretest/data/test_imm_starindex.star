# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_starindex.star — Immediate Starlark file indexing.
#
# Validates: starindex.index_files (callable with empty input)

result = starindex.index_files(files=[], with_docstrings=True, with_globals=True)
t.expect_unit_count(0)
