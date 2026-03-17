# Integration test for archive provider.
# test_archive and test_dest are injected by the Go test.
# Exercises: extract (tar.gz).

result_extract = archive.extract(test_archive, test_dest)

# Signal completion.
result_done = True
