# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui_one_string.star — the narration methods take one STRING, and the script renders it.
#
# Deliberate, not a gap: starlark's % is an operator that runs before the call, so a format-string
# signature would re-scan an already-rendered string and corrupt any data containing a %, and a
# variadic one would receive Go natives after conversion and render True as true, None as <nil>.
# Requiring str() keeps rendering in starlark, where it is correct by construction.
#
# An integer is refused here, which it was not until 2026-08-27. op.Convert reached
# reflect.Value.Convert, where int64 counts as "convertible" to string and yields the RUNE at that
# code point -- print(65) emitted "A". #709 made a cross-category conversion an error whatever the
# value, which is python's rule: str(65) says what an author means, and 65 does not.

t.expect_error(r"write str\(x\) to render it")

print(42)
