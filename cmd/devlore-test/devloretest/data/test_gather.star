# test_gather.star — Use plan.gather to synchronize parallel outputs.
#
# Write two files independently, then gather their outputs so a
# shell command runs only after both writes complete.
#
# Validates: plan.gather, plan.file.write_text, plan.shell.exec

a = t.tmp("gather_a.txt")
b = t.tmp("gather_b.txt")
c = t.tmp("gather_c.txt")

# Two independent writes.
out_a = plan.file.write_text(destination_path=a, content="alpha", mode=0o644)
out_b = plan.file.write_text(destination_path=b, content="bravo", mode=0o644)

# Gather creates a fan-in: shell.exec depends on both writes completing.
# The shell command concatenates both files into a third.
g = plan.gather(out_a, out_b)
plan.shell.exec(command="cat " + a + " " + b + " > " + c)

t.expect_file(a, content="alpha")
t.expect_file(b, content="bravo")
t.expect_file(c, content="alphabravo")
t.expect_node_count(3)  # write_text(a) + write_text(b) + shell.exec
