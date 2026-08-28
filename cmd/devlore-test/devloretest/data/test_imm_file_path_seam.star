# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_file_path_seam.star — Path producers feed path consumers.
#
# Validates: file.name(file.join(...)), file.parent(file.join(...)), file.name(<glob match>)
#
# The provider speaks two path dialects. file.join, file.glob, file.resolve and file.walk_tree build
# paths FOR USE, so they are OS-native. file.name and file.parent answer questions ABOUT a path, so
# they are slash-form. Every existing assertion writes its input in the dialect of the function under
# test — test_imm_file.star checks file.name against the literal "/some/dir/file.txt" — so each one
# passes on every platform and the mismatch between them is never exercised.
#
# The defect only ever appears where a producer's output reaches a consumer's input: on Windows
# file.join returns "a\b", slash-form Base finds no separator in it, and the whole path comes back as
# the "last element". That has been fixed four times in seventeen days (#395, #548, #600, #719).
#
# test_imm_file.star declines multi-part join because "a fixture cannot assert it without encoding a
# platform". True of join's RESULT — but not of the composition. Nothing below names a separator or a
# platform: each assertion says only that taking the last element of a joined path returns the last
# element. That holds everywhere, and it is false on Windows whenever the seam is broken.

# --- file.name(file.join(...)) ---

t.expect_equal(file.name(path=file.join("knowledge", "packages", "slots")), "slots")
t.expect_equal(file.name(path=file.join("a", "b")), "b")
t.expect_equal(file.name(path=file.join("one", "two", "three", "four.txt")), "four.txt")

# --- file.parent(file.join(...)) ---
#
# Slash-form on both platforms, because parent answers a question ABOUT a path rather than building
# one for use. Without the conversion this returns "." on Windows: Dir finds no separator in a
# backslash path and reports the current directory.

t.expect_equal(file.parent(path=file.join("knowledge", "packages", "slots")), "knowledge/packages")
t.expect_equal(file.parent(path=file.join("a", "b")), "a")

# --- file.name(<glob match>) ---
#
# This is the composition the knowledge indexer performs on every directory entry, and the one that
# made it treat every asset directory on Windows as unrecognized.

seam_dir = t.tmp("seam_dir")
file.mkdir(path=seam_dir, mode=0o755)
file.write_text(destination_path=file.join(seam_dir, "homebrew.yaml"), content="x: 1\n", mode=0o644)
file.write_text(destination_path=file.join(seam_dir, "macports.yaml"), content="x: 1\n", mode=0o644)

matches = file.glob(pattern=file.join(seam_dir, "*.yaml"), include_gitignored=True)
t.expect_equal(len(matches), 2)

names = sorted([file.name(path=m) for m in matches])
t.expect_equal(names, ["homebrew.yaml", "macports.yaml"])

# --- the same composition against a directory, which is what the indexer classifies ---

nested = file.join(seam_dir, "slots")
file.mkdir(path=nested, mode=0o755)
file.write_text(destination_path=file.join(nested, "keep.yaml"), content="x: 1\n", mode=0o644)

dirs = [file.name(path=m) for m in file.glob(pattern=file.join(seam_dir, "*"), include_gitignored=True) if file.is_dir(path=m)]
t.expect_equal(dirs, ["slots"])

t.expect_unit_count(0)
