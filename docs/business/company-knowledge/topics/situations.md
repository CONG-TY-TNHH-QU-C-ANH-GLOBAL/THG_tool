---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 09. Tình huống để luyện và kiểm tra

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="case-seasonal-supplier"></a>

## Tình huống: tìm nhà cung cấp hàng mùa đông

Mã: `case-seasonal-supplier` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Đầu vào: Looking for a reliable supplier for winter products. Hiểu: tìm nguồn, chưa biết loại hàng cụ thể; không chọn tùy tiện áo POD. Hành động: hỏi loại hàng, lượng mua và điểm đến còn thiếu.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Which winter products are you looking for, and roughly how many units? Where would you like them delivered?

**Điều cần tránh**

- Khẳng định đã tìm thấy hàng đúng nhu cầu khi winter products còn quá rộng.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="case-uae-supplier"></a>

## Tình huống: supplier ship UAE

Mã: `case-uae-supplier` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Đầu vào: Looking for a reliable supplier/agent shipping to UAE. Hiểu: điểm đến đã biết, sản phẩm chưa rõ. Hành động: hỏi sản phẩm và số lượng; kiểm tra khả năng hỗ trợ điểm đến trước khi hứa tuyến.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Which products are you sourcing, and roughly how many units?

**Điều cần tránh**

- Hỏi lại destination hoặc tự gắn một nguồn hàng không liên quan.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="case-dog-supplement"></a>

## Tình huống: viên bổ khớp cho chó, 300 hộp/tháng đi Mỹ

Mã: `case-dog-supplement` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Hiểu: sản phẩm cho chó, sản lượng theo tháng và điểm đến đã biết. Tra đúng link/mẫu nếu có, kiểm giá nguồn theo lượng mua thật. Kiểm tra loại hàng/chính sách tuyến và kiện; không tự áp cước hàng thường hoặc coi 300 hộp là một kiện.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Quy cách đóng gói và lượng gửi mỗi lần; model/link chỉ khi chưa đủ nhận diện.

**Điều cần tránh**

- Chép lại một mức cước trong ảnh minh họa cũ thành giá hiện tại.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="case-pod-alternative"></a>

## Tình huống: cần túi cotton in logo, catalog chỉ có polyester

Mã: `case-pod-alternative` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Hiểu: vật liệu cotton và yêu cầu in là hai điều kiện. Tìm catalog phù hợp; nếu không có, được tra phôi cotton trên sàn như nguồn thay thế. Xác nhận khả năng in trước khi báo giá. Polyester chỉ được nêu như lựa chọn khác có ghi khác biệt.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khả năng chấp nhận vật liệu thay thế nếu cần.

**Điều cần tránh**

- Đổi cotton thành polyester mà vẫn nói phù hợp đúng yêu cầu.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="case-photo-only-product"></a>

## Tình huống: cần mẫu này kèm ảnh

Mã: `case-photo-only-product` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Đầu vào: cần mẫu này, 300 cái/tháng gửi Mỹ và ảnh. Giữ số lượng theo kỳ/nơi nhận từ text, dùng ảnh nhận loại hàng/model nếu đủ rõ. Khi chưa đọc được ảnh, không tự đoán một sản phẩm cụ thể hoặc hứa đã tìm đúng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Ảnh nhãn/mã rõ hơn nếu việc đó cần để phân biệt mẫu.

**Điều cần tránh**

- Tự suy cân nặng, pin hoặc thành phần bên trong từ dáng bao bì.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="case-shipping-known-product"></a>

## Tình huống: ship hoodie đi Mỹ, đã nêu 200kg/tháng

Mã: `case-shipping-known-product` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Hiểu: chỉ cần vận chuyển, loại hàng và sản lượng tháng đã có. Hỏi nơi lấy, quy cách từng lần gửi/kích thước còn thiếu và kiểm tra tuyến. Không đi tìm hoodie để bán thêm hoặc xin lại loại hàng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Hàng lấy ở đâu và mỗi lần gửi đóng thành bao nhiêu kiện, kích thước thế nào?

**Điều cần tránh**

- Tự coi 200kg/tháng là trọng lượng một chuyến.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).
