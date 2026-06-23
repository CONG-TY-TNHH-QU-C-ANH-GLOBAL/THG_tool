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
    "target_role": "buyer",
    "domain": "fulfillment_us",
    "queries": [
      "looking for fulfillment US",
      "need warehouse USA",
      "3PL recommendation US"
    ],
    "include_signals": ["looking for", "need", "recommendation", "problem", "help"],
    "exclude_signals": ["we provide", "our service", "agency", "supplier", "DM me for price"],
    "crawl_strategy": {
      "use_group_search": true,
      "use_feed_scan": true,
      "use_comment_mining": true,
      "target_groups_keywords": ["shopify dropshipping", "e-commerce US"]
    }
  }