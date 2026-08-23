# SPDX-License-Identifier: Apache-2.0
# Copyright Noble Factor. All rights reserved.

# test_judgment_entry_slot_refusal.star — judgment scenario: the Entry slot refuses authored strings
# (docs/plans/resource-construction.md, explicit-conversion suite item 13).
#
# An authored literal into an interface-typed resource slot draws the shaped plan-time refusal — a claim
# asserts a kind and an interface asserts none; the author states the kind or feeds a discovery. Never an
# incidental construction error, and never a plan that limps to dispatch.

t.expect_error("a claim asserts a kind")

plan.file.observe(resource="some/path")
