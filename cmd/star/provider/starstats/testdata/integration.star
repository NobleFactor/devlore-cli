# Integration test for starstats provider.
# test_files is injected by the Go test as a list of absolute paths.

st = starstats.compute_stats(test_files, with_bytes=True, with_loc=True)

result_file_count = st.totals.file_count
result_total_bytes = st.totals.total_bytes
result_total_loc = st.totals.total_loc

# Signal completion.
result_done = True
