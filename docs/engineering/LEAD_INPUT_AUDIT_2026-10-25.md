---
doc_type: engineering
status: draft
owner: platform
last_reviewed: 2026-10-25
related_pr_or_issue: Add Taobao sourcing to replies
---

# Rà soát đầu vào chữ và ảnh cho gợi ý trả lời

## Phạm vi và kết luận

Đối chiếu `origin/main` tại `e80c388c` và trace extension → ingest → event →
request model. Không đọc nội dung khách, URL ảnh có chữ ký, key hoặc phản hồi
model thật. Audit production chỉ xuất số đếm và trạng thái cố định.

**Chưa thể xác nhận bước này đã đúng hoàn toàn.** Có đường truyền chữ và ảnh
cùng bài; hai lỗi chọn dữ liệu được tái hiện và sửa trong branch audit. Phần
tách chữ vẫn có giới hạn về nguồn dữ liệu; đọc ảnh thật hiện chưa đạt.

## Luồng thực tế trong code

1. `local-connector-extension/content/crawl.js` lấy toàn bộ `textOf(article)`
   và ảnh qua `THGCrawlImages.fromArticle(article)` cho cả broad/direct crawl.
   `content/shared.js` gộp mọi khoảng trắng và xuống dòng thành dấu cách.
2. Item giữ `content`, `image_urls`, người đăng và permalink. Bộ nhận chuyển
   danh sách ảnh của từng item vào `leadingest.Input`, không dùng ảnh cấp batch.
3. Trước khi có gợi ý, ingest có nhánh AI đánh giá bài tùy cấu hình:
   `runAIClassifier` gọi `UniversalClassify` với chữ thô, người đăng và profile;
   nhánh này không nhận danh sách ảnh và không gọi `leadPostBody`.
4. Khi tạo lead, event giữ chữ gốc đầy đủ và một slice ảnh đã lọc riêng.
   Cả scraper và worker truyền event vào nhánh tạo gợi ý khi cờ/org cho phép.
5. `BuildLeadSuggestionWithContext` gọi `leadPostBody`, kiểm lại `media.URLs`,
   rồi `AnalyzeLeadContext`. Không có bộ lọc từ khóa trước model ở facade này.
6. Request có một user message chứa một phần text prompt và N phần `image_url`.
   Chữ được sanitize/cap 6000 ký tự Unicode. Không gửi cookie đăng nhập, không
   tải ảnh về server hay biến screenshot toàn màn hình thành ảnh sản phẩm.
7. Kết quả phải đúng schema/evidence trước khi dùng. Không có bảo đảm model
   hiểu đúng chỉ vì request đã chứa URL hoặc JSON đầu ra hợp lệ.

## Phát hiện và trạng thái

### 1. Một số tiêu đề Facebook còn lẫn vào chữ — đã sửa trong branch

Regex cũ bỏ sót tiêu đề có Follow/Theo dõi, dấu chấm giữa trước timestamp,
hoặc Just now/Vừa xong. Test với tên giả tái hiện năm trường hợp thất bại.
Tiêu đề còn tiếng Việt có thể làm sai nhận diện ngôn ngữ bài tiếng Anh.

Sửa trong `internal/services/facebook/lead_post_body.go`: cho phép tối đa hai
nhãn trạng thái đã biết, dấu phân cách và dạng timestamp vừa xong. Các test
bảo toàn câu văn thông thường, thời gian giao hàng và ranh giới bình luận cũ
vẫn phải đạt. Không đổi raw excerpt, điểm số hay điều kiện tiếp nhận bài.

### 2. Khối FeedUnit có thể trộn ảnh article ngang hàng — đã sửa trong branch

Bộ chọn cũ cho qua mọi article không có article cha, kể cả article thứ hai
trong cùng wrapper. Test DOM giả tái hiện ảnh của bài khác lẫn vào kết quả.

Sửa trong `platforms/facebook/crawl_images.js`: article gốc không nhận ảnh
article con; wrapper chỉ nhận ảnh của article đầu tiên. Test bảo toàn ảnh gốc,
loại bình luận lồng và loại article ngang hàng. Đây là bằng chứng logic trên
fixture, chưa phải xác minh mọi dạng DOM Facebook thực tế. Máy quét phải nạp
extension chứa bản sửa mới dùng được logic này.

### 3. Tách chữ bằng nhãn UI còn mơ hồ — chưa giải quyết bằng hai sửa trên

Toàn bộ article vẫn chứa cả bài, giao diện và bình luận, đã bị gộp dòng.
`leadPostUIBoundary` cắt ở các cụm như `see translation` hoặc `xem bản dịch`.
Nếu chính người đăng dùng cụm này trong câu văn, phần phía sau cũng có thể bị
cắt. Nếu bình luận không có mẫu ranh giới mà regex nhận biết, nó có thể còn
lẫn vào chữ. Không thể khẳng định mọi nguồn chữ đã sạch bằng vài regex.

Extension chưa có bước mở rộng riêng nội dung bị thu gọn bằng See more/Xem thêm.
Phần chưa hiện trong DOM không thể được server khôi phục. Nhãn thiếu/thu gọn,
người đăng sai và nội dung >6000 ký tự cần có kiểm chứng chất lượng riêng.
`authorFromArticle` cũng quét heading/strong/b trên toàn khối; chưa có đủ
bằng chứng DOM thật để bảo đảm mọi tên tác giả được chọn đúng. Sửa chọn ảnh
ở trên không tự tách riêng chữ hoặc sửa metadata người đăng.

Hướng xử lý tiếp theo: lấy nội dung người đăng từ cấu trúc DOM riêng, lưu tách
raw excerpt, giữ trạng thái xuất xứ/độ đầy đủ và dùng heuristic cũ cho extension
cũ. Cần contract tương thích và test các kiểu DOM, không thay raw content vì
nó đang ảnh hưởng intake/dedupe. Không tự chuyển văn bản thành kết luận sản phẩm.

### 4. URL hợp lệ chưa chứng minh model đọc được ảnh — còn lỗi runtime

Ảnh giới hạn 3 URL HTTPS Facebook CDN, loại trùng, giới hạn URL 4096 ký tự,
không userinfo/port/fragment. Extension yêu cầu ảnh hiển thị ít nhất 120×120,
link photo hợp lệ và loại vùng header/comment. Các giới hạn này có thể bỏ ảnh
chưa tải, ảnh nhỏ hoặc kiểu DOM chưa hỗ trợ. Server kiểm URL, không xác minh
bytes ảnh, hạn chữ ký, quyền truy cập từ provider, chất lượng hay đúng SKU.

Diagnostic `40160151361`, kiểm tra lúc 03:58:30 UTC / 10:58 ngày 25/10 giờ VN,
dùng cửa sổ 2880 phút: 10 sự kiện gợi ý, 6 có ảnh, 6 image_analysis=analysis_failed,
0 observations, 4 analysis_error=timeout. Các lỗi lịch sử không có mã mới không
thể suy ngược nguyên nhân. Cả hai binary vẫn khớp `e80c388c`, active, context và
image bật, org cho phép. Pricing Hub ready chỉ chứng minh kiểm tra quota phản
hồi được, không chứng minh có số dư hay đã tìm nguồn thành công.

`requestLeadAnalysis` hiện có deadline 5 giây, không retry. Có bằng chứng timeout
thực tế; chưa có bằng chứng rằng tăng deadline sẽ đủ hoặc model hiện tại đọc
được ảnh. Cần trial ảnh được phép dùng, kiểm provider/model và đo latency trước
khi đổi budget. Audit này không đổi model, key, deadline hay retry policy.

## Kiểm thử và giới hạn bằng chứng

- RED: năm dạng header không được bỏ và ảnh article ngang hàng bị nhận nhầm.
- GREEN: `go test ./internal/services/facebook ./internal/platform/facebook/media
  ./internal/ai ./internal/leadingest ./internal/server/agent/crawlingest
  -count=1 -timeout=120s`; `go vet` cùng năm package.
- Fake HTTP kiểm tra JSON request thật: chỉ chữ của người đăng, bỏ comment và
  header; cùng request có ba URL đã lọc đúng thứ tự, giữ nguyên signed query.
- Node tests kiểm image ownership, bộ lọc hiện có và result wire compatibility.
- Các test dùng HTTP/DOM giả không chứng minh provider truy cập hay hiểu ảnh.
  Không chạy lại lead cũ, không gửi thông báo Telegram giả hoặc bài Facebook.

## Ranh giới phát hành

Hai sửa nhỏ không có migration/DTO mới. Chúng không thay classifier đầu vào,
nguồn tri thức, truy vấn Elim, cước hay điều kiện thông báo. Server mới vẫn nhận
extension cũ; sửa chọn ảnh cần cài/nạp extension mới ở máy quét. Không coi phần
này hoàn tất chỉ vì test/CI đạt. Cần quan sát lead mới có ảnh và kiểm đúng nội
dung request, xuất xứ ảnh và kết quả phân tích trước khi chốt chất lượng.
