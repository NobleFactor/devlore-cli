# Starlark integration test for file provider immediate bindings.
#
# Globals injected by Go harness:
#   file      — FileReceiver wrapping a Provider rooted at tmp_dir
#   tmp_dir   — absolute path to a temp directory
#   fixture   — absolute path to a pre-created file (contains "fixture content")
#   sep       — OS path separator ("/" or "\")

# ── Pure functions ──────────────────────────────────────────────────────────────

# join
joined = file.join(["a", "b", "c"])
result_join = sep.join(["a", "b", "c"]) == joined

# join single
result_join_single = file.join(["only"]) == "only"

# name
result_name = file.name(path=tmp_dir + sep + "foo" + sep + "bar.txt") == "bar.txt"

# parent
result_parent = file.parent(path=tmp_dir + sep + "child.txt") == tmp_dir

# exists — file
result_exists_file = file.exists(resource=fixture)

# exists — missing
result_exists_missing = file.exists(resource=tmp_dir + sep + "nonexistent") == False

# is_dir — directory
result_is_dir_true = file.is_dir(resource=tmp_dir)

# is_dir — file
result_is_dir_false = file.is_dir(resource=fixture) == False

# is_file — file
result_is_file_true = file.is_file(resource=fixture)

# is_file — directory
result_is_file_false = file.is_file(resource=tmp_dir) == False

# mkdir
mkdir_path = file.mkdir(resource=tmp_dir + sep + "new_subdir", mode=0o755)
result_mkdir = file.is_dir(resource=mkdir_path)

# mkdir nested
nested_path = file.mkdir(resource=tmp_dir + sep + "a" + sep + "b" + sep + "c", mode=0o755)
result_mkdir_nested = file.is_dir(resource=nested_path)

# glob
glob_results = file.glob(pattern=tmp_dir + sep + "*.txt", honor_gitignore=False)
result_glob_count = len(glob_results)

# read — returns a blob struct with source_path and size
read_result = file.read(path=fixture)
result_read_has_path = hasattr(read_result, "source_path")

# ── Compensable (no recovery needed) ────────────────────────────────────────────

# write_text
wt_path = file.write_text(destination=tmp_dir + sep + "written.txt", content="hello starlark", mode=0o644)
result_write_text = file.exists(resource=wt_path)

# write_bytes
wb_path = file.write_bytes(destination=tmp_dir + sep + "written.bin", content="binary data", mode=0o600)
result_write_bytes = file.exists(resource=wb_path)

# write_text creates parent directories
wt_nested = file.write_text(
    destination=tmp_dir + sep + "nested" + sep + "dir" + sep + "file.txt",
    content="nested",
    mode=0o644,
)
result_write_text_nested = file.exists(resource=wt_nested)

# link
link_path = file.link(source=fixture, path=tmp_dir + sep + "symlink.txt")
result_link = file.exists(resource=link_path)

# move
move_src = tmp_dir + sep + "to_move.txt"
file.write_text(destination=move_src, content="moveme", mode=0o644)
move_dest = tmp_dir + sep + "moved.txt"
move_result = file.move(source=move_src, destination=move_dest)
result_move_dest_exists = file.exists(resource=move_dest)
result_move_src_gone = file.exists(resource=move_src) == False

# backup
backup_src = tmp_dir + sep + "to_backup.txt"
file.write_text(destination=backup_src, content="backupme", mode=0o644)
backup_path = file.backup(path=backup_src, backup_suffix=".bak")
result_backup_created = file.exists(resource=backup_path)
result_backup_src_gone = file.exists(resource=backup_src) == False

# copy
copy_dest = tmp_dir + sep + "copied.txt"
copy_result = file.copy(source_file=fixture, destination_filename=copy_dest, destination_file_mode=0o644)
result_copy = file.exists(resource=copy_dest)

# ── walk_tree ───────────────────────────────────────────────────────────────────

# Simple walk — just visit without error
# Note: walk_tree passes initial=<previous_return> as a kwarg on every call after the first,
# so all callbacks must accept the initial parameter even if unused.
def visitor(path, entry, initial=None):
    return None

file.walk_tree(root=tmp_dir, fn=visitor, honor_gitignore=False)
result_walk_simple = True

# Fold — count files using accumulator
def count_files(path, entry, initial=0):
    if not entry.is_dir:
        return initial + 1
    return initial

file_count = file.walk_tree(root=tmp_dir, fn=count_files, honor_gitignore=False)
result_walk_file_count = file_count

# Walk — verify DirEntryHandle attributes
dir_names = []
file_names = []

def collect_names(path, entry, initial=None):
    if entry.is_dir:
        dir_names.append(entry.name)
    else:
        file_names.append(entry.name)
    return None

file.walk_tree(root=tmp_dir, fn=collect_names, honor_gitignore=False)
result_walk_has_dirs = len(dir_names) > 0
result_walk_has_files = len(file_names) > 0

# ── Blocked on recovery (#164) ──────────────────────────────────────────────────
# remove, remove_all, unlink — tested in Go-level provider_test.go

result_done = True
