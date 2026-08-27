# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui_one_string.star — the narration methods take one STRING, and the script renders it.
#
# Deliberate, not a gap: starlark's % is an operator that runs before the call, so a format-string
# signature would re-scan an already-rendered string and corrupt any data containing a %, and a
# variadic one would receive Go natives after conversion and render True as true, None as <nil>.
# Requiring str() keeps rendering in starlark, where it is correct by construction.
#
# A bool is rejected, which is what this pins.
#
# An INT is not, and that is a defect rather than a decision: op.Convert reaches
# reflect.Value.Convert, where int64 counts as "convertible" to string and yields the RUNE at that
# code point -- print(65) emits "A". Tracked separately; it affects every provider method taking a
# string parameter, not just these. When it is fixed, print(42) belongs in this fixture too.

t.expect_error("param msg: bool value is neither assignable nor convertible to string")

print(True)
