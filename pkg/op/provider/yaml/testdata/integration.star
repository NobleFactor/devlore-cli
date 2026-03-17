# Integration test for yaml provider.
# Exercises encode and decode via the executing receiver.
# Sets result_* globals for the Go test to inspect.

# --- encode ---
encoded = yaml.encode({"name": "alice", "age": 30})
result_encode_type = type(encoded)
result_encode_has_name = "alice" in encoded

# --- decode ---
decoded = yaml.decode("color: blue\ncount: 42\n")
result_decode_color = decoded["color"]
result_decode_count = decoded["count"]

# --- round-trip ---
original = {"key": "value"}
rt = yaml.decode(yaml.encode(original))
result_roundtrip_key = rt["key"]

# Signal completion.
result_done = True
