# test_imm_file.star — Immediate file operations.
#
# Validates: file.join, file.name, file.parent, file.write_text, file.read,
#            file.exists, file.is_file, file.is_dir, file.mkdir, file.copy,
#            file.move, file.remove, file.glob

# Pure path functions — return strings
t.expect_equal(file.join(parts=["a", "b", "c.txt"]), "a/b/c.txt")  # keyword list
t.expect_equal(file.join("a", "b", "c.txt"), "a/b/c.txt")          # positional args
t.expect_equal(file.join("only"), "only")                           # single positional
t.expect_equal(file.join(), "")                                     # empty
t.expect_equal(file.name(path="/some/dir/file.txt"), "file.txt")
t.expect_equal(file.parent(path="/some/dir/file.txt"), "/some/dir")

# Write — returns a Resource (verify callable, not None)
dest = t.tmp("imm_write.txt")
written = file.write_text(destination=dest, content="immediate write", mode=0o644)
t.expect_equal(type(written), "struct")

# Read — returns a Resource (not the string content)
content = file.read(path=dest)
t.expect_equal(type(content), "struct")

# Existence checks — return bools
t.expect_equal(file.exists(resource=dest), True)
t.expect_equal(file.is_file(resource=dest), True)
t.expect_equal(file.is_dir(resource=dest), False)
t.expect_equal(file.exists(resource=t.tmp("no_such_file")), False)

# Mkdir and is_dir
dir = t.tmp("imm_dir")
file.mkdir(resource=dir, mode=0o755)
t.expect_equal(file.is_dir(resource=dir), True)

# Copy — returns a Resource
dst = t.tmp("imm_copy.txt")
copied = file.copy(source_file=dest, destination_filename=dst, destination_file_mode=0o644)
t.expect_equal(type(copied), "struct")

# Move — returns a Resource
moved = t.tmp("imm_moved.txt")
file.move(source=dst, destination=moved)
t.expect_equal(file.exists(resource=dst), False)
t.expect_equal(file.exists(resource=moved), True)

# Remove
file.remove(path=moved, prune=False, prune_boundary="")
t.expect_equal(file.exists(resource=moved), False)

# Glob — returns a list
file.write_text(destination=t.tmp("imm_dir/a.txt"), content="a", mode=0o644)
file.write_text(destination=t.tmp("imm_dir/b.txt"), content="b", mode=0o644)
matches = file.glob(pattern=t.tmp("imm_dir/*.txt"), honor_gitignore=False)
t.expect_equal(len(matches), 2)

# No graph nodes — all immediate
t.expect_node_count(0)
