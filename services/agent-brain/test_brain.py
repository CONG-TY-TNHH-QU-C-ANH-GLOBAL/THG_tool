import unittest

from brain import classify_market_signal, plan


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
        self.assertIn("provider advertisement", out["market_signal_gate"]["negative_signals"])

    def test_prompt_without_url_discovers_sources(self):
        out = plan({"prompt": "Cào tôi POD dropship sellers cần fulfillment", "business_profile": PROFILE})
        self.assertEqual(out["intent"], "discover_sources")
        self.assertEqual(out["actions"][0]["tool"], "search_groups")
        self.assertNotIn("url", out["actions"][0]["args"])

    def test_out_of_facebook_refuses(self):
        out = plan({"prompt": "Viết cho tôi công thức nấu ăn tối nay", "business_profile": PROFILE})
        self.assertEqual(out["domain_scope"], "out_of_scope")
        self.assertEqual(out["decision"], "refuse")

    def test_provider_ad_rejected_unless_profile_targets_providers(self):
        post = "Bên mình cung cấp dịch vụ fulfillment giá tốt, nhận order POD toàn cầu."
        rejected = classify_market_signal(post, PROFILE)
        self.assertFalse(rejected["keep"])
        self.assertEqual(rejected["reason"], "provider_ad")

        provider_profile = dict(PROFILE)
        provider_profile["target_author_role"] = "partners, suppliers, providers"
        kept = classify_market_signal(post, provider_profile)
        self.assertTrue(kept["keep"])


if __name__ == "__main__":
    unittest.main()
