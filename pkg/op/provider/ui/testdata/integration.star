# Integration test for ui provider.
# Exercises all five messaging methods via the executing receiver.
# UI methods write to the context writer; void methods return None.
# Sets result_* globals for the Go test to inspect.

# --- void methods (write to writer, return None) ---
result_note = ui.note("hello from note")
result_success = ui.success("operation completed")
result_warn = ui.warn("something looks off")
result_error = ui.error("something went wrong")

# note/success/warn/error return None
result_note_is_none = (result_note == None)
result_success_is_none = (result_success == None)
result_warn_is_none = (result_warn == None)
result_error_is_none = (result_error == None)

# Signal completion.
result_done = True
