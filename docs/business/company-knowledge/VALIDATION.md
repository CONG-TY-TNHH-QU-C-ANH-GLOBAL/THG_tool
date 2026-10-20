---
doc_type: engineering
status: draft
owner: platform
last_reviewed: 2026-10-20
related_pr_or_issue: Add Taobao sourcing to replies
---

# Bằng chứng kiểm tra bản nền tảng

Repo thay đổi: THG_tool, worktree `feat/company-knowledge-foundation`, nền
`b2a3cf6dcf21c88277875960b746a8258db98adf`. Thay đổi gồm tài liệu/nội dung, script
kiểm tra/sinh, công cụ truy xuất và preview model local, nhánh lexical v2 và
đường nối tri thức vào gợi ý trả lời. Không đổi schema database hoặc cấu hình production;
chưa deploy dịch vụ live hoặc nạp pack.

## Nối ảnh bài đăng ngày 19/10/2026

### Bổ sung model riêng và phân tích nhu cầu

Quyết định mới của task owner ngày 19/10/2026: tạm gác Muse Spark, dùng model/API
hiện có cho giai đoạn tiếp theo. Những kết quả kiểm thử fake/khả năng tương thích
Muse bên dưới là lịch sử local, không phải quyết định đổi provider hoặc đã gọi
Muse thật. Không chạy lại test Go cho cập nhật quyết định chỉ sửa tài liệu này.

- Implemented local `LEAD_MODEL_*` override độc lập, `LEAD_CONTEXT_ANALYSIS_ENABLED`
  mặc định false; Muse Spark dùng sampling mặc định và reasoning low. Không dùng
  credential comment nếu override thiếu. Không thay classifier/outbound provider.
- Schema phân tích gồm intent, loại hàng chung, bằng chứng nguyên văn, thông tin
  thiếu và quan sát ảnh. Hint không được thêm số lượng hoặc nơi nhận vào parser.
  Supplier rewrite phải giữ giá/MOQ/link, nhãn tương tự/nguồn Trung Quốc, điều kiện
  POD và cước từng kiện; không qua guard thì giữ nguyên nháp có căn cứ.
- `ReplyNote` nối tới Telegram ở cả scraper/worker. Log có `analysis` và `wording`.
- PASS: `go test ./internal/config ./internal/ai ./internal/ai/leadcontext ./internal/ai/visualcontext ./internal/services/facebook ./internal/leadingest ./internal/telegram/render ./internal/telegram/control ./internal/server/agent/crawlingest ./internal/crmleadsync ./cmd/scraper ./cmd/worker ./cmd/company_knowledge_preview -count=1 -timeout=90s`.
- Sau kiểm tra chống hint thêm route/số lượng, test lại AI, leadcontext,
  visualcontext, Facebook, render và preview: PASS. Scoped `go vet` gồm config,
  `internal/ai/...`, Facebook, leadingest, Telegram, crawlingest, ba cmd: PASS.
  `go build ./cmd/scraper ./cmd/worker ./cmd/company_knowledge_preview`: PASS.
- 6/6 Node media/manifest tests PASS; pack `--check`: PASS (92 mục, vẫn not_loaded).
  Supplementary complexity <=15 và file-size allowlist check PASS; không đổi allowlist.
- Official `ai_preflight.sh`: import guard exit 0 với 5 cảnh báo có sẵn, sau đó
  dừng vì thiếu Python. Không ghi thành preflight/ai_validate đầy đủ đã qua.
- Docs governance và `git diff --check`: PASS. Chạy `-live-pipeline` bằng template
  thiếu key dừng đúng tại kiểm tra cấu hình, không gọi provider. File riêng
  `tmp/company-knowledge-model/muse.env` đã tạo nếu chưa có, được Git bỏ qua.
- Sau bổ sung guard so sánh cả số tiền và tiền tệ (không chỉ ký hiệu `$`), chạy lại
  toàn bộ test Facebook, vet package và build ba cmd: PASS. Test bảo đảm số lượng
  300 trong bài không cho phép thêm phụ phí `$300` bên cạnh cước tham chiếu thật.
- `-live-pipeline` là mô phỏng public approval trong bộ nhớ, không approve pack
  hoặc gọi Elim/shipping/Telegram. Tests xác nhận bản gốc vẫn pending và override
  thiếu key không dùng nhầm credential. Chưa có API: người dùng cần hướng dẫn tạo
  Meta Model API, chưa chạy model thật hoặc xác nhận chất lượng/ngân sách latency.
- Chưa commit/push/deploy. Chưa đổi intake/classifier upstream; lead bị loại trước
  enrichment chưa được cứu bởi phân tích ảnh. Chi tiết ở
  [runbook](../../engineering/LEAD_MODEL_PIPELINE.md).

- Implemented local đường Chrome Extension → `image_urls` → lead event → model đọc text+ảnh → gợi ý nguồn/clarification → Telegram `ContextNote`. Không đổi schema DB hoặc contract CRM. Ảnh và note không tự được import vào kho tri thức.
- `go test ./internal/ai ./internal/ai/visualcontext ./internal/platform/facebook/media ./internal/services/facebook ./internal/leadingest ./internal/server/agent/crawlingest ./internal/telegram/render ./internal/telegram/control ./internal/config ./cmd/company_knowledge_preview -count=1`: PASS.
- Sau khi bổ sung kiểm tra observations bắt buộc, ảnh thiếu và refactor enqueue helper: `go test ./internal/ai ./internal/ai/visualcontext ./internal/services/facebook ./cmd/scraper ./cmd/worker ./internal/crmleadsync -count=1`: PASS (worker không có test riêng, đã biên dịch).
- `node --test --test-isolation=none local-connector-extension/platforms/facebook/crawl_images.test.mjs local-connector-extension/src/content_files_sync.test.mjs`: 6/6 PASS; `node --check local-connector-extension/content/crawl.js`: PASS. Bao gồm FeedUnit bọc article thật để không bỏ sót ảnh bài chính hoặc lấy nhầm ảnh comment lồng nhau.
- `go vet ./internal/ai/... ./internal/platform/facebook/media ./internal/services/facebook ./internal/leadingest ./internal/server/agent/crawlingest ./internal/telegram/render ./internal/telegram/control ./cmd/company_knowledge_preview ./cmd/scraper ./cmd/worker`: PASS. `go build ./cmd/scraper ./cmd/worker`: PASS.
- Đo gocognit bổ sung trên mọi file Go thay đổi: PASS ngưỡng 15. File-size baseline kiểm bổ sung bằng PowerShell: không có file Go/JS thay đổi vượt 200 dòng ngoài allowlist hiện có; không sửa allowlist.
- Import-boundary guard: exit 0 với 5 cảnh báo có sẵn, không ghi thành hoàn toàn sạch. Docs governance: PASS. Không chạy trọn preflight Python như giới hạn môi trường đã ghi ở lịch sử.
- `LEAD_IMAGE_CONTEXT_ENABLED` mặc định false, vẫn cần org allowlist của gợi ý. Provider/model giữ cấu hình hiện tại; perception tối đa 3 ảnh, một request 5 giây. Chưa có credential để thử model thật; chưa xác minh DOM Facebook live, chưa deploy hoặc phát hành extension.
- Test chứng minh protocol và chính sách fallback bằng dữ liệu/HTTP/DOM giả lập. Không chứng minh độ chính xác nhận diện ảnh, khả năng đọc model/nhãn nhỏ hoặc hiểu hết mâu thuẫn ngữ nghĩa. Legacy CDP/Stateful producer vẫn chưa phát ảnh trong thay đổi này.

Chi tiết kiến trúc, source và rollout: [Lead image context](../../engineering/LEAD_IMAGE_CONTEXT.md).

## Rà soát tiếp ngày 19/10/2026

- Bổ sung nguyên tắc giữ đơn vị/kỳ của số liệu và bằng chứng đọc ảnh vào các mục hiện có; vẫn 92 mục và 21 ví dụ, toàn pack pending/not_loaded.
- Runtime kiểm tối đa 12 ứng viên công ty để chọn tối đa 4 public projections, tránh dừng ở 4 FAQ cũ chưa có payload đủ điều kiện. Test giữ nguyên org/state/type và không lộ description cũ.
- Nhánh model không có quote/ảnh được chặn thêm các mẫu thời gian định lượng, lời hứa giao miễn phí và lời khẳng định đã kiểm tra ảnh. Test dùng cùng số có sẵn trong yêu cầu (200 kg/tháng, 2 kiện) để chứng minh không thể đổi thành 200 ngày hoặc 2 ngày giao hàng. Yêu cầu cung cấp ảnh vẫn được phép.
- Câu fallback vận chuyển đề xuất kiểm tra phương án thay vì suy đã hỗ trợ tuyến cụ thể từ tên nước khách nêu. Tách phần dựng câu thành helper để giữ độ phức tạp <=15.
- `go test ./internal/services/facebook ./internal/workspace_knowledge/runtime ./cmd/company_knowledge_preview -count=1`: PASS. Đây là package bị ảnh hưởng, có HTTP model giả lập; chưa gọi model thật.
- `go vet ./internal/services/facebook ./internal/workspace_knowledge/runtime ./cmd/company_knowledge_preview` và `go build ./cmd/scraper ./cmd/worker`: PASS.
- Đánh giá lại trên pack đã cập nhật: v1 17/24, v2 24/24; report lưu hash mới của pack. Vẫn là bộ hồi quy tổng hợp.
- Đo bổ sung bằng gocognit đã cache trên tất cả file Go thay đổi: PASS ngưỡng 15. Guard chính thức vẫn có giới hạn môi trường đã ghi bên dưới.
- `node scripts/knowledge/build-company-knowledge.mjs --check`, `bash scripts/check_docs_governance.sh`: PASS.
- Tạo template không có secret ở `scripts/knowledge/model-preview.env.example` và bản trống riêng local `tmp/company-knowledge-model/.env`. `git check-ignore` xác nhận bản riêng bị bỏ qua. Chưa có credential trong tiến trình; không đọc/in giá trị secret, không gọi provider hay ghi production.

Bộ lọc claim vẫn bảo thủ: có thể chuyển về fallback khi câu hợp lệ nhắc lại thời hạn khách yêu cầu, và không nhận ra hết cách diễn đạt/phủ định. Không coi test giả lập hay regex là đánh giá ngữ nghĩa hoặc độ tự nhiên của model thật. Các repo ngoài THG_tool không sửa; chưa commit/push/deploy/import/approve.

## Kết quả mới: nối tri thức vào bản nháp model

- Pack hiện có 92 mục; thêm hướng dẫn chọn câu hỏi theo bước và giữ phạm vi/hiệu lực báo giá. 13 mục đủ tiêu chí public-only để preview, tất cả vẫn pending. Mức xác minh công khai không tự cấp approval.
- `go test ./internal/services/facebook ./internal/workspace_knowledge/runtime ./cmd/company_knowledge_preview ./internal/ai -count=1`: PASS. Gồm gửi public facts vào request model, lọc private/pending/hidden/khác org, ngân sách sản phẩm riêng, fallback khi model trả rỗng hoặc thêm giá/link, và bài tuyển trợ lý không tạo nháp sourcing.
- Lượt chạy rộng `go test ./internal/workspace_knowledge/... ./cmd/company_knowledge_preview ./internal/services/facebook ./internal/ai -count=1` đầu tiên: mọi package Knowledge OS và AI PASS; hai test mới ở CLI/Facebook FAIL. Đã sửa kỳ vọng public projection 14 thành 13 (một mục công khai có audience nội bộ), và sửa nhận nhầm virtual assistant thành sản phẩm. Sau đó chạy lại các package bị ảnh hưởng và PASS như trên.
- Sau khi tách helper test, các assertion ở Facebook/AI PASS nhưng Go thoát 1 vì Windows chặn xóa test binary trong Temp. Đã chạy lại `go test ./internal/services/facebook ./internal/ai -run "TestCompanyKnowledge|TestLeadReplyPrompt" -count=1` với GOTMPDIR trong toolcache workspace: PASS, exit 0.
- `go vet ./internal/workspace_knowledge/... ./cmd/company_knowledge_preview ./internal/services/facebook ./internal/ai`: PASS. `go build ./cmd/scraper ./cmd/worker`: PASS.
- Đánh giá truy xuất trên pack 92 mục: v1 17/24, v2 24/24; report được tạo lại với SHA256 của pack hiện tại. Đây vẫn là bộ hồi quy tổng hợp.
- `node scripts/knowledge/build-company-knowledge.mjs --check`, kiểm 186 liên kết Markdown nội bộ, `bash scripts/check_docs_governance.sh`, `git diff --check`: PASS.
- Đo bổ sung gocognit đã cache: mọi hàm trong các file Go thay đổi đều <=15, kể cả helper sourcing/shipping đã tách. Không thay kết luận về guard chính thức bị thiếu dependency ở lần trước.
- Đã chạy lệnh `-live-reply` với bài tổng hợp. Lệnh dừng rõ tại bước thiếu credential, không gọi provider, không gửi Telegram/Facebook hoặc ghi DB. **Chưa kiểm tra câu trả lời của model thật.**

Model prompt yêu cầu trả lời đúng nhu cầu, dùng ngôn ngữ bài gốc, hỏi phần còn thiếu và giữ các điều kiện trong nháp có căn cứ. Test HTTP giả lập xác nhận giao thức ghép context và fallback, không chứng minh chất lượng ngôn ngữ/ngữ nghĩa của LLM. Giá/cước supplier giữ đường dựng câu theo dữ liệu; chưa thay sang model.

Code và tài liệu chỉ thay trong THG_tool worktree, chưa commit/push/deploy, chưa import/approve pack. Các repo THG_Training, THG_SALE_WORKFLOW, THG_pricingtool, CMS_management- và THG_landingpage không đổi trong lượt này.

## Lịch sử kiểm tra trước khi nối model (pack 90 mục)

| Kiểm tra | Kết quả |
|---|---|
| `node scripts/knowledge/build-company-knowledge.mjs` | Sinh bản đọc từ nguồn JSON thành công |
| `node scripts/knowledge/build-company-knowledge.mjs --check` | 90 mục, 10 nhóm, 38 nguồn, 21 ví dụ đề xuất, 15 câu hỏi; JSON và bản sinh đồng bộ |
| `node --check scripts/knowledge/build-company-knowledge.mjs` | Cú pháp hợp lệ |
| `node scripts/knowledge/audit-training-sources.mjs --training-root D:/Projects/THG_Ecosystem/THG_Training` | 21 HTML khớp revision sau chuẩn hóa CRLF/LF; 39 tiêu đề phần đọc chọn lọc có trong file; lưu hash vào TRAINING_SOURCE_AUDIT.json |
| `node --check scripts/knowledge/audit-training-sources.mjs` | Cú pháp hợp lệ |
| Kiểm tra các link Markdown nội bộ bằng Node | Link tương đối và anchor ID của các mục được kiểm tra tồn tại |
| `go test ./internal/workspace_knowledge/... ./cmd/company_knowledge_preview -count=1` | PASS sau khi cho phép HTTP loopback của test giả lập; gồm cả ingestion và soak |
| `go test ./internal/workspace_knowledge/retrieval/... ./internal/workspace_knowledge/runtime/... ./cmd/company_knowledge_preview ./internal/services/facebook -count=1` | PASS; gồm ca hồi quy trên pack thật, ranh giới dữ liệu và luồng gợi ý Facebook |
| `go vet ./internal/workspace_knowledge/... ./cmd/company_knowledge_preview` | PASS |
| `go build ./cmd/scraper ./cmd/worker` | PASS |
| `go test ./internal/workspace_knowledge/... ./cmd/company_knowledge_preview ./internal/services/facebook -count=1` | PASS sau khi đơn giản hóa hàm; bao gồm Knowledge OS và luồng gợi ý Facebook |
| `go run ./cmd/company_knowledge_preview -review-draft -evaluate docs/business/company-knowledge/retrieval-cases.json` | Trên pack 90 mục có 26 mục gắn từ khóa tương đương: v1 đạt 17/24, v2 đạt 24/24; report có hash dữ liệu |
| `bash scripts/check_docs_governance.sh` | PASS; không có root doc sai chỗ, không thiếu governance doc |
| `git diff --check` | PASS |
| Đếm dòng các file CLI Go và script mới bằng Node | Tất cả dưới 200 dòng; chưa chạy check_file_size.py vì không có Python thật |
| `bash scripts/ai_preflight.sh` | Chạy tới import guard (5 cảnh báo ở file không sửa), dừng tại file-size vì máy thiếu Python; không ghi là toàn bộ preflight PASS |
| `bash scripts/go_cognitive_check.sh` | Chưa chạy trọn: tải dependency x/tools v0.23.0 bị timeout, dù gocognit v1.1.3 đã tải được |
| Đo bổ sung bằng CLI/core gocognit v1.1.3 đã cache, bỏ riêng plugin x/tools Analyzer | PASS ngưỡng 15 trên các file CLI, lexical, hybrid và runtime chạm tới; không thay guard chính thức hoặc coi guard chính thức đã PASS |

Git Bash ban đầu bị sandbox chặn CreateFileMapping; chạy lại cùng script chỉ đọc
ngoài sandbox đã thành công. Bộ test rộng ban đầu bị chặn socket HTTP 127.0.0.1
ở ingestion/soak; chạy lại ngoài sandbox đã PASS. Test runtime mới ban đầu có
fixture hai tài liệu hòa điểm; đã bổ sung tài liệu dịch vụ thứ ba để kiểm tra
trọng số tần suất trên corpus có từ chung lặp lại. Không thay cấu hình toàn máy.

Đo độ phức tạp bổ sung dùng cùng CLI và thuật toán Complexity của gocognit v1.1.3
trong `.workspace/toolcache/gocognit-stdlib`; chỉ bỏ phần tích hợp Analyzer cần
x/tools khỏi bản công cụ tạm. Không sửa cache module gốc hay script guard của repo.
Đã tách phần kiểm dữ liệu, lập chỉ mục và tính điểm; làm phẳng guard recorder có
sẵn để giữ hành vi nhưng giảm nhánh lồng. CI chính thức vẫn cần chạy trong môi
trường có đủ Python và dependency của gocognit.

Đã đối chiếu public website, quyết định trong trao đổi và phần liên quan của 21 tài
liệu Training trong repo. Chưa đọc SOP Training live, chưa đọc/duyệt đầy đủ điều
khoản dạng ảnh, chưa kiểm tra giá/SKU live. Go 1.26.8 dùng module/build cache local,
GOTOOLCHAIN=local và GOPROXY=off.

Các mục bổ sung làm sâu quy trình POD/Dropship/kho, thành phần phí, bao bì,
trạng thái giao nhận và phối hợp theo vai trò. Các trường điều kiện áp dụng,
đầu vào, bước xử lý, đầu ra và liên kết tri thức được kiểm tra cấu trúc và sinh
sang bản đọc. 21 ví dụ là tình huống tổng hợp để góp ý, chưa được owner duyệt
hoặc chạy đánh giá bằng LLM. Hướng dẫn văn phong cũng là đề xuất biên tập.

Lần rà tiếp bổ sung cách chọn luồng ship trực tiếp/qua kho, kiểm hiệu lực cước,
so sánh giá cùng phạm vi, xử lý supplier chậm, tiếp nhận khách POD và giới hạn tìm
bằng ảnh. Hai câu hỏi mới ghi rõ mơ hồ về cấu phần basecost và khác biệt mốc active
tracking giữa các snapshot. Chưa xác định bản nào đúng cho gói/line hiện hành.

Theo ủy quyền tiếp tục bằng phán đoán của người dùng, đã thêm hai hướng dẫn đề
xuất về diễn giải basecost theo báo giá và phân biệt các mốc tracking. Hai câu
hỏi tương ứng có `handling_state=interim_guidance_defined`, giữ nguyên nhu cầu
xác nhận điều khoản kinh doanh. Đã thêm ba tình huống về phí đã gồm, phí chưa rõ
và yêu cầu SLA không có bằng chứng. Validator kiểm tra hướng dẫn tạm thời phải
trỏ tới mục đề xuất nội bộ; không tự duyệt asset hoặc kích hoạt production.

Đã thêm chuẩn hóa Latin/Unicode, cặp từ liền nhau và trọng số tiêu đề/tag/mô tả
ở v2; giữ nguyên v1 để replay. Ca `dịch vụ kho Mỹ` từng lấy mục POD nay lấy kho Mỹ
đứng đầu, cả có dấu và không dấu. Bộ 24 ca là regression set tổng hợp trong quá
trình phát triển; không phải held-out benchmark, tỷ lệ đúng production hay bằng
chứng AI viết lời đáp tốt. Baseline 82 mục cũ vẫn được giữ riêng.

Ca catalog trong dataset ban đầu kỳ vọng một mục nội bộ ở chế độ public-facts;
đã sửa ca sang review và bổ sung kiểm tra từ chối expected ID không đủ điều kiện.
Không sửa bộ lọc quyền để thỏa kỳ vọng sai. Chưa chạy toàn bộ application suite,
chưa đánh giá LLM hoặc đọc ảnh. Chi tiết thuật toán/rollback ở
[tài liệu v2](../../engineering/KNOWLEDGE_MULTILINGUAL_RETRIEVAL.md).

Không sửa THG_Training, THG_SALE_WORKFLOW, THG_pricingtool, CMS_management- hoặc
THG_landingpage. Các repo nguồn được đọc để đối chiếu; không deploy/import/approve.

## Product link regression — 2026-10-20 (local only)

Reproduced before fixing: generic catalog/home pages and marketplace search/home
pages could be selected as product URLs; an invalid live offer could reach the
freight callback. Added `lead_product_links_test.go` and
`lead_reply_links_test.go`, and strengthened the POD marketplace-alternative test.

Implemented canonical marketplace listing checks in indexed/search/detail/final
lookup paths, catalog index/empty-title rejection, and exact selected-link repair
before generated-reply validation. Unrelated or changed URLs/prices still retain
the grounded draft. No usable source retains clarification/status without a URL.
Split the existing high-complexity supplier lookup into per-call bounded search
and linked-listing handling; request budgets, quota stop and cross-marketplace
fallback remain covered by the existing tests.

Passed with Go 1.26.8, `GOTOOLCHAIN=local`, `GOPROXY=off`, workspace module/build
caches, and local HTTP fixtures (no external model/source calls):

```text
go test ./internal/services/facebook ./internal/ai ./internal/ai/leadcontext ./internal/telegram/render ./internal/server/agent/crawlingest -count=1 -timeout=120s
go test ./internal/services/facebook -count=1 -timeout=120s
go vet ./internal/services/facebook ./internal/ai ./internal/ai/leadcontext ./internal/telegram/render ./internal/server/agent/crawlingest
bash scripts/check_docs_governance.sh
git -c core.safecrlf=false diff --check
```

The second Facebook run verifies the final search refactor. Supplementary
PowerShell file-size checks and the previously documented same-core gocognit
check also pass; they do not replace official CI. The full official validation
remains incomplete because Python is unavailable locally. No commit/push/deploy,
knowledge approval/import, real model, Elim, freight or Telegram delivery was
performed. Other repositories remain unchanged in this slice.
