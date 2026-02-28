# Starlark integration test for file provider planned bindings.
#
# Planned bindings build an execution graph — they do NOT execute immediately.
# Each call creates a Node with filled slots and returns an Output (promise).
#
# Globals injected by Go harness:
#   file      — FilePlan wrapping a Graph, project "test-project", and ActionRegistry

# ── Compensable actions ─────────────────────────────────────────────────────────

# write_text — creates a node with destination/content/mode slots
wt_output = file.write_text(destination="/tmp/planned.txt", content="hello planned", mode=0o644)
result_write_text_type = type(wt_output) == "Output"

# write_bytes — creates a node with destination/content/mode slots
wb_output = file.write_bytes(destination="/tmp/planned.bin", content="binary", mode=0o600)
result_write_bytes_type = type(wb_output) == "Output"

# link — creates a node with source/path slots
link_output = file.link(source="/tmp/source.txt", path="/tmp/link.txt")
result_link_type = type(link_output) == "Output"

# move — creates a node with source/destination slots
move_output = file.move(source="/tmp/from.txt", destination="/tmp/to.txt")
result_move_type = type(move_output) == "Output"

# backup — creates a node with path/backup_suffix slots
backup_output = file.backup(path="/tmp/backup-target.txt", backup_suffix=".bak")
result_backup_type = type(backup_output) == "Output"

# remove — creates a node with path/prune/prune_boundary slots
remove_output = file.remove(path="/tmp/to-remove.txt", prune=False, prune_boundary="")
result_remove_type = type(remove_output) == "Output"

# remove_all — creates a node with path/prune/prune_boundary slots
remove_all_output = file.remove_all(path="/tmp/to-remove-all", prune=True, prune_boundary="/tmp")
result_remove_all_type = type(remove_all_output) == "Output"

# unlink — creates a node with path/prune/prune_boundary slots
unlink_output = file.unlink(path="/tmp/to-unlink.txt", prune=False, prune_boundary="")
result_unlink_type = type(unlink_output) == "Output"

# copy — creates a node with destination/source/mode slots
copy_output = file.copy(destination="/tmp/copy-target.txt", source="/tmp/source.txt", mode=0o644)
result_copy_type = type(copy_output) == "Output"

# ── Non-compensable actions ─────────────────────────────────────────────────────

# mkdir — creates a node with path/mode slots
mkdir_output = file.mkdir(path="/tmp/new-dir", mode=0o755)
result_mkdir_type = type(mkdir_output) == "Output"

# glob — creates a node with pattern/honor_gitignore slots
glob_output = file.glob(pattern="/tmp/*.txt", honor_gitignore=False)
result_glob_type = type(glob_output) == "Output"

# read — creates a node with path slot
read_output = file.read(path="/tmp/read-target.txt")
result_read_type = type(read_output) == "Output"

# exists — creates a node with path slot
exists_output = file.exists(path="/tmp/exists-check.txt")
result_exists_type = type(exists_output) == "Output"

# is_dir — creates a node with path slot
is_dir_output = file.is_dir(path="/tmp")
result_is_dir_type = type(is_dir_output) == "Output"

# is_file — creates a node with path slot
is_file_output = file.is_file(path="/tmp/check.txt")
result_is_file_type = type(is_file_output) == "Output"

# name — creates a node with path slot
name_output = file.name(path="/tmp/foo/bar.txt")
result_name_type = type(name_output) == "Output"

# parent — creates a node with path slot
parent_output = file.parent(path="/tmp/foo/bar.txt")
result_parent_type = type(parent_output) == "Output"

# ── Promise chaining (edge creation) ───────────────────────────────────────────

# Chain: write_text output → backup input (creates an edge in the graph)
chain_write = file.write_text(destination="/tmp/chain.txt", content="chain me", mode=0o644)
chain_backup = file.backup(path=chain_write, backup_suffix=".chain")
result_chain_done = True

# Chain: write_text output → move source (creates another edge)
chain_src = file.write_text(destination="/tmp/move-src.txt", content="move me", mode=0o644)
chain_move = file.move(source=chain_src, destination="/tmp/move-dst.txt")
result_chain_move_done = True

# ── Output attributes ──────────────────────────────────────────────────────────

# Output has node_id attribute
result_output_has_node_id = hasattr(wt_output, "node_id")

result_done = True
