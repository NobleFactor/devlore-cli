# test_imm_starindex.star — Immediate Starlark file indexing.
#
# Validates: starindex.index_files (callable with empty input)

result = starindex.index_files(files=[], with_docstrings=True, with_globals=True)
t.expect_node_count(0)
