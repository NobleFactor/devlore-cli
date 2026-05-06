# test_walk_tree_closure.star — WalkTree with lambda capturing closure bindings.
#
# Verifies that a lambda with captured variables works correctly when
# passed as the fn parameter.

dir = t.tmp("walk_closure")
file.mkdir(path=dir, chmod=0o755)
file.write_text(destination_path=t.tmp("walk_closure/hello.py"), content="print", chmod=0o644)
file.write_text(destination_path=t.tmp("walk_closure/readme.md"), content="docs", chmod=0o644)
file.write_text(destination_path=t.tmp("walk_closure/main.py"), content="main", chmod=0o644)
file.write_text(destination_path=t.tmp("walk_closure/notes.txt"), content="notes", chmod=0o644)

# Closure variable: filter by extension.
ext = ".py"

def filter_by_ext(initial, resource, path, stack):
    if initial == None:
        initial = []
    if path.endswith(ext):
        return initial + [path]
    return initial

result = file.walk_tree(root=dir, fn=filter_by_ext, honor_gitignore=False)

paths = sorted(result)
t.expect_equal(len(paths), 2)
t.expect_equal(paths[0], "hello.py")
t.expect_equal(paths[1], "main.py")

t.expect_node_count(0)
