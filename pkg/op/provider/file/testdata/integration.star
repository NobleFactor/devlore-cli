# Integration test for file provider.
# test_dir is injected by the Go test as the temp directory path.
# Exercises: write_text, read_text, exists, is_file, is_dir, mkdir,
#            join, name, parent, root, copy, link, remove.

# --- path utilities (pure, no I/O) ---
result_join = file.join(test_dir, "sub", "file.txt")
result_name = file.name("/a/b/c.txt")
result_parent = file.parent("/a/b/c.txt")
result_root = file.root

# --- mkdir ---
sub = file.join(test_dir, "subdir")
file.mkdir(sub, 0o755)
result_is_dir = file.is_dir(sub)

# --- write_text + read_text ---
txt_path = file.join(test_dir, "hello.txt")
file.write_text(txt_path, "hello world", 0o644)
result_exists = file.exists(txt_path)
result_is_file = file.is_file(txt_path)
result_read = file.read_text(txt_path)

# --- copy ---
copy_dest = file.join(test_dir, "hello_copy.txt")
file.copy(txt_path, copy_dest, 0o644)
result_copy_exists = file.exists(copy_dest)
result_copy_read = file.read_text(copy_dest)

# --- link ---
link_target = file.join(test_dir, "hello_link.txt")
file.link(txt_path, link_target)
result_link_exists = file.exists(link_target)

# --- remove ---
file.remove(copy_dest, False, test_dir)
result_removed = file.exists(copy_dest) == False

# --- defaults: write_text without mode ---
defaults_txt = file.join(test_dir, "defaults.txt")
file.write_text(defaults_txt, "default mode")
result_defaults_write = file.read_text(defaults_txt)

# --- defaults: mkdir without mode ---
defaults_dir = file.join(test_dir, "defaults_dir")
file.mkdir(defaults_dir)
result_defaults_mkdir = file.is_dir(defaults_dir)

# --- defaults: glob without include_gitignored ---
result_defaults_glob = file.glob(file.join(test_dir, "*.txt"))

# --- defaults: remove without prune/boundary ---
file.remove(defaults_txt)
result_defaults_remove = file.exists(defaults_txt) == False

# --- find: recursive ** pattern ---
# Create nested files for find to discover.
nested = file.join(test_dir, "findtest", "deep")
file.mkdir(nested)
file.write_text(file.join(test_dir, "findtest", "top.go"), "package top")
file.write_text(file.join(nested, "deep.go"), "package deep")
file.write_text(file.join(nested, "notes.md"), "# notes")

result_find_go = file.find(file.join(test_dir, "findtest", "**", "*.go"))
result_find_md = file.find(file.join(test_dir, "findtest", "**", "*.md"))
result_find_all = file.find(file.join(test_dir, "findtest", "**", "*.*"))

# Signal completion.
result_done = True
