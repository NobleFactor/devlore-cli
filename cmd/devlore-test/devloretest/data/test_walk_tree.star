# test_walk_tree.star — Immediate mode: walk temp dir, collect relative paths.

# Set up temp directory with files.
dir = t.tmp("walk_root")
file.mkdir(path=dir, chmod=0o755)
file.write_text(destination_path=t.tmp("walk_root/a.txt"), content="a", chmod=0o644)
file.write_text(destination_path=t.tmp("walk_root/b.txt"), content="b", chmod=0o644)

sub = t.tmp("walk_root/sub")
file.mkdir(path=sub, chmod=0o755)
file.write_text(destination_path=t.tmp("walk_root/sub/c.txt"), content="c", chmod=0o644)

# Walk and collect relative paths.
def collector(initial, resource, path, stack):
    if initial == None:
        return [path]
    return initial + [path]

result = file.walk_tree(root=dir, fn=collector, honor_gitignore=False)

# Result is a list; sort to get deterministic order.
# Directories and files are both included.
paths = sorted(result)
t.expect_equal(len(paths), 4)  # a.txt, b.txt, sub, sub/c.txt
t.expect_equal(paths[0], "a.txt")
t.expect_equal(paths[1], "b.txt")
t.expect_equal(paths[2], "sub")
t.expect_equal(paths[3], "sub/c.txt")

# No graph nodes — all immediate.
t.expect_node_count(0)
