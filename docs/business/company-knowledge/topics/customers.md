---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 03. Khách hàng và nhu cầu

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="customer-segments"></a>

## Các nhóm khách được website nhắc tới

Mã: `customer-segments` · Loại: `faq` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Website hướng tới seller TMĐT, người mới bắt đầu, seller mở rộng và thương hiệu/DTC.

**Cách tư vấn / xử lý đề xuất**

- Dùng nhóm khách để chọn cách giải thích, không dùng để suy ngân sách hoặc kinh nghiệm của một cá nhân. Khách mới cần bước bắt đầu; khách đang vận hành cần giải pháp cho điểm nghẽn cụ thể.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách mới bắt đầu hay đang có đơn?

**Điều cần tránh**

- Cam kết lợi nhuận hoặc doanh số cho khách.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi).

<a id="customer-demand-vs-offer"></a>

## Phân biệt người cần dịch vụ và người đang chào bán

Mã: `customer-demand-vs-offer` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Đọc chủ thể và hành động của tác giả: tìm/mua/nhờ báo giá khác với đang bán/nhận dịch vụ/quảng cáo. Nội dung bình luận không thay thế nhu cầu tác giả. Kết hợp phân loại upstream với bằng chứng trong bài.

**Điều cần tránh**

- Chào mua hàng cho nhà cung cấp chỉ vì bài có tên sản phẩm.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="customer-out-of-scope"></a>

## Bài không phải nhu cầu THG trong task này

Mã: `customer-out-of-scope` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Bài tuyển trợ lý ảo, chia sẻ kinh nghiệm hoặc quảng cáo không tự trở thành nhu cầu sourcing vì có Amazon/dropship. Với workspace THGFulfill, ghi lý do ngoài phạm vi nếu không có nhu cầu dịch vụ phù hợp; không áp quy tắc ngành này sang mọi tenant khác.

**Điều cần tránh**

- Bịa một câu chào bán để lấp phần gợi ý trống.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="customer-intent-evidence"></a>

## Lưu bằng chứng cho nhu cầu

Mã: `customer-intent-evidence` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Mỗi dữ kiện hiểu từ bài cần gắn với câu chữ hoặc ảnh liên quan. Phân biệt khách nói rõ, suy luận có điều kiện và chưa biết. Từ phủ định, điều kiện, nhu cầu người khác và câu hỏi giả định phải được giữ trong ngữ cảnh.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Chỉ hỏi điều đang cản trở bước hỗ trợ tiếp theo.

**Điều cần tránh**

- Coi điểm tự tin do AI tự chấm là bằng chứng xác thực.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="training-customer-scope-conflict"></a>

## Phạm vi seller và khách mua hàng cá nhân còn cần chốt

Mã: `training-customer-scope-conflict` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Tài liệu tổng quan trong repo hướng tới seller và loại nhu cầu gửi quà cá nhân; trang order lại hướng tới người Việt tại Mỹ mua hàng Trung Quốc.

**Cách tư vấn / xử lý đề xuất**

- Đây là khác biệt phạm vi theo nguồn/dịch vụ cần owner xác nhận. Phân biệt khách mua hàng hộ và khách chỉ gửi quà; không tự từ chối mọi cá nhân hoặc hứa nhận mọi nhu cầu cá nhân.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Dịch vụ order hiện nhận nhóm khách nào và điều kiện gì?

**Điều cần tránh**

- Đưa một mô tả snapshot thành quy tắc loại khách toàn công ty.

Nguồn: Training: công ty, khách hàng, bốn mảng dịch vụ (training-company-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); [Đặt hàng Taobao/1688 giao Mỹ](https://thgfulfill.com/vi/thg-order).
