# Integration test for starcomplexity provider.
# test_files is injected by the Go test as a list of absolute paths.

report = starcomplexity.compute_complexity(test_files)

result_file_count = len(report.files)

# Signal completion.
result_done = True
