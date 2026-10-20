---
doc_type: engineering
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# Đưa bộ tri thức vào hệ thống hiện có

## Trạng thái thực tế

Đã xây nội dung, nguồn, câu hỏi cần xác nhận và bản đọc cho người/AI. Chưa tạo
source trong database, chưa import/approve asset hoặc deploy. Đã thêm đường nối
tri thức đã duyệt vào prompt và công cụ preview gọi model local; pack vẫn pending. JSON của bộ này là định dạng biên soạn, không phải payload
có thể POST thẳng vào một API hiện hành.

THG_tool đã có loại asset `faq`, `shipping_policy`, `sales_playbook`,
`pricing_rule`, `banned_claim`, `cta`, `POD_product`, `supplier_product`.
Pack chỉ dùng năm loại đầu; sản phẩm tiếp tục thuộc nguồn catalog riêng.
Không thêm loại asset, schema database, endpoint hay dependency trong thay đổi này.

## Tận dụng nguồn chuẩn đã có

- THG Training: có signed read-only export và ingestor `training_export` trong
  THG_tool. Code export lấy docs/doc_contents trong hệ thống cũ và các trang CMS
  published, theo scope sale/marketing. Nhánh docs không có trường publish/approval
  trong payload; vẫn phải kiểm tra từng SOP. Không dùng HTML tĩnh cũ thay nguồn quản lý.
- CMS/website: nội dung dịch vụ/chính sách công khai có nguồn, phiên bản/hiệu lực
  và phần cần đối chiếu khi tài liệu là ảnh hoặc có điều kiện chưa rõ.
- Catalog: tiếp tục pipeline đồng bộ sản phẩm/biến thể đã có, không tạo sản phẩm
  trong pack chỉ để có dữ liệu minh họa.
- Pricing Hub/CRM: tra số liệu động theo contract hiện hành. Pack ghi cách chọn
  nguồn và điều kiện, không thay bộ tính giá hay danh sách line được vận hành duyệt.

Snapshot kỹ thuật: THG_tool `b2a3cf6d`, THG_Training `ec170a949623`,
THG_landingpage `4fd55f1`. Đây là đối chiếu source, không phải xác nhận deploy.

## Các ranh giới phải xử lý trước khi bật sử dụng

1. Gắn đúng org/workspace; chưa có org ID trong pack và không suy từ tên nhóm Telegram.
2. Duyệt nội dung theo mục, người duyệt, phạm vi và hiệu lực. `public_source_checked`
   là bằng chứng đã đối chiếu nguồn, không phải `Asset.StateApproved`.
3. Lọc audience trước khi cấp ngữ cảnh cho bản nháp khách. Nhãn trong file/CSV
   không tự tạo phân quyền và prompt không thay bộ lọc dữ liệu.
4. Duyệt lại nội dung thay đổi. `Asset.MergeFromIngest` hiện giữ state approved khi
   tái đồng bộ; không dùng việc re-sync để âm thầm thay quy tắc đã duyệt bằng draft.
5. `banned_claim` hiện bị loại khỏi `CandidatesForLead`; muốn áp quy tắc cấm phải
   đi qua policy/context đúng chức năng, không nhập rồi giả định retrieval sẽ áp dụng.
6. `matchingCandidates` và `BuildGroundedFacts` hiện ưu tiên/giữ dữ kiện sản phẩm;
   chỉ nhập FAQ/playbook không bảo đảm các nhánh sourcing/service sẽ đọc chúng.
7. Tách ngân sách truy xuất sản phẩm và tri thức doanh nghiệp, tránh để các FAQ
   chiếm hết top-k ứng viên rồi làm mất sản phẩm phù hợp.

## Trình tự đưa nội dung vào sử dụng

1. Owner bổ sung/duyệt các mục đã có nguồn; giữ câu hỏi chưa rõ ở trạng thái chờ.
2. Chọn điểm quản lý canonical cho từng loại nội dung: SOP ở Training; sản phẩm
   ở catalog; giá ở provider; draft tổng hợp này là bản biên soạn có provenance.
3. Dùng ingestor có sẵn khi đúng contract. Nếu dùng CSV thì mỗi source chỉ một
   `asset_type`; tham chiếu `internal/workspace_knowledge/ingestion/csv`.
   Chưa sinh CSV để tránh coi metadata audience/review là cơ chế thực thi có sẵn.
4. Nối bản nhu cầu của lead với truy xuất tri thức liên quan. Dùng cùng snapshot
   ngữ cảnh cho việc chọn phương án và viết câu; giữ nguồn/điều kiện/thời điểm.
5. Kiểm thử một org local với nguồn giả: đúng phạm vi, không dùng mục pending,
   không lộ ghi chú nội bộ, không biến ví dụ thành giá và không phá retrieval sản phẩm.
6. So sánh trước/sau trên các ca POD, sourcing, chỉ vận chuyển/kho, bài ngoài phạm vi,
   nguồn lỗi và dữ liệu thiếu. Bổ sung ca có ảnh trước khi tuyên bố hiểu đa phương thức.
7. Sau đó phát hành theo repo và kiểm tra lead mới. Thu hồi các asset mới nếu có
   lỗi nội dung; rollback code riêng nếu lỗi nằm ở tích hợp. Ghi lại phiên bản trước đó.

## Công cụ kiểm tra truy xuất ngoại tuyến

`cmd/company_knowledge_preview` nạp pack vào bộ nhớ và chạy hybrid retriever hiện
có của THG_tool. Org ID 1 là giá trị giả chỉ tồn tại trong tiến trình, không trỏ
tới workspace hoặc database thật. Mọi asset luôn là pending.

```powershell
go test ./cmd/company_knowledge_preview -count=1 -v
go run ./cmd/company_knowledge_preview -query "nguồn hàng Taobao 1688"
go run ./cmd/company_knowledge_preview -review-draft -query "nguồn hàng Taobao 1688"
go run ./cmd/company_knowledge_preview -review-draft -public-facts-only -query "dịch vụ kho Mỹ"
go run ./cmd/company_knowledge_preview -review-draft -legacy-lexical -query "dịch vụ kho Mỹ"
go run ./cmd/company_knowledge_preview -review-draft -evaluate docs/business/company-knowledge/retrieval-cases.json
```

Lệnh mặc định phải không trả mục pending. `-review-draft` cho biên tập viên xem
thử, không duyệt nội dung. `-public-facts-only` chỉ lấy mục public_source_checked /
public_reference có facts, không đưa hướng dẫn đề xuất và ghi chú avoid vào kết quả.
Mỗi hit giữ nguồn và mức xác nhận. Banned-claim là policy riêng, bị hybrid loại;
report đếm số mục này để không nhầm rằng chúng đã được đưa vào ngữ cảnh.
Chế độ review còn hiển thị điều kiện áp dụng, đầu vào, bước xử lý, đầu ra và ID
liên quan nếu mục có các trường đó. Chế độ public-facts-only loại các trường
hướng dẫn này. Bộ ví dụ diễn đạt được giữ riêng, chưa đưa vào truy xuất như dữ kiện.

Lần chạy v1 ban đầu chưa đưa `service-warehouse` vào top 3 cho câu hỏi `dịch vụ kho
Mỹ`; giữ [baseline lịch sử](LOCAL_RETRIEVAL_RESULTS.json) để đối chiếu. Đã bổ sung
`hybrid.NewMultilingual` với trace `hybrid-v2-multilingual`, chuẩn hóa tiếng Việt,
từ/cặp từ liền nhau, trọng số theo tiêu đề/tag và nội dung mô tả. Hai constructor
runtime chọn v2 cho nhánh lexical; pgvector primary giữ hành vi cũ. `Tokenize` và
`hybrid.New` vẫn là v1 để tái hiện kết quả cũ. Preview mặc định v2; cờ
`-legacy-lexical` dùng để so sánh.

[Kết quả mới](RETRIEVAL_EVALUATION.json) trên cùng pack 92 mục: v1 đạt 17/24 và v2
đạt 24/24 tiêu chí truy xuất tổng hợp. Hai câu hỏi kho Mỹ có/không dấu yêu cầu
đứng đầu; các ca khác có top-k theo dataset. Report lưu hash pack và dataset.
Đây là bộ hồi quy trong quá trình phát triển, chưa phải đánh giá độc lập bằng lead
thật hoặc tỷ lệ thành công production. Xem [thiết kế, giới hạn và rollback](../../engineering/KNOWLEDGE_MULTILINGUAL_RETRIEVAL.md).

26 mục có `search_terms` với các thuật ngữ/diễn đạt tương đương, gồm tiếng Anh;
preview đưa vào Tags khi nạp trong bộ nhớ. Khi import thực tế cần ánh xạ có chủ
đích vào trường tag hiện có; chưa có thao tác import hoặc duyệt asset trong thay đổi này.

Chế độ truy xuất mặc định không gọi model. Chỉ cờ `-live-reply` mới cho phép gọi
provider theo cấu hình riêng. Công cụ không đọc ảnh, liên hệ Training/Elim/Telegram
hay ghi database. Chất lượng xếp hạng vẫn cần đo bằng ca hỏi thực; kết quả truy xuất
không chứng minh câu trả lời đã đúng. Luồng BuildLeadSuggestion đã được nối như dưới đây.

Không có lệnh ghi production trong bộ này. Việc tạo bộ tài liệu không chứng minh
AI đang dùng tài liệu; phải có bằng chứng truy xuất và đầu ra sau khi tích hợp.

## Đường tích hợp đã viết, chưa phát hành

`Builder.ReplyKnowledgeForLead` chỉ lấy approved asset đúng org và đúng loại,
qua bộ lọc source health hiện có. Payload phải có schema `company_reply_v1`,
audience `public_reference`, verification `public_source_checked`, facts và
source_ids. Chỉ facts và provenance đi vào prompt; title/description/SOP thô
không được chuyển sang câu trả lời khách. Đây là bộ lọc cho luồng này, không
thay cơ chế ACL của toàn bộ Knowledge OS.

`ProductCandidatesForLead` dành riêng ngân sách POD/supplier, còn các consumer
khác giữ `CandidatesForLead`. Nhánh POD và service/no-offer có draft có thể gọi
model với dữ kiện công ty; khi không có tri thức đủ điều kiện thì giữ fallback
hiện có (POD đã có sản phẩm vẫn được gọi model như trước). Supplier có offer
tiếp tục dựng câu theo dữ kiện, không để model sửa giá/cước/nhãn tương tự.

Pack được materialize trong preview với payload public-only nhưng vẫn pending.
Chưa có importer cho contract này, chưa có asset production được nạp/duyệt.
Không tự đổi pending thành approved chỉ để thử model.

### Gọi model thật với dữ liệu tổng hợp

```powershell
go run ./cmd/company_knowledge_preview -review-draft -public-facts-only -live-reply -author Lan -query "Looking for a shipping agent from China to the US, 200kg monthly"
```

Nếu có bất kỳ `LEAD_MODEL_*` nào, dùng override riêng đủ key/base URL/model như
[runbook model](../../engineering/LEAD_MODEL_PIPELINE.md); không mượn credential
comment khi override thiếu. Khi không có override, đọc cấu hình giống luồng comment:
`LLM_COMMENT_API_KEY`, fallback `LLM_API_KEY`
hoặc `OPENAI_API_KEY`; `LLM_COMMENT_BASE_URL`, fallback `LLM_BASE_URL`; model từ
`OPENAI_COMMENT_MODEL` (mặc định của app là `gpt-4.1`, không xác nhận model production).
Có thể dùng `-env-file` với đường dẫn file cấu hình riêng do người dùng cung cấp;
không in nội dung file/key/lỗi HTTP thô, không thay Git hoặc env toàn máy.
Chỉ dùng bài tổng hợp không có thông tin khách, không gửi Telegram/Facebook và
không ghi DB. Thời gian tối đa 45 giây; nhánh gợi ý hiện gọi một lần, không retry
và không chấp nhận completion bị cắt/refusal. Đây là preview phần retrieval → prompt → model, không phải chạy
toàn bộ ingestion/sourcing/shipping/Telegram. Kết quả thực của model phải đọc
và chấm thủ công về đúng ý, căn cứ, câu hỏi không lặp và ngôn ngữ tự nhiên.

Kiểm thử HTTP giả lập chứng minh việc ghép ngữ cảnh và fallback, không đo được
chất lượng ngôn ngữ của model thật. Guard giá/link/số cũng không chứng minh được
mọi cam kết mang ý nghĩa nghiệp vụ là chính xác. Chưa có cấu hình API local tại
lúc rà soát; chưa có kết quả model thật để kết luận chất lượng. API giữ giao thức
[Chat Completions hiện có](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create).

## Bổ sung kiểm soát ngày 19/10/2026

Runtime lấy tối đa 12 ứng viên công ty rồi chọn tối đa 4 mục đúng contract, thay
vì dừng ở 4 ứng viên đầu có thể toàn là FAQ cũ chưa có public projection. Vẫn giữ
org/state/type/source health và ngân sách context. Đây là giảm nguy cơ bị FAQ cũ
chiếm chỗ; không bảo đảm tìm đủ mọi mục trong một kho lớn hoặc thay truy xuất semantic.

Nhánh model chưa có quote/ảnh được kiểm tra thêm các mẫu thời gian định lượng,
miễn phí giao hàng và lời khẳng định đã đối chiếu ảnh. Số 200 trong cân nặng
không cho phép model nói giao 200 ngày. Bộ lọc này bảo thủ: có thể giữ fallback
khi model nhắc lại một thời hạn khách yêu cầu; chưa hiểu được mọi diễn đạt hay
phủ định. Mặc định supplier vẫn dựng câu theo dữ kiện riêng; khi bật phân tích
nhu cầu, có thể viết lại rồi kiểm tra giữ giá, nhãn và điều kiện cước, lỗi giữ nháp cũ.

File mẫu cấu hình: `scripts/knowledge/model-preview.env.example`. Bản trống riêng
đã chuẩn bị local tại `tmp/company-knowledge-model/.env`; Git bỏ qua cả thư mục tmp.
Nhờ người phụ trách AI điền trực tiếp API key, API base URL và tên model hiện dùng
vào bản riêng; không điền vào file example có thể commit và không gửi key qua chat.
Sau đó có thể thử bằng lệnh (chạy từ worktree này):

```powershell
go run ./cmd/company_knowledge_preview -review-draft -public-facts-only -live-reply -env-file tmp/company-knowledge-model/.env -author Lan -query "Looking for a shipping agent from China to the US, 200kg monthly"
```

File cấu hình local không phải bằng chứng model đã được gọi. Tại lúc chuẩn bị,
không có credential trong tiến trình; file riêng chỉ là bản mẫu trống.

## Nối ngữ cảnh ảnh

Đã thêm luồng đọc text cùng ảnh tham chiếu trước khi retrieval/sourcing, mặc định
tắt bằng `LEAD_IMAGE_CONTEXT_ENABLED`. Contract mới là `image_urls` tùy chọn từ
extension; quan sát ảnh được giữ riêng với company facts và product facts đã xác
minh. Có mâu thuẫn thì hỏi lại; lỗi/thiếu ảnh vẫn giữ nháp theo chữ và note cho sale.
Không coi ảnh là nguồn giá, trọng lượng hoặc xác nhận đúng listing.
Xem [luồng, giới hạn, kiểm thử và rollback](../../engineering/LEAD_IMAGE_CONTEXT.md).
Đây là code local đã kiểm thử bằng model giả lập; chưa có kết quả đọc ảnh bằng
model thật, chưa phát hành extension/backend hoặc bật cấu hình production.

## Nối model riêng và phân tích nhu cầu

`LEAD_CONTEXT_ANALYSIS_ENABLED` mặc định false. Nhánh này đọc nhu cầu từ chữ và
ảnh (nếu được bật), giữ bằng chứng nguyên văn từ bài, rồi dùng các nguồn dữ kiện
hiện có. Log có mã phân biệt model viết được, model lỗi/bị chặn, thiếu public
knowledge hoặc dùng template. Telegram có `ReplyNote` riêng khi phân tích chưa
xong/nhu cầu chưa rõ/bài không có nhu cầu thương mại.

`-live-pipeline -review-draft -public-facts-only` chạy cùng facade gợi ý với
public facts trong bộ nhớ ở trạng thái approval **mô phỏng**, không approve pack,
không tìm nguồn thật và không gửi tin. File cấu hình trống riêng đã chuẩn bị ở
`tmp/company-knowledge-model/muse.env`; không có API key trong ví dụ tracked.
Xem [hướng dẫn, bộ ca nghiệm thu và các giới hạn](../../engineering/LEAD_MODEL_PIPELINE.md).

### Quyết định hiện hành: dùng model/API hiện có

Ngày 19/10/2026, task owner tạm gác Muse Spark. Giai đoạn tiếp theo dùng provider
và model đang cấu hình, tập trung hiểu đúng nhu cầu, lấy đúng tri thức/dữ kiện và
kiểm tra lời gợi ý. Không cần tạo tài khoản Meta hoặc điền `muse.env` lúc này.
Preview dùng file riêng `tmp/company-knowledge-model/.env` theo template chung;
không tự sao chép secret từ production hoặc ghi đè file riêng đã có.
Các phần phân tích nhu cầu, kiểm tra dữ kiện và fallback độc lập nhà cung cấp vẫn
được giữ. Khả năng đọc ảnh của model thực tế cần xác minh trước khi bật nhánh ảnh.
