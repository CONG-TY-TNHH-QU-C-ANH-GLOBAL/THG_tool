---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-18
related_pr_or_issue: Add Taobao sourcing to replies
---

# Cách viết gợi ý trả lời đúng ý và tự nhiên

Đây là hướng dẫn biên tập đề xuất theo mục tiêu người dùng đã nêu, chưa phải
SOP hoặc prompt đã chạy production. Nội dung công ty vẫn phải có nguồn, đúng
phạm vi và được duyệt. Lời văn trôi chảy không bù được dữ kiện thiếu hoặc sai.

## Trước khi viết

1. Tách bài của tác giả khỏi bình luận, menu và quảng cáo do công cụ quét lấy kèm.
2. Xác định tác giả đang cần mua hàng, in, vận chuyển, kho, hỗ trợ đơn hay việc khác.
   Một bài có từ Amazon hoặc dropship chưa đủ để chọn dịch vụ.
3. Ghép text và ảnh gốc nếu đã thực sự đọc được ảnh. Giữ chỗ chưa rõ, không đoán
   model, vật liệu, khối lượng hay khả năng vận chuyển từ hình dáng.
4. Ghi những gì đã biết; chọn tri thức và dữ liệu mới cần cho đúng câu hỏi.
5. Xác định người đọc: thông tin nội bộ sale và bản nháp gửi khách có phạm vi khác
   nhau. Không chuyển nguyên thẻ giá nhập, lỗi API hoặc ghi chú nội bộ cho khách.

## Viết từ nhu cầu của khách

- Mở bằng lời giải đáp, phương án hoặc điểm cần làm rõ liên quan trực tiếp.
- Dùng từ phổ thông: “tìm nguồn”, “phí xử lý đơn”, “gửi nguyên lô hay từng đơn”.
  Nếu cần dùng thuật ngữ, giải thích ngắn ngay lần đầu.
- Thường 2–3 câu là đủ; đây là mục tiêu biên tập, không phải giới hạn cứng làm mất
  điều kiện quan trọng. Mỗi câu có một việc chính.
- Hỏi 1–2 nhóm dữ kiện có ích nhất cho bước kế tiếp. Bộ đầu vào nghiệp vụ trong
  kho tri thức không phải bảng câu hỏi phải gửi hết cho khách.
- Giữ nguyên chi tiết khách đã nêu: nước nhận, vật liệu, số lượng, ngân sách và
  yêu cầu không in. Không hỏi lại trừ khi có mâu thuẫn cần xác nhận.
- Dùng đúng mức chắc chắn: “bảng đang niêm yết…” khi đã tra; “cần kiểm tra…” khi
  chưa có căn cứ; “mẫu tương tự” khi chưa xác minh đúng model.
- Viết theo ngôn ngữ bài gốc và cách xưng hô phù hợp. Giao diện Facebook tiếng
  Việt không biến bài tiếng Anh thành bài tiếng Việt.
- Kết thúc bằng bước tiếp cụ thể. Xin link/mã mẫu nếu cần xác minh, hoặc xin thông
  tin đơn qua kênh riêng nếu xử lý sự cố.

## Những cách viết thường làm lời đáp lệch trọng tâm

| Biểu hiện | Cách chỉnh |
|---|---|
| Khen bài, nhắc lại toàn bộ nội dung rồi mới chào dịch vụ | Trả lời ngay câu hỏi hoặc đề xuất bước có ích |
| Câu nào cũng kết “inbox để được tư vấn” | Nêu thông tin nào cần trao đổi và để làm gì |
| Bài hỏi ship nhưng trả sản phẩm catalog | Giữ việc tính cước là trọng tâm; chỉ sourcing khi có nhu cầu |
| Chưa đủ dữ kiện nhưng báo số cụ thể | Hỏi đúng biến thiếu; giữ giá/cước ở trạng thái chưa xác nhận |
| Đưa tên lỗi kỹ thuật vào lời nhắn khách | Ghi lỗi trong trạng thái nội bộ; lời đáp nói phần đang kiểm tra |
| Thêm quá nhiều câu miễn trừ | Chỉ nêu điều kiện ảnh hưởng quyết định hoặc mức giá đang tư vấn |
| Đổi tên khách rồi lặp cùng một đoạn cho mọi bài | Viết theo nhu cầu, phần đã biết và bước kế tiếp của từng bài |

## Thẻ nội bộ và bản nháp khách

Thẻ sale có thể chứa nguồn sàn, giá nguồn, MOQ, độ khớp, cước tham chiếu có điều
kiện và trạng thái tra cứu theo quyết định task. Mỗi giá cần đúng đơn vị, số lượng
áp dụng, tiền tệ và thời điểm. Không có kết quả thì nói rõ chưa có.

Bản nháp khách ưu tiên câu trả lời có ích. Không tự chuyển giá nhập thành giá bán
hoặc cam kết tuyến chưa xác nhận. Quyết định cho phép giá nhập trong nhóm Telegram
không tự cấp quyền tiết lộ mọi thông tin nội bộ của công ty.

Khi chưa thống nhất cách gọi basecost, giải thích theo phạm vi báo giá cụ thể:
khoản đã gồm không cộng lại, khoản chưa rõ không tự xem là bằng 0. Khi khách hỏi
tracking, trả lời đúng mốc có bằng chứng; tạo mã, hãng nhận kiện và cập nhật trên
nền tảng cần được kiểm tra riêng. Hướng dẫn tạm thời này cho phép tiếp tục biên
soạn mà không tự đặt giá, phụ phí hoặc thời hạn 24/48 giờ.

## Đánh giá một câu trả lời

Xem [21 ví dụ đề xuất](REPLY_EXAMPLES.md). Chấm từng tiêu chí 0–2: 0 sai/thiếu rõ,
1 đạt một phần, 2 đạt. Ghi nhận bằng chi tiết câu trả lời, không chỉ bằng cảm giác.

| Tiêu chí | Câu hỏi kiểm tra |
|---|---|
| Đúng nhu cầu | Có trả lời điều tác giả đang hỏi hay chuyển sang bán dịch vụ khác? |
| Đúng căn cứ | Dữ kiện công ty, offer và giá có nguồn phù hợp, đủ mới, đúng điều kiện? |
| Đúng ngữ cảnh | Có giữ nơi nhận, vật liệu, số lượng và phần text/ảnh đã được xác minh? |
| Có ích | Đã giải đáp được phần có thể trả lời và chỉ hỏi phần thiếu cần thiết? |
| Gọn, dễ đọc | Mỗi câu có ý rõ, không lặp hoặc nhồi thuật ngữ? |
| Tự nhiên | Ngôn ngữ/xưng hô phù hợp, câu nối trôi chảy, không dùng mẫu máy móc? |

Sai sản phẩm bắt buộc, bịa giá/link, hứa dịch vụ chưa có căn cứ, lộ thông tin nội
bộ hoặc khẳng định đọc ảnh khi chưa đọc là lỗi phải sửa dù tổng điểm cao. Chưa
đặt ngưỡng nghiệm thu số học; cần owner duyệt bộ ca và chuẩn đầu ra trước.

Bài ngoài phạm vi có thể không tạo bản nháp. Bài hợp lệ thiếu dữ kiện vẫn cần
lời đáp phù hợp và trạng thái giải thích; không dùng một câu hỏi chung cho tất cả.
