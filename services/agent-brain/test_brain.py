import os
import unittest
from unittest.mock import patch

# Ensure planner_llm sees no API key during these unit tests so it returns
# empty plans deterministically. brain tests cover the rule-based router only;
# planner_llm has its own test file.
os.environ.pop("OPENAI_API_KEY", None)

from brain import build_market_signal_gate, combine_keywords, plan


PROFILE = {
    "name": "THG",
    "industry": "fulfillment",
    "description": "Fulfillment, express and warehouse for sellers.",
    "services": "source hàng VN/TQ, fulfillment, ship đi Mỹ và thế giới",
    "targets": "POD/dropship sellers cần tìm hàng và fulfillment",
    "target_author_role": "customers",
    "target_signals": "tìm fulfillment, cần báo giá, looking for supplier",
    "negative_signals": "bài quảng cáo dịch vụ, tuyển CTV, spam link, đối thủ tự bán",
}


class BrainPlannerTest(unittest.TestCase):
    def test_pod_prompt_with_group_url_returns_crawl_source(self):
        out = plan(
            {
                "prompt": "Tôi cần tìm tệp khách POD,dropship có nhu cầu fulfillment https://www.facebook.com/groups/1312868109620530 cào 20 bài",
                "business_profile": PROFILE,
            }
        )
        self.assertEqual(out["intent"], "crawl_source")
        self.assertEqual(out["decision"], "execute")
        self.assertEqual(out["actions"][0]["tool"], "scrape_group")
        self.assertEqual(out["actions"][0]["args"]["max_items"], 20)
        # Gate negatives must come from the org's own profile, not from a
        # vertical-keyed switch in the brain. The profile's negative_signals
        # field above contains "bài quảng cáo dịch vụ" — that's what should
        # surface, not English literals injected on a "pod/dropship" keyword.
        self.assertIn("bài quảng cáo dịch vụ", out["market_signal_gate"]["negative_signals"])

    def test_prompt_without_url_discovers_sources(self):
        out = plan({"prompt": "Cào tôi POD dropship sellers cần fulfillment", "business_profile": PROFILE})
        self.assertEqual(out["intent"], "discover_sources")
        self.assertEqual(out["actions"][0]["tool"], "search_groups")
        self.assertNotIn("url", out["actions"][0]["args"])

    def test_out_of_facebook_refuses(self):
        out = plan({"prompt": "Viết cho tôi công thức nấu ăn tối nay", "business_profile": PROFILE})
        self.assertEqual(out["domain_scope"], "out_of_scope")
        self.assertEqual(out["decision"], "refuse")

    def test_gate_is_empty_when_profile_has_no_signals(self):
        # No target_signals / negative_signals → gate sides are empty. The
        # downstream pipeline (deterministic scorer + AI classifier) then
        # decides without any baked-in vertical phrases.
        empty_profile = {"name": "Acme", "industry": "anything"}
        gate = build_market_signal_gate(empty_profile)
        self.assertEqual(gate["positive_signals"], [])
        self.assertEqual(gate["negative_signals"], [])
        self.assertEqual(gate["reject_rules"], [])

    def test_gate_merges_llm_plan_with_profile(self):
        # LLM plan signals must be additive on top of the profile's signals,
        # and prompt-derived target_role wins over the profile default.
        plan_payload = {
            "target_role": "potential_customer",
            "include_signals": ["tìm kho", "cần supplier"],
            "exclude_signals": ["chuyên cung cấp"],
        }
        gate = build_market_signal_gate(PROFILE, plan_payload)
        self.assertEqual(gate["target_role"], "potential_customer")
        self.assertIn("tìm fulfillment", gate["positive_signals"])  # from profile
        self.assertIn("tìm kho", gate["positive_signals"])  # from LLM plan
        self.assertIn("bài quảng cáo dịch vụ", gate["negative_signals"])  # from profile
        self.assertIn("chuyên cung cấp", gate["negative_signals"])  # from LLM plan

    def test_gate_falls_back_to_profile_when_plan_role_empty(self):
        gate = build_market_signal_gate(PROFILE, {"target_role": "", "include_signals": [], "exclude_signals": []})
        self.assertEqual(gate["target_role"], "customers")  # profile default

    def test_combine_keywords_merges_unique(self):
        merged = combine_keywords("kho, xuong", ["tìm kho", "cần warehouse", "xuong"])
        # case-folded duplicate ("xuong") filtered; LLM phrases appended in order.
        self.assertIn("kho", merged)
        self.assertIn("tìm kho", merged)
        self.assertIn("cần warehouse", merged)
        # base 2 (kho, xuong) + 2 unique extras (tìm kho, cần warehouse); xuong dedup'd
        parts = [p.strip() for p in merged.split(",") if p.strip()]
        self.assertEqual(len(parts), 4)
        self.assertEqual(parts.count("xuong"), 1)

    def test_plan_uses_llm_signals_when_available(self):
        # Mock the LLM extractor so we don't need an OpenAI key.
        fake_plan = {
            "target_role": "potential_customer",
            "domain": "warehouse_us",
            "queries": ["tìm kho mỹ", "looking for warehouse US"],
            "include_signals": ["tìm kho", "cần xưởng"],
            "exclude_signals": ["chuyên cung cấp", "agency cần tuyển"],
        }
        with patch("brain.planner_llm.extract_execution_plan", return_value=fake_plan):
            out = plan(
                {
                    "prompt": "Tìm khách có nhu cầu tìm kho hoặc xưởng tại Mỹ cào 100 bài https://www.facebook.com/groups/1312868109620530",
                    "business_profile": PROFILE,
                }
            )
        self.assertEqual(out["intent"], "crawl_source")
        gate = out["market_signal_gate"]
        self.assertEqual(gate["target_role"], "potential_customer")
        self.assertIn("tìm kho", gate["positive_signals"])
        self.assertIn("chuyên cung cấp", gate["negative_signals"])
        # crawl action keywords enriched with LLM queries
        kws = out["actions"][0]["args"].get("keywords", "")
        self.assertIn("tìm kho mỹ", kws)


if __name__ == "__main__":
    unittest.main()
