# Integration test for starindex provider.
# test_files is injected by the Go test as a list of absolute paths.

idx = starindex.index_files(test_files, with_docstrings=True, with_globals=True)

result_file_count = idx.totals.file_count
result_functions = idx.totals.functions
result_globals = idx.totals.globals

# Signal completion.
result_done = True
