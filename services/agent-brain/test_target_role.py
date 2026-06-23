"""Tests for canonical target_role normalization at the sidecar boundary.

Covers planner_llm.normalize_target_role (the synonym map) and the LLM-off /
profile-fallback path in brain.build_market_signal_gate. No network, no DB.
"""
import os
import unittest

# build_market_signal_gate must work with the LLM disabled (no API key) so the
# profile-fallback path is deterministic.
os.environ.pop("OPENAI_API_KEY", None)

import planner_llm
from brain import build_market_signal_gate


class NormalizeTargetRoleTest(unittest.TestCase):
    """The sidecar must only ever emit the canonical target_role enum."""

    def test_synonyms_map_to_canonical(self):
        cases = {
            # potential_customer family
            "buyer": "potential_customer",
            "buyers": "potential_customer",
            "customer": "potential_customer",
            "customers": "potential_customer",
            "potential_customer": "potential_customer",
            # candidate family
            "candidate": "candidate",
            "candidates": "candidate",
            # partner family
            "partner": "partner",
            "partners": "partner",
            "supplier": "partner",
            "suppliers": "partner",
            "provider": "partner",
            "providers": "partner",
            "seller": "partner",
            "sellers": "partner",
            # empty / unknown / unsupported -> ""
            "unknown": "",
            "": "",
            "spammer_god_mode": "",
        }
        for raw, expected in cases.items():
            self.assertEqual(planner_llm.normalize_target_role(raw), expected, raw)

    def test_case_and_whitespace_insensitive(self):
        self.assertEqual(planner_llm.normalize_target_role("  Customers "), "potential_customer")
        self.assertEqual(planner_llm.normalize_target_role("SUPPLIERS"), "partner")

    def test_non_string_normalizes_to_empty(self):
        self.assertEqual(planner_llm.normalize_target_role(None), "")
        self.assertEqual(planner_llm.normalize_target_role(123), "")

    def test_shape_check_normalizes_synonym_role(self):
        # An LLM following the OLD spec example ("buyer") must still yield a
        # canonical role, not be dropped to "".
        plan = planner_llm._shape_check(
            {"target_role": "buyer", "queries": [], "include_signals": [], "exclude_signals": []}
        )
        self.assertEqual(plan["target_role"], "potential_customer")


class GateFallbackNormalizationTest(unittest.TestCase):
    """LLM-off / profile-fallback must not leak plural profile roles to Go."""

    def test_profile_customers_role_normalized_to_potential_customer(self):
        profile = {"name": "THG", "target_author_role": "customers"}
        # Empty plan == what extract_execution_plan returns when the LLM is off.
        gate = build_market_signal_gate(profile, planner_llm.empty_plan())
        self.assertEqual(gate["target_role"], "potential_customer")

    def test_profile_suppliers_role_normalized_to_partner(self):
        profile = {"name": "THG", "target_author_role": "suppliers"}
        gate = build_market_signal_gate(profile, None)
        self.assertEqual(gate["target_role"], "partner")

    def test_canonical_plan_role_wins_and_stays_canonical(self):
        profile = {"name": "THG", "target_author_role": "customers"}
        gate = build_market_signal_gate(profile, {"target_role": "candidate"})
        self.assertEqual(gate["target_role"], "candidate")


if __name__ == "__main__":
    unittest.main()
