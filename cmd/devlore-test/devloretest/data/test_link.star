# test_link.star — Write a file and create a symlink to it.
#
# Validates: plan.file.write_text, plan.file.link

target = t.tmp("link_target.txt")
link   = t.tmp("link_pointer.txt")

written = plan.file.write_text(destination_path=target, content="linked content", chmod=0o644)
linked  = plan.file.link(source=written, target_path=link)

graph = plan.assemble_definition([written, linked])

t.expect_file(target, content="linked content")
t.expect_file(link, content="linked content")
t.expect_unit_count(2)

t.run(graph)
