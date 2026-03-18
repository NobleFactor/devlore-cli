# Integration test for appnet provider.
# appnet and test_url are injected by the Go test.
# Exercises: download.

result_download = appnet.download(test_url)
result_download_type = type(result_download)

# Signal completion.
result_done = True
