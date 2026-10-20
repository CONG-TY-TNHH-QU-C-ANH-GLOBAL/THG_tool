---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-18
related_pr_or_issue: Add Taobao sourcing to replies
---

# Bộ tri thức công ty cho gợi ý trả lời THGFulfill

Bắt đầu ở [mục lục nội dung](INDEX.md). Bộ này giúp người và AI hiểu dịch vụ,
đọc nhu cầu khách, chọn phương án và biết phần nào phải xác nhận thêm.
Đây là nội dung nền tảng đã biên soạn có nguồn; chưa được nạp vào AI production.

## Nội dung và cách đọc

- **Dữ kiện có nguồn:** phần thực sự được đối chiếu. Website công bố khác với một
  cam kết áp dụng cho mọi đơn. Một số mục chỉ xác nhận hành vi code.
- **Cách tư vấn đề xuất:** suy luận/hướng dẫn biên tập, chờ owner duyệt; không tự
  trở thành chính sách công ty vì đã có trong file.
- **Chỉ hỏi khi thiếu:** gợi ý dữ kiện cần cho hành động tiếp theo, không phải
  danh sách bắt khách trả lời lại nếu họ đã cung cấp.
- **Điều cần tránh:** lời hứa/diễn giải dễ sai hoặc vượt quá bằng chứng đang có.
- **Nguồn:** link, phạm vi kiểm tra và phiên bản để tra ngược.

Mọi mục đều chờ duyệt theo quy trình runtime. Nhãn `public_reference` chỉ nghĩa
nội dung tham chiếu có thể lấy từ nguồn công khai; không nghĩa được phép tự động
gửi khách. Luồng reply mới chỉ nhận payload public đã duyệt; nhãn trong pack
không tự thay cơ chế ACL của những luồng Knowledge OS khác.

## Các file nguồn

| File | Vai trò |
|---|---|
| `knowledge.json` | Nội dung biên soạn chuẩn của bản draft; sửa tại đây |
| `sources.json` | Nguồn, thời điểm, phạm vi và giới hạn xác minh |
| `open-questions.json` | Câu hỏi còn mở, ảnh hưởng và cách xử lý khi chưa có thông tin |
| `response-examples.json` | 21 tình huống tổng hợp và lời đáp đề xuất, có liên kết tri thức |
| `INDEX.md`, `topics/*.md` | Bản dễ đọc sinh từ JSON |
| `SOURCES.md`, `OPEN_QUESTIONS.md` | Bản dễ đọc của nguồn và phần còn thiếu |
| `INTEGRATION.md` | Ranh giới với Knowledge OS/Training và cách nối vào hệ thống |
| `GLOSSARY.md` | Giải thích thuật ngữ dùng khi biên tập và tư vấn |
| `COVERAGE.md` | Bản đồ chiều rộng/chiều sâu, phần đã có và còn thiếu |
| `REPLY_STYLE.md`, `REPLY_EXAMPLES.md` | Cách diễn đạt và bản dễ đọc của tình huống đề xuất |
| `VALIDATION.md` | Các kiểm tra đã chạy và phạm vi chưa được kiểm chứng |
| `LOCAL_RETRIEVAL_RESULTS.json` | Kết quả tìm thử và trường hợp chưa lấy đúng ngữ cảnh |
| `retrieval-cases.json`, `RETRIEVAL_EVALUATION.json` | 24 ca truy xuất tổng hợp và kết quả so v1/v2 trên cùng dữ liệu |
| `TRAINING_SOURCE_AUDIT.json` | Hash snapshot và tiêu đề phần đã đọc; không phải chứng nhận SOP hiện hành |

Từ gốc repo, chạy:

```powershell
node scripts/knowledge/build-company-knowledge.mjs
node scripts/knowledge/build-company-knowledge.mjs --check
node scripts/knowledge/audit-training-sources.mjs --training-root D:/Projects/THG_Ecosystem/THG_Training
```

Lệnh kiểm tra liên kết nguồn, loại asset, trạng thái xác minh, tính đầy đủ của
các mục và sự đồng bộ JSON–Markdown. Nó không xác minh thay con người rằng một
lời công bố trên web còn là chính sách kinh doanh hiện hành.
Lệnh audit riêng chỉ đọc các HTML đã đăng ký và đối chiếu với Git revision;
chuẩn hóa CRLF/LF khi so nội dung. Nó không thay bước đọc và đánh giá nghiệp vụ.

## Thu thập và bổ sung tiếp

Nguồn đầu tiên đã dùng là website chính thức, quyết định người dùng chuyển tiếp
từ leader và contract trong repo. Đã đọc các phần liên quan của 21 tài liệu
Training có sẵn trong repo, gắn nhãn snapshot chưa xác nhận hiện hành. Chưa truy
cập nội dung Training live, tài khoản khách hay dữ liệu vận hành production.
Catalog động được ghi nhận là đầu mối;
không tạo danh sách SKU/giá giả khi chưa đọc được dữ liệu.

Bản hiện tại có 92 mục trong 10 nhóm, 38 nguồn và 21 tình huống diễn đạt đề xuất.
Các phần nghiệp vụ bổ sung có điều kiện áp dụng, đầu vào, bước xử lý, đầu ra và
liên kết sang chủ đề liên quan. Chúng giúp hiểu quy trình sâu hơn; không có nghĩa
toàn bộ nội dung công ty đã đầy đủ hoặc đã được duyệt.
26 mục có thêm từ khóa tra cứu tương đương, gồm tiếng Anh. Đây là metadata tìm
kiếm của nội dung đã có; không bổ sung lời hứa về dịch vụ hoặc điều khoản mới.

Ưu tiên bổ sung bản company profile mới nhất, SOP sales/service đang publish,
chính sách theo line có ngày hiệu lực, catalog được duyệt và câu trả lời mẫu
được owner duyệt. Các mục cụ thể ở [OPEN_QUESTIONS.md](OPEN_QUESTIONS.md).
Không cần chờ đủ mọi tài liệu để tiếp tục hoàn thiện các phần đã có căn cứ.

Hai điểm chưa thống nhất về basecost và active tracking đã có hướng dẫn tạm thời
theo ủy quyền dùng phán đoán của người dùng. Xem [cách tiếp tục](COVERAGE.md).
Việc biên soạn/kiểm thử tiếp tục; điều khoản hiện hành vẫn được ghi đúng là chưa xác nhận.

Nội dung Training/CMS được duyệt tiếp tục là nguồn chuẩn của SOP. Khi đưa draft
này lên hệ thống quản lý tri thức, gắn ID nguồn/revision của bản đã publish và
duy trì một bản chuẩn; không để bản Git và bản CMS cùng được coi là chính sách
hiện hành nhưng cập nhật độc lập.

## Cách dùng để soạn một gợi ý

1. Đọc chữ và ảnh thuộc bài, xác định nhu cầu và phần đã biết.
2. Chọn vài mục tri thức liên quan, giữ nguyên nguồn và điều kiện áp dụng.
3. Tra dữ liệu hiện hành cho sản phẩm/giá/cước khi cần; không lấy giá từ ví dụ.
4. Viết câu trả lời đúng ngôn ngữ, ngắn và chỉ hỏi phần thiếu cần thiết.
5. Sale xem lại phương án và cam kết trước khi gửi ra ngoài.

Tài liệu này chưa bổ sung chức năng đọc ảnh hoặc thay đổi thuật toán reply.
Kho tri thức cung cấp ngữ cảnh công ty; khả năng hiểu ảnh, chọn đúng dữ kiện và
tuân theo tri thức vẫn phải được kiểm thử riêng.

## Thử truy xuất ở local

Đã có công cụ `cmd/company_knowledge_preview` dùng bộ tìm kiếm hiện tại trên pack
trong bộ nhớ. Chế độ mặc định không lấy mục pending; bật review mới xem bản nháp,
và có tùy chọn chỉ xem dữ kiện công khai đã đối chiếu. Kết quả giữ riêng dữ kiện,
hướng dẫn đề xuất và nguồn. Xem lệnh chạy/phạm vi ở [INTEGRATION.md](INTEGRATION.md).
Đây là thử truy xuất, chưa phải AI tạo câu trả lời hay tích hợp production.
Phiên bản v2 đã sửa việc làm vỡ từ tiếng Việt, tìm được nội dung mô tả và giữ riêng
v1 để đối chiếu. Bộ ca hiện đạt 24/24 tiêu chí truy xuất tổng hợp; vẫn phải đánh
giá lời đáp AI và dữ liệu thực tế trước khi kết luận chất lượng tư vấn đạt yêu cầu.

Đã viết đường nối tri thức công khai đã duyệt vào model và preview gọi API local.
Xem [hướng dẫn và ranh giới tích hợp](INTEGRATION.md#đường-tích-hợp-đã-viết-chưa-phát-hành).
Nội dung pack vẫn pending/not_loaded, chưa import hoặc deploy.
