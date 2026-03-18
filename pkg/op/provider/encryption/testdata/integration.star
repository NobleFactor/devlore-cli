# Integration test for encryption provider.
# encryption, test_source, and test_dest are injected by the Go test.
# Exercises: decrypt_sops_file.

result_decrypt = encryption.decrypt_sops_file(test_source, test_dest)

# Signal completion.
result_done = True
