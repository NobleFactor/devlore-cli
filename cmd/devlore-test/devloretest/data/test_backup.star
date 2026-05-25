# test_backup.star — Write a file and then back it up.
#
# Backup moves the original to path + suffix + timestamp. We verify the
# original is gone (backup is a move, not a copy).
#
# Validates: plan.file.write_text, plan.file.backup

src = t.tmp("backup_src.txt")

written     = plan.file.write_text(destination_path=src, content="backup me", chmod=0o644)
backed_up   = plan.file.backup(source=written, backup_suffix=".bak")

graph = plan.assemble([written, backed_up])

# Backup is a rename — the original should no longer exist.
t.expect_no_file(src)
t.expect_unit_count(2)

t.run(graph)
