# test_backup.star — Write a file and then back it up.
#
# Backup moves the original to path + suffix + timestamp. We verify the
# original is gone (backup is a move, not a copy).
#
# Validates: plan.file.write_text, plan.file.backup

src = t.tmp("backup_src.txt")

written = plan.file.write_text(destination=src, content="backup me", mode=0o644)
plan.file.backup(path=written, backup_suffix=".bak")

# Backup is a rename — the original should no longer exist.
t.expect_no_file(src)
t.expect_node_count(2)
