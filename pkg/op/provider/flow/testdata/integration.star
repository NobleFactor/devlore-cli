# Integration test for plan Provider receiver.
# plan is injected as a Starlark global bound to an ExecutingReceiver.
# Exercises: complete, degraded, fatal — the three terminal actions
# exposed via the plan Provider's methods.

# --- complete (no output) ---
result_complete = plan.complete()

# --- complete with output ---
result_complete_out = plan.complete(output="done")

# --- degraded with format string ---
result_degraded = plan.degraded("service %s is slow", "auth")

# --- fatal with format string ---
result_fatal = plan.fatal("disk full on %s", "node-1")

# Signal completion.
result_done = True
