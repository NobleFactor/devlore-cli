# Integration test for template provider.
# Exercises render_text and render_bytes via the executing receiver.

# --- render_text ---
result_text = template.render_text(
    content="Hello {{ .Name }}, project={{ .Project }}",
    data={"Name": "Alice", "Project": "test"},
)

# --- render_bytes ---
result_bytes = template.render_bytes(
    content=b"value={{ .X }}",
    data={"X": "42"},
)

# --- render_text with nil data ---
result_static = template.render_text(content="static", data={})

# Signal completion.
result_done = True
