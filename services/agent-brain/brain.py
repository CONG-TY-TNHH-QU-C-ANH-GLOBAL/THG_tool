#!/usr/bin/env python3
"""THG Facebook Agent Brain v1.

This sidecar is intentionally dependency-free for the first production slice.
It produces a strict planner contract; Go remains the only executor.
"""

from __future__ import annotations

import json
import os
import re
import unicodedata
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

import planner_llm


FACEBOOK_URL_RE = re.compile(r"https?://[^\s]+(?:facebook\.com|fb\.com)[^\s]*", re.I)


def fold(text: str) -> str:
    text = (text or "").lower()
    text = unicodedata.normalize("NFD", text)
    return "".join(ch for ch in text if unicodedata.category(ch) != "Mn")


def compact(text: str, limit: int = 220) -> str:
    text = re.sub(r"\s+", " ", (text or "").strip())
    return text[:limit].strip()


def first_facebook_url(prompt: str) -> str:
    match = FACEBOOK_URL_RE.search(prompt or "")
    if not match:
        return ""
    return match.group(0).rstrip(".,);]")


def is_facebook_scope(prompt: str) -> bool:
    folded = fold(prompt)
    if first_facebook_url(prompt):
        return True
    terms = [
        "facebook",
        "fb ",
        "group",
        "fanpage",
        "profile",
        "messenger",
        "inbox",
        "comment",
        "crawl",
        "scrape",
        "cao ",
        "quet ",
        "tim khach",
        "lead",
        "dang bai",
        "posting",
        "local kit",
        "browser",
    ]
    return any(term in folded for term in terms)


def is_business_context_prompt(prompt: str) -> bool:
    folded = fold(prompt)
    terms = [
        "doanh nghiep",
        "cong ty",
        "chung toi",
        "brand",
        "business profile",
        "dich vu",
        "target",
        "khach mua",
        "tin hieu",
        "loai bo",
    ]
    return any(term in folded for term in terms)


def prompt_keywords(prompt: str) -> str:
    text = FACEBOOK_URL_RE.sub(" ", prompt or "")
    folded = fold(text)
    stop = {
        "toi",
        "can",
        "tim",
        "tep",
        "khach",
        "cao",
        "crawl",
        "scrape",
        "quet",
        "bai",
        "viet",
        "lien",
        "quan",
        "cho",
        "toi",
        "trong",
        "nhom",
        "facebook",
    }
    tokens: list[str] = []
    seen: set[str] = set()
    for raw in re.split(r"[\s,;|/]+", folded):
        raw = raw.strip(" .:-_")
        if len(raw) < 3 or raw in stop or raw in seen or raw.startswith("http"):
            continue
        seen.add(raw)
        tokens.append(raw)
        if len(tokens) >= 8:
            break
    return ", ".join(tokens)


def extract_max_items(prompt: str) -> int:
    folded = fold(prompt)
    for pattern in (
        r"(\d{1,3})\s*(?:bai|post|posts|lead|leads)",
        r"(?:lay|cao|crawl|quet|tim)\s*(\d{1,3})",
    ):
        match = re.search(pattern, folded)
        if match:
            return min(max(int(match.group(1)), 1), 200)
    return 0


def profile_value(profile: dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = str(profile.get(key) or "").strip()
        if value:
            return value
    return ""


def has_business_profile(profile: dict[str, Any]) -> bool:
    return bool(profile_value(profile, "description", "industry", "services", "targets", "name"))


def build_market_signal_gate(
    profile: dict[str, Any],
    llm_plan: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Build a Market Signal Gate from BusinessProfile + per-prompt LLM plan.

    Profile-derived phrases (org's standing target_signals / negative_signals)
    are merged with the LLM Planner's per-prompt include_signals /
    exclude_signals. The merge is additive: profile signals always survive,
    and LLM signals enrich them with prompt-specific phrases (e.g. "tìm kho",
    "cần supplier") so the downstream Go gate can hard-reject obvious
    off-target posts before they reach the AI classifier.

    target_role: prompt-derived role wins when set, otherwise the org default.
    Empty fields disable each rule — the AI classifier still gets the prompt
    context via ClassifyIntent on the Go side.
    """
    profile_role = profile_value(profile, "target_author_role")
    profile_positives = split_signal_field(profile_value(profile, "target_signals"))
    profile_negatives = split_signal_field(profile_value(profile, "negative_signals", "reject_rules"))

    plan_role = ""
    plan_positives: list[str] = []
    plan_negatives: list[str] = []
    if isinstance(llm_plan, dict):
        plan_role = str(llm_plan.get("target_role") or "").strip()
        plan_positives = [str(x).strip() for x in (llm_plan.get("include_signals") or []) if str(x).strip()]
        plan_negatives = [str(x).strip() for x in (llm_plan.get("exclude_signals") or []) if str(x).strip()]

    positives = merge_unique(profile_positives, plan_positives)
    negatives = merge_unique(profile_negatives, plan_negatives)
    # The LLM plan_role is already canonical; profile_role uses the plural
    # business-profile vocabulary (customers/suppliers/providers/...). Normalize
    # the chosen value so the Go classifier only ever sees the canonical enum.
    target_role = planner_llm.normalize_target_role(plan_role or profile_role)

    return {
        "target_role": target_role,
        "positive_signals": positives,
        "negative_signals": negatives,
        "reject_rules": negatives,
        "min_confidence": 0.65,
    }


def split_signal_field(value: str) -> list[str]:
    parts = re.split(r"[\n,;|]+", value or "")
    return [compact(p, 80) for p in parts if compact(p, 80)]


def combine_keywords(base: str, extra: Any) -> str:
    """Merge prompt-derived keywords with LLM-derived queries.

    The crawler `keywords` arg is a comma-separated string. We append unique
    LLM queries so the connector's keyword filter benefits from richer phrases
    without duplicating folded tokens already present.
    """
    base_parts = [p.strip() for p in (base or "").split(",") if p.strip()]
    extras: list[str] = []
    if isinstance(extra, list):
        for item in extra:
            if isinstance(item, str):
                clean = " ".join(item.split()).strip()
                if clean:
                    extras.append(clean)
    merged = merge_unique(base_parts, extras)
    return ", ".join(merged)


def merge_unique(base: list[str], extra: list[str]) -> list[str]:
    seen = {fold(x) for x in base}
    out = list(base)
    for item in extra:
        key = fold(item)
        if key not in seen:
            seen.add(key)
            out.append(item)
    return out


def action(tool: str, args: dict[str, Any], reason: str, evidence: list[str], requires_browser: bool = False, requires_profile: bool = False) -> dict[str, Any]:
    return {
        "tool": tool,
        "args": args,
        "reason": reason,
        "evidence": evidence,
        "requires_browser": requires_browser,
        "requires_profile": requires_profile,
        "recurrence": {"enabled": tool in {"scrape_group", "search_groups"}, "interval_minutes": 30},
    }


def plan(payload: dict[str, Any]) -> dict[str, Any]:
    prompt = str(payload.get("prompt") or "")
    source = str(payload.get("source") or "")
    profile = payload.get("business_profile") or {}
    if not isinstance(profile, dict):
        profile = {}
    folded = fold(prompt)
    fb_url = first_facebook_url(prompt)
    max_items = extract_max_items(prompt)
    llm_plan = planner_llm.extract_execution_plan(prompt, profile)
    gate = build_market_signal_gate(profile, llm_plan)

    base = {
        "domain_scope": "facebook",
        "intent": "strategy_chat",
        "decision": "chat",
        "confidence": 0.72,
        "response_summary": "Mình sẽ xử lý trong phạm vi Facebook sales intelligence của workspace.",
        "market_signal_gate": gate,
        "actions": [],
    }

    if not is_facebook_scope(prompt) and not is_business_context_prompt(prompt):
        return {
            **base,
            "domain_scope": "out_of_scope",
            "intent": "strategy_chat",
            "decision": "refuse",
            "confidence": 0.91,
            "response_summary": "Workspace này chỉ xử lý chiến lược, dữ liệu và automation liên quan Facebook.",
        }

    if is_business_context_prompt(prompt) and not any(term in folded for term in ["cao", "crawl", "scrape", "quet", "tim "]):
        return {
            **base,
            "intent": "business_context",
            "decision": "execute",
            "confidence": 0.82,
            "response_summary": "Mình sẽ lưu định vị doanh nghiệp để các lần crawl/comment/inbox sau lọc đúng tệp hơn.",
            "actions": [
                action("describe_business", {"description": compact(prompt, 1200)}, "Update org business context", ["User described business context"], False, False)
            ],
        }

    # Outbound actions on already-stored leads (inbox / comment) and posting
    # are evaluated BEFORE the "needs positioning" ask-back. The leads exist
    # in the workspace already — scored by the signal gate that captured
    # them — so refusing to act until the operator re-supplies positioning
    # blocks legitimate execution. The comment generator handles missing
    # profile data gracefully (downstream context loader returns a safe
    # fallback when nothing is set), so dispatching without a profile
    # produces blander copy, not broken copy. Positioning belongs to crawl
    # branches below.
    if any(term in folded for term in ["inbox", "messenger", "nhan tin", "dm"]) and any(term in folded for term in ["lead", "khach", "tat ca", "all"]):
        return {
            **base,
            "intent": "inbox_leads",
            "decision": "execute",
            "confidence": 0.82,
            "response_summary": "Mình sẽ tạo hàng đợi inbox cho leads đủ điều kiện theo guardrails của workspace.",
            "actions": [action("inbox_all_leads", {"score_filter": "hot"}, "Queue inbox outreach for qualified leads", ["User requested inbox automation"], True, True)],
        }

    if any(term in folded for term in ["comment", "binh luan"]) and any(term in folded for term in ["lead", "khach", "tat ca", "all"]):
        return {
            **base,
            "intent": "comment_leads",
            "decision": "execute",
            "confidence": 0.82,
            "response_summary": "Mình sẽ tạo hàng đợi comment cho leads đủ điều kiện theo guardrails của workspace.",
            "actions": [action("comment_all_leads", {"score_filter": "hot"}, "Queue comments for qualified leads", ["User requested comment automation"], True, True)],
        }

    if any(term in folded for term in ["dang bai", "posting", "post len", "tao bai"]):
        args: dict[str, Any] = {"content": compact(prompt, 2000)}
        if fb_url:
            args["group_url"] = fb_url
        return {
            **base,
            "intent": "post_content",
            "decision": "execute",
            "confidence": 0.78,
            "response_summary": "Mình sẽ tạo draft bài đăng Facebook theo context doanh nghiệp.",
            "actions": [action("create_job_post", args, "Create Facebook post draft", ["User requested posting"], True, True)],
        }

    # Crawl-prep ask-back: only fires for genuine crawl/discovery prompts.
    # The bare token "lead"/"leads" was removed from the keyword set —
    # it false-positives on outbound action prompts ("comment all leads",
    # "inbox the leads") that already passed through the branches above.
    # "tim khach" (find customers) is the durable crawl-intent signal that
    # remains.
    if not has_business_profile(profile) and any(term in folded for term in ["cao", "crawl", "scrape", "quet", "tim khach"]):
        return {
            **base,
            "intent": "needs_context",
            "decision": "ask_user",
            "confidence": 0.86,
            "response_summary": "Mình cần định vị doanh nghiệp trước khi crawl để Market Signal Gate lọc đúng buyer intent và loại bài quảng cáo dịch vụ.",
        }

    if fb_url and any(term in folded for term in ["cao", "crawl", "scrape", "quet", "tim", "lead", "phan tich"]):
        tool = "scrape_comments" if any(x in folded for x in ["comment", "binh luan"]) and any(x in fb_url.lower() for x in ["/posts/", "/permalink/", "story_fbid=", "/videos/", "/reel/"]) else "scrape_group"
        args = {"post_url" if tool == "scrape_comments" else "url": fb_url}
        if max_items:
            args["max_items"] = max_items
        keywords = combine_keywords(prompt_keywords(prompt), llm_plan.get("queries"))
        if keywords:
            args["keywords"] = keywords
        return {
            **base,
            "intent": "crawl_source",
            "decision": "execute",
            "confidence": 0.88,
            "response_summary": "Mình sẽ crawl nguồn Facebook đã chọn, áp Market Signal Gate và lưu leads đủ điều kiện về dashboard.",
            "actions": [action(tool, args, "Crawl concrete Facebook source", ["User provided Facebook URL", "User requested lead discovery"], True, True)],
        }

    if any(term in folded for term in ["cao", "crawl", "scrape", "quet", "tim tep", "tim khach", "lead", "leads"]):
        query = combine_keywords(prompt_keywords(prompt), llm_plan.get("queries")) or compact(prompt, 160)
        args = {"query": query}
        if max_items:
            args["max_items"] = max_items
        return {
            **base,
            "intent": "discover_sources",
            "decision": "execute",
            "confidence": 0.8,
            "response_summary": "Mình sẽ tìm nguồn Facebook phù hợp trước, sau đó hệ thống mới crawl và lọc leads theo định vị doanh nghiệp.",
            "actions": [action("search_groups", args, "Discover candidate Facebook sources", ["User did not provide a source URL"], True, True)],
        }

    return base


class Handler(BaseHTTPRequestHandler):
    server_version = "THGAgentBrain/0.1"

    def _json(self, status: int, body: dict[str, Any]) -> None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/healthz":
            self._json(200, {"ok": True, "service": "agent-brain"})
            return
        self._json(404, {"error": "not_found"})

    def do_POST(self) -> None:  # noqa: N802
        if self.path != "/v1/plan":
            self._json(404, {"error": "not_found"})
            return
        try:
            length = int(self.headers.get("Content-Length") or "0")
            payload = json.loads(self.rfile.read(length).decode("utf-8") or "{}")
            self._json(200, plan(payload))
        except Exception as exc:  # pragma: no cover - defensive HTTP boundary
            self._json(500, {"error": str(exc)})

    def log_message(self, fmt: str, *args: Any) -> None:
        if os.getenv("AGENT_BRAIN_ACCESS_LOG") == "1":
            super().log_message(fmt, *args)


def main() -> None:
    host = os.getenv("AGENT_BRAIN_HOST", "0.0.0.0")
    port = int(os.getenv("AGENT_BRAIN_PORT", "8091"))
    server = ThreadingHTTPServer((host, port), Handler)
    print(f"THG Agent Brain listening on {host}:{port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
