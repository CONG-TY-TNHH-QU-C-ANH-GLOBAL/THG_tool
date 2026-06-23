"""Tests for canonical target_role normalization at the sidecar boundary.

Covers planner_llm.normalize_target_role (the synonym map) and the LLM-off /
profile-fallback path in brain.build_market_signal_gate. No network, no DB.
"""
import os
import unittest
from unittest.mock import patch

# build_market_signal_gate / brain.plan must work with the LLM disabled (no API
# key) so the profile-fallback path is deterministic.
os.environ.pop("OPENAI_API_KEY", None)

import brain
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


class CrawlRoutingUnchangedTest(unittest.TestCase):
    """Role normalization must NOT change crawl action/tool selection.

    Each scenario runs brain.plan twice on identical input: once with
    normalize_target_role patched to identity ("before" — the legacy raw
    plan_role-or-profile_role), once with the real normalization ("after").
    The selected intent/decision/tool must be byte-identical; only the emitted
    market_signal_gate.target_role may differ (and only for legacy/plural roles).
    """

    # Static org-5-shaped fixture (NOT live org 5 data, no DB read). A real
    # business profile so has_business_profile() is true and routing is stable.
    ORG5_SHAPED = {
        "name": "THG Fulfill",
        "industry": "fulfillment",
        "description": "Fulfillment, sourcing and US shipping for POD sellers.",
        "services": "source hàng VN/TQ, fulfillment, ship đi Mỹ",
        "targets": "POD/dropship sellers cần fulfillment",
    }
    # A crawl prompt with a concrete group URL -> deterministic scrape_group.
    GROUP_PROMPT = (
        "tìm khách POD cần fulfillment "
        "https://www.facebook.com/groups/1312868109620530 cào 20 bài"
    )

    _IDENTITY = staticmethod(lambda raw: raw if isinstance(raw, str) else "")

    def _route(self, target_author_role, plan_role, normalize):
        profile = dict(self.ORG5_SHAPED, target_author_role=target_author_role)
        fake_plan = planner_llm.empty_plan()
        fake_plan["target_role"] = plan_role
        with patch.object(planner_llm, "extract_execution_plan", return_value=fake_plan), \
                patch.object(planner_llm, "normalize_target_role", side_effect=normalize):
            out = brain.plan({"prompt": self.GROUP_PROMPT, "business_profile": profile})
        tool = out["actions"][0]["tool"] if out.get("actions") else None
        return {
            "target_role": out["market_signal_gate"]["target_role"],
            "intent": out["intent"],
            "decision": out["decision"],
            "tool": tool,
        }

    def _assert_routing_stable(self, target_author_role, plan_role, expected_after_role):
        before = self._route(target_author_role, plan_role, self._IDENTITY)
        after = self._route(target_author_role, plan_role, planner_llm.normalize_target_role)
        # Routing (intent/decision/tool) identical before vs after normalization.
        self.assertEqual(before["intent"], after["intent"])
        self.assertEqual(before["decision"], after["decision"])
        self.assertEqual(before["tool"], after["tool"])
        # Sanity: this fixture routes to a concrete crawl tool.
        self.assertEqual(after["tool"], "scrape_group")
        self.assertEqual(after["intent"], "crawl_source")
        # Only the emitted role changes (to canonical).
        self.assertEqual(after["target_role"], expected_after_role)
        return before["target_role"], after["target_role"], after["tool"]

    def test_scenario1_canonical_role_present_unchanged(self):
        b, a, _ = self._assert_routing_stable("customers", "potential_customer", "potential_customer")
        self.assertEqual(b, "potential_customer")  # plan role wins; no change
        self.assertEqual(a, "potential_customer")

    def test_scenario2_llm_off_customers_fallback(self):
        b, a, _ = self._assert_routing_stable("customers", "", "potential_customer")
        self.assertEqual(b, "customers")  # legacy unsafe value
        self.assertEqual(a, "potential_customer")  # normalized

    def test_scenario3_partner_fallback(self):
        b, a, _ = self._assert_routing_stable("suppliers", "", "partner")
        self.assertEqual(b, "suppliers")
        self.assertEqual(a, "partner")

    def test_scenario4_candidate_fallback(self):
        b, a, _ = self._assert_routing_stable("candidates", "", "candidate")
        self.assertEqual(b, "candidates")
        self.assertEqual(a, "candidate")


if __name__ == "__main__":
    unittest.main()
