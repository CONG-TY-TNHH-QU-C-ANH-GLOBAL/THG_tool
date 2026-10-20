---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 01. Công ty và phạm vi năng lực

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="company-identity"></a>

## Công ty là ai?

Mã: `company-identity` · Loại: `faq` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- company profile
- THG Fulfill
- Transport Happiness Group

**Dữ kiện có nguồn**

- Website thgfulfill.com giới thiệu THG Fulfill/Transport Happiness Group và ghi pháp nhân CÔNG TY TNHH QUỐC ANH GLOBAL.

**Cách tư vấn / xử lý đề xuất**

- Dùng tên doanh nghiệp từ hồ sơ workspace đã được duyệt khi viết lời chào. Giữ riêng tên thương hiệu, tên pháp nhân và tên dịch vụ.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Tên thương hiệu chuẩn để dùng trong lời nhắn với khách.

**Điều cần tránh**

- Tự suy giấy phép, chứng nhận, số năm hoạt động hoặc vị trí dẫn đầu.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi).

<a id="company-ecosystem"></a>

## THG giải quyết chuỗi việc gì?

Mã: `company-ecosystem` · Loại: `faq` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- service overview
- end to end fulfillment

**Dữ kiện có nguồn**

- Các nhóm dịch vụ được công bố gồm sourcing, POD, kho/fulfillment và vận chuyển.

**Cách tư vấn / xử lý đề xuất**

- Xác định khách đang cần khâu nào và đề xuất khâu đó trước. Có thể ghi nhận nhu cầu kết hợp nhiều dịch vụ, nhưng chỉ trình bày phần liên quan.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách đã có hàng, có nhà cung cấp hay mới đang tìm sản phẩm?

**Điều cần tránh**

- Giới thiệu cả hệ sinh thái dài dòng trong mọi câu trả lời.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi).

<a id="company-locations"></a>

## Mạng lưới và địa điểm

Mã: `company-locations` · Loại: `faq` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- warehouse locations
- service locations

**Dữ kiện có nguồn**

- Trang giới thiệu công bố hiện diện tại Việt Nam, Trung Quốc và Mỹ; trang kho Mỹ nêu Pennsylvania và North Carolina.

**Cách tư vấn / xử lý đề xuất**

- Xác nhận địa chỉ tiếp nhận và mã nhập kho trước khi hướng dẫn khách gửi hàng. Có địa điểm trong mạng lưới không chứng minh còn chỗ trống hoặc phù hợp mọi loại hàng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Địa điểm nhập/xuất được vận hành xác nhận cho yêu cầu này.

**Điều cần tránh**

- Suy có kho tại châu Âu chỉ vì có tuyến giao châu Âu.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi); [Dịch vụ kho Mỹ](https://thgfulfill.com/vi/thg-warehouse).

<a id="company-capability-evidence"></a>

## Năng lực công bố khác cam kết cho một đơn cụ thể

Mã: `company-capability-evidence` · Loại: `banned_claim` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Mỗi lời hứa về tồn kho, hạn giao, công suất, QC, bồi thường hoặc tích hợp cần bằng chứng áp dụng cho dịch vụ và đơn hiện tại. Nội dung quảng bá chỉ cung cấp bối cảnh để hỏi và kiểm tra.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Điều kiện áp dụng và người có thẩm quyền xác nhận.

**Điều cần tránh**

- Khẳng định mọi hàng đều nhận, mọi đơn đúng hẹn hoặc mọi mất mát đều được bồi thường toàn bộ.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); [Giới thiệu THG Fulfill](https://thgfulfill.com/vi).

<a id="training-company-network"></a>

## Đối chiếu tổng quan công ty trong Training

Mã: `training-company-network` · Loại: `faq` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Bản tổng quan trong repo Training mô tả chuỗi sourcing–POD–kho–giao hàng và công bố số kho khác phần đọc được ở trang chủ.

**Cách tư vấn / xử lý đề xuất**

- Dùng để tìm đúng bộ phận/dịch vụ cần trao đổi; chờ xác nhận danh sách địa điểm đang nhận hàng. Không chọn một con số để quảng bá khi hai nguồn chưa thống nhất.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Danh sách kho hiện hành và phạm vi dịch vụ của từng kho.

**Điều cần tránh**

- Gộp kho đối tác, điểm tập kết và kho đang hoạt động thành một số chắc chắn.

Nguồn: Training: công ty, khách hàng, bốn mảng dịch vụ (training-company-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); [Giới thiệu THG Fulfill](https://thgfulfill.com/vi).

<a id="company-service-roles"></a>

## Vai trò phối hợp quanh một yêu cầu khách hàng

Mã: `company-service-roles` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Snapshot mô tả sale tư vấn/chốt, CSKH chăm sóc/theo dõi, Ops phối hợp xử lý, Finance xác nhận thanh toán/bồi hoàn và kho nhận–đóng gói–xuất hàng.

**Áp dụng khi**

- Cần xác nhận giá, khả năng cung ứng, sự cố hoặc bàn giao khách.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Dịch vụ/yêu cầu
- Dữ kiện đã có
- Phần cần quyết định
- Trạng thái cam kết

**Cách tư vấn / xử lý đề xuất**

- Chuyển yêu cầu theo vai trò cần xử lý, giữ bối cảnh đã trao đổi. Đây là bản phân vai trong training; xác nhận người/nhóm đang phụ trách trước khi hứa đã có người nhận xử lý.

**Đầu ra cần có**

- Đúng đầu mối vai trò và nội dung cần xác nhận.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Ai đang phụ trách tài khoản/dịch vụ và ai duyệt ngoại lệ?

**Điều cần tránh**

- Gán tên cá nhân, thời hạn phản hồi hoặc quyền phê duyệt từ một snapshot cũ.

**Mục liên quan**

- [Thông tin tư vấn phải giữ được khi bàn giao](commercial.md#training-sales-handover)
- [Lên đơn, thanh toán và tích hợp](commercial.md#commercial-onboarding)
- [Đền bù, hoàn tiền và gửi lại](commercial.md#commercial-compensation)

Nguồn: Training: vai trò phòng ban và bàn giao (training-cross-roles-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="company-confirmation-culture"></a>

## Rõ ràng và xác nhận khi thông tin thay đổi

Mã: `company-confirmation-culture` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Tài liệu văn hóa Ops nhấn mạnh phản hồi xác nhận rõ khi nhận thông tin mới để các bên hiểu thống nhất.

**Áp dụng khi**

- Khách bổ sung/đổi thông tin của yêu cầu hoặc đơn đã có.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Thông tin cũ/mới
- Đơn/biến thể liên quan
- Trạng thái xử lý

**Cách tư vấn / xử lý đề xuất**

- Khi khách đổi mẫu, địa chỉ, quy cách hoặc điều kiện, nhắc lại đúng phần đổi và kiểm tra ảnh hưởng trước khi nói đã xử lý. Lời xác nhận ngắn nên chỉ rõ đối tượng và trạng thái.

**Đầu ra cần có**

- Xác nhận cụ thể và bước kiểm tra khả năng thay đổi.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Thay đổi nào được chốt và phần xử lý cũ đã thực hiện tới đâu?

**Điều cần tránh**

- Nói đã đổi thành công khi mới chỉ nhận tin hoặc chỉ trả một lời đồng ý chung chung.

**Mục liên quan**

- [Thông tin tư vấn phải giữ được khi bàn giao](commercial.md#training-sales-handover)
- [Điều kiện xử lý đơn xuất kho](services.md#warehouse-outbound-procedure)
- [Chỉ hỏi điều chưa biết](replies.md#reply-known-vs-missing)

Nguồn: Training: xác nhận thông tin khi phối hợp (training-ops-culture-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).
