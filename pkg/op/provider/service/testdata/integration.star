# Integration test for service provider.
# service is injected by the Go test with a mock ServiceManager.
# Exercises: exists, running, enabled.

result_exists = service.exists("sshd")
result_running = service.running("sshd")
result_enabled = service.enabled("sshd")

result_not_exists = service.exists("nonexistent-svc") == False

# Signal completion.
result_done = True
