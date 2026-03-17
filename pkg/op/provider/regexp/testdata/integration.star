# Integration test for regexp provider.
# Exercises all eight methods via the executing receiver.
# Sets result_* globals for the Go test to inspect.

text = "The quick brown fox jumps over 42 lazy dogs at 3pm"

# --- match ---
result_match_true = regexp.match(r"\d+", text)
result_match_false = regexp.match(r"^zzz", text)

# --- find ---
result_find_first = regexp.find(r"\d+", text)
result_find_none = regexp.find(r"zzz", text)

# --- find_all ---
result_find_all = regexp.find_all(r"\d+", text, -1)
result_find_all_count = len(result_find_all)

# --- find_submatch ---
result_find_submatch = regexp.find_submatch(r"(\d+)\s+(\w+)", text)
result_submatch_full = result_find_submatch[0]
result_submatch_group1 = result_find_submatch[1]
result_submatch_group2 = result_find_submatch[2]

# --- find_all_submatch ---
result_find_all_submatch = regexp.find_all_submatch(r"(\d+)", text, -1)
result_all_submatch_count = len(result_find_all_submatch)

# --- replace ---
result_replace = regexp.replace(r"\d+", text, "NUM")

# --- replace_literal ---
result_replace_literal = regexp.replace_literal(r"\d+", "a1b2c3", "X")

# --- split ---
result_split = regexp.split(r"\s+", "one two three", -1)
result_split_count = len(result_split)

# Signal completion.
result_done = True
