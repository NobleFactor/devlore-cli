# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_imm_ui_no_bool.star — a bool is refused where a string is wanted.
#
# Go permits no bool-to-string conversion, so this was already refused before #709 and is pinned
# here to keep the pair visible: an integer is refused by RULE, a bool by the language. Losing
# either one silently would be a regression, and only one of them has a guard protecting it.

t.expect_error("string")

print(True)
