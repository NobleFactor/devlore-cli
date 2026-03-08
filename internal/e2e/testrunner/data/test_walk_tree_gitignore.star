# test_walk_tree_gitignore.star — WalkTree with .gitignore filtering.
#
# Creates a temp directory with a .gitignore that excludes *.log files,
# then walks with honor_gitignore=True to verify filtering.

dir = t.tmp("walk_gi")
file.mkdir(resource=dir, mode=0o755)

# Write .gitignore that excludes .log files.
file.write_text(destination=t.tmp("walk_gi/.gitignore"), content="*.log\n", mode=0o644)

# Write a mix of included and excluded files.
file.write_text(destination=t.tmp("walk_gi/keep.txt"), content="keep", mode=0o644)
file.write_text(destination=t.tmp("walk_gi/skip.log"), content="skip", mode=0o644)
file.write_text(destination=t.tmp("walk_gi/also_keep.md"), content="md", mode=0o644)

# Walk with gitignore filtering.
def collector(initial, resource, path, stack):
    if initial == None:
        return [path]
    return initial + [path]

result = file.walk_tree(root=dir, fn=collector, honor_gitignore=True)

# .gitignore itself is included; skip.log is excluded.
paths = sorted(result)
t.expect_equal(len(paths), 3)  # .gitignore, also_keep.md, keep.txt
t.expect_equal(paths[0], ".gitignore")
t.expect_equal(paths[1], "also_keep.md")
t.expect_equal(paths[2], "keep.txt")

t.expect_node_count(0)
