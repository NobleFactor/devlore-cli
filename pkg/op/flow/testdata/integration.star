# Integration test for flow Plan receiver.
# flow is injected as a Starlark global bound to a Plan instance.
# Exercises: complete, degraded, fatal — the three terminal actions
# exposed via the Plan receiver's Attr() method.

# --- complete (no output) ---
result_complete = flow.complete()

# --- complete with output ---
result_complete_out = flow.complete(output="done")

# --- degraded with format string ---
result_degraded = flow.degraded("service %s is slow", "auth")

# --- fatal with format string ---
result_fatal = flow.fatal("disk full on %s", "node-1")

# Signal completion.
result_done = True
