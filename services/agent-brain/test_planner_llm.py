"""Unit tests for planner_llm. The OpenAI client is fully mocked — no network."""

import json
import os
import unittest
from types import SimpleNamespace
from unittest.mock import patch

import planner_llm


def _fake_openai_response(payload: dict) -> SimpleNamespace:
    """Mimic the shape of openai.ChatCompletion .choices[0].message.content."""
    return SimpleNamespace(
        choices=[SimpleNamespace(message=SimpleNamespace(content=json.dumps(payload, ensure_ascii=False)))]
    )


class _FakeChatCompletions:
    def __init__(self, payloads):
        self._payloads = list(payloads)
        self.calls = 0

    def create(self, **_kwargs):  # noqa: D401
        self.calls += 1
        item = self._payloads.pop(0)
        if isinstance(item, Exception):
            raise item
        return _fake_openai_response(item)


class _FakeOpenAI:
    def __init__(self, payloads):
        self.chat = SimpleNamespace(completions=_FakeChatCompletions(payloads))


class PlannerLLMTest(unittest.TestCase):
    def setUp(self):
        os.environ["OPENAI_API_KEY"] = "test-key"
        planner_llm.clear_cache()

    def tearDown(self):
        os.environ.pop("OPENAI_API_KEY", None)

    def _patch_openai(self, payloads):
        fake = _FakeOpenAI(payloads)
        return patch("planner_llm.OpenAI", return_value=fake, create=True), fake

    def test_warehouse_buyer_prompt_extracts_potential_customer(self):
        payload = {
            "target_role": "potential_customer",
            "domain": "warehouse_us",
            "queries": ["tìm kho mỹ", "looking for warehouse US"],
            "include_signals": ["tìm kho", "cần xưởng", "looking for warehouse"],
            "exclude_signals": ["chuyên cung cấp", "agency cần tuyển"],
        }
        ctx, _ = self._patch_openai([payload])
        with ctx:
            plan = planner_llm.extract_execution_plan(
                "Tìm khách có nhu cầu tìm kho hoặc xưởng tại Mỹ", {"name": "THG"}
            )
        self.assertEqual(plan["target_role"], "potential_customer")
        self.assertIn("tìm kho", plan["include_signals"])
        self.assertIn("chuyên cung cấp", plan["exclude_signals"])
        self.assertEqual(plan["domain"], "warehouse_us")

    def test_recruitment_prompt_extracts_candidate(self):
        payload = {
            "target_role": "candidate",
            "domain": "recruitment_it",
            "queries": ["tìm việc backend", "looking for dev job"],
            "include_signals": ["đang tìm việc", "looking for job", "open to work"],
            "exclude_signals": ["công ty tuyển", "we are hiring"],
        }
        ctx, _ = self._patch_openai([payload])
        with ctx:
            plan = planner_llm.extract_execution_plan("Tìm ứng viên backend đang tìm việc", {})
        self.assertEqual(plan["target_role"], "candidate")
        self.assertIn("đang tìm việc", plan["include_signals"])

    def test_supplier_prompt_extracts_partner(self):
        payload = {
            "target_role": "partner",
            "domain": "supplier_china",
            "queries": ["nhà cung cấp", "supplier 1688"],
            "include_signals": ["chuyên cung cấp", "nhận đặt hàng"],
            "exclude_signals": ["tìm nhà cung cấp", "looking for supplier"],
        }
        ctx, _ = self._patch_openai([payload])
        with ctx:
            plan = planner_llm.extract_execution_plan("Tìm supplier hàng Trung Quốc", {})
        self.assertEqual(plan["target_role"], "partner")

    def test_returns_empty_plan_when_api_key_missing(self):
        os.environ.pop("OPENAI_API_KEY", None)
        plan = planner_llm.extract_execution_plan("anything", {})
        self.assertEqual(plan, planner_llm.empty_plan())

    def test_falls_back_to_empty_on_persistent_failure(self):
        ctx, fake = self._patch_openai([RuntimeError("boom"), RuntimeError("still boom")])
        with ctx:
            plan = planner_llm.extract_execution_plan("prompt that fails", {})
        self.assertEqual(plan, planner_llm.empty_plan())
        self.assertEqual(fake.chat.completions.calls, 2)  # one retry

    def test_retry_succeeds_on_second_attempt(self):
        good = {
            "target_role": "potential_customer",
            "domain": "",
            "queries": ["q1"],
            "include_signals": ["s1"],
            "exclude_signals": ["x1"],
        }
        ctx, fake = self._patch_openai([RuntimeError("transient"), good])
        with ctx:
            plan = planner_llm.extract_execution_plan("prompt", {})
        self.assertEqual(plan["target_role"], "potential_customer")
        self.assertEqual(fake.chat.completions.calls, 2)

    def test_invalid_role_is_normalized_to_empty(self):
        payload = {
            "target_role": "spammer_god_mode",
            "domain": "",
            "queries": [],
            "include_signals": ["ok"],
            "exclude_signals": [],
        }
        ctx, _ = self._patch_openai([payload])
        with ctx:
            plan = planner_llm.extract_execution_plan("prompt", {})
        self.assertEqual(plan["target_role"], "")
        self.assertEqual(plan["include_signals"], ["ok"])

    def test_oversized_lists_are_truncated(self):
        payload = {
            "target_role": "potential_customer",
            "domain": "x" * 200,
            "queries": [f"q{i}" for i in range(100)],
            "include_signals": [f"s{i}" for i in range(100)],
            "exclude_signals": [f"x{i}" for i in range(100)],
        }
        ctx, _ = self._patch_openai([payload])
        with ctx:
            plan = planner_llm.extract_execution_plan("prompt", {})
        self.assertLessEqual(len(plan["queries"]), planner_llm.MAX_QUERIES)
        self.assertLessEqual(len(plan["include_signals"]), planner_llm.MAX_SIGNALS)
        self.assertLessEqual(len(plan["exclude_signals"]), planner_llm.MAX_SIGNALS)
        self.assertLessEqual(len(plan["domain"]), 60)

    def test_cache_hit_skips_api_call(self):
        payload = {
            "target_role": "potential_customer",
            "domain": "",
            "queries": [],
            "include_signals": ["once"],
            "exclude_signals": [],
        }
        ctx, fake = self._patch_openai([payload])
        with ctx:
            first = planner_llm.extract_execution_plan("same prompt", {"k": "v"})
            second = planner_llm.extract_execution_plan("same prompt", {"k": "v"})
        self.assertEqual(first, second)
        self.assertEqual(fake.chat.completions.calls, 1)

    def test_malformed_json_returns_empty_plan(self):
        # Send a string that looks like a real response object but with garbage content.
        bad = SimpleNamespace(
            choices=[SimpleNamespace(message=SimpleNamespace(content="not json at all"))]
        )

        class FakeChat:
            def __init__(self):
                self.calls = 0

            def create(self, **_kwargs):
                self.calls += 1
                return bad

        fake = SimpleNamespace(chat=SimpleNamespace(completions=FakeChat()))
        with patch("planner_llm.OpenAI", return_value=fake, create=True):
            plan = planner_llm.extract_execution_plan("prompt", {})
        self.assertEqual(plan, planner_llm.empty_plan())


if __name__ == "__main__":
    unittest.main()
