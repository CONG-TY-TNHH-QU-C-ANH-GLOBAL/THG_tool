# 🚀 SYSTEM SPECIFICATION: FACEBOOK AUTOMATION SAAS (INFORMATION RETRIEVAL)

## 1. CORE MINDSET & ARCHITECTURE PRINCIPLES
**Tuyệt đối tuân thủ:** Đây KHÔNG phải là một tool cào dữ liệu (Scraping Tool). Đây là một nền tảng SaaS chuẩn mực dựa trên nguyên lý: **Retrieval System + Reasoning System + Memory System**.
Mục tiêu cốt lõi: Lấy đúng data theo Intent của User trong môi trường cực kỳ nhiễu (noisy) của Facebook.

### The "No-Blind-Crawling" Rule
* Agents tuyệt đối KHÔNG được thực thi bất kỳ lệnh crawl nào trực tiếp từ User Prompt.
* Hành động crawl phải luôn được cấu trúc hóa, lập kế hoạch và sinh query linh hoạt thông qua một **Planner Agent** trước khi chạm vào trình duyệt/API.
* Không hardcode domain (bất động sản, logistics, crypto...). Mọi thứ phải được Planner tự động hóa (Cross-domain generalization).

---

## 2. SYSTEM ARCHITECTURE & PIPELINE 

Hệ thống hoạt động theo Pipeline tuyến tính một chiều. Mọi Agent và Module phải được chia tách rõ ràng:

1. `User Prompt` (Natural Language)
2. `Planner Agent` (Reasoning & Strategy Generation)
3. `Execution Plan` (Structured JSON Schema)
4. `Crawler Agent` (Targeted Data Acquisition)
5. `Pre-filter Engine` (Rule-based Noise Reduction)
6. `AI Analyzer` (Deep Intent Classification)
7. `Scoring Engine` (Lead Quality Ranking)
8. `Memory & Dedup` (System Learning & Duplication Prevention)
9. `Lead Output` (Final Deliverable)

---

## 3. AGENT DEFINITIONS & RESPONSIBILITIES

### A. The Planner Agent (Core Brain)
* **Vai trò:** Chuyển đổi User Prompt thành Chiến lược khai thác (Retrieval Strategy).
* **Nhiệm vụ:** Trích xuất Domain, tạo bộ từ khóa tìm kiếm (Queries), xác định tín hiệu nhiễu (Signals) và chọn phương pháp crawl.
* **Input Example:** "Tìm khách hàng cần dịch vụ fulfill đi US trong group Shopify, loại bỏ bọn agency."
* **Output BẮT BUỘC (Strict JSON Schema):**
  ```json
  {
    "target_role": "potential_customer",
    "domain": "fulfillment_us",
    "queries": [
      "looking for fulfillment US",
      "need warehouse USA",
      "3PL recommendation US"
    ],
    "include_signals": ["looking for", "need", "recommendation", "problem", "help"],
    "exclude_signals": ["we provide", "our service", "agency", "supplier", "DM me for price"]
  }
  ```

---

## 4. IMPLEMENTATION REALITY (must match code — updated 2026-06)

This section reconciles the idealized pipeline above with what the code in
`services/agent-brain/` and the Go consumers actually do today.

### Canonical `target_role` enum
The only values emitted to the Go classifier are:

```text
potential_customer | candidate | partner | ""   (empty = no role filter)
```

`"buyer"` is **not** canonical. The LLM, the org business-profile field
`target_author_role` (plural vocabulary: `customers`, `suppliers`, `providers`,
`candidates`, `partners`, ...), and older drafts all use synonyms. These are
**normalized at the sidecar boundary** by `planner_llm.normalize_target_role`
before emission (e.g. `buyer/customer/customers → potential_customer`,
`supplier/suppliers/provider/providers/seller → partner`,
`candidates → candidate`, `unknown`/unsupported → `""`). This applies to both
the LLM plan role and the profile fallback in `brain.build_market_signal_gate`,
so Go (`internal/ai/universal.go`, `classifier.go`) never sees a plural/profile
role. Code authoring a new role MUST stay within the canonical enum.

### `crawl_strategy` is aspirational — NOT emitted today
The `crawl_strategy{...}` block above is a **future/aspirational** shape. The
current planner (`planner_llm.extract_execution_plan`) emits only
`{target_role, domain, queries, include_signals, exclude_signals}`. Crawl-tool
selection (`scrape_group` / `scrape_comments` / `search_groups`) is decided by
rule-based keyword routing in `brain.plan`, not by a `crawl_strategy` object. Do
not rely on `crawl_strategy` until it is implemented.

### Role of the LLM (rule-based first, LLM is enrichment)
The Planner Agent is **not** a mandatory LLM call. `brain.plan` routes intent and
chooses actions via deterministic keyword rules; the LLM planner is **best-effort
enrichment** that adds per-prompt `queries`/`include_signals`/`exclude_signals`
and a canonical role **when `OPENAI_API_KEY` is set**. When the LLM is
unavailable it returns an empty plan and the rule-based path still works. **Go
remains the sole executor.**

### Field limits (match `planner_llm.py`)
`queries` ≤ `MAX_QUERIES` (10), each ≤ `MAX_QUERY_LEN` (120) chars;
`include_signals`/`exclude_signals` ≤ `MAX_SIGNALS` (25), each ≤ `MAX_SIGNAL_LEN`
(80); `domain` ≤ 60 chars. The system-prompt guidance ("3–8 queries") is a hint
to the model; the shape-check limits above are what is enforced.

### `domain` field
`domain` is an LLM-generated, length-limited slug. It is **not** a routing
authority today — `brain.plan` does not branch on it — so treat it as a label,
not a gate. (No domain catalog / validation is implemented.)

### Market Signal Gate may be empty (valid, low-filtering)
`positive_signals`/`negative_signals` come from the org profile's
`target_signals`/`negative_signals` plus LLM signals. If both the profile fields
and the LLM signals are empty, the gate is effectively a **no-op** — every post
passes the pre-filter and the downstream deterministic scorer + AI classifier
decide alone. This is a valid state, not an error.

### Identity seed scope
`scripts/seed_company_identity.sql` is a **minimal** SQLite `user_context`
identity seed — it only sets `business_name`, `business_website`,
`official_contact`, `primary_cta` (consumed by the grounded comment generator
via `ai.BusinessProfile`). It does **not** seed the Market Signal Gate fields
(`target_signals`, `negative_signals`, `target_author_role`, `industry`,
`services`, ...); those must be populated separately or the gate stays empty
(see above).