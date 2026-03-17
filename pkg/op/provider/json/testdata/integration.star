# Integration test for json provider.
# Exercises encode, encode_indent, and decode via the executing receiver.
# Sets result_* globals for the Go test to inspect.

# --- encode ---
encoded = json.encode({"name": "alice", "age": 30})
result_encode_type = type(encoded)
result_encode_has_name = "alice" in encoded

# --- decode ---
decoded = json.decode('{"color":"blue","count":42}')
result_decode_color = decoded["color"]
result_decode_count = decoded["count"]

# --- encode_indent ---
indented = json.encode_indent({"x": 1}, "  ")
result_indent_type = type(indented)
result_indent_has_newline = "\n" in indented

# --- round-trip ---
original = {"key": "value", "list": [1, 2, 3]}
rt = json.decode(json.encode(original))
result_roundtrip_key = rt["key"]
result_roundtrip_list_len = len(rt["list"])

# Signal completion.
result_done = True
