---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 10. Nguồn chuẩn, cập nhật và quyền sử dụng

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="governance-evidence-status"></a>

## Mỗi mục có nguồn và trạng thái riêng

Mã: `governance-evidence-status` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Đã đọc trên website chỉ chứng minh có công bố; task owner xác nhận chỉ áp dụng phạm vi quyết định đó. Code chứng minh hành vi triển khai, không chứng minh dịch vụ kinh doanh. Đề xuất và tình huống tổng hợp không tự trở thành chính sách công ty.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Người chịu trách nhiệm duyệt và ngày hiệu lực.

**Điều cần tránh**

- Tự gắn nhãn approved cho toàn bộ nội dung vừa thu thập.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

<a id="governance-dynamic-data"></a>

## Tách kiến thức ổn định và dữ liệu phải tra mới

Mã: `governance-dynamic-data` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Giữ mô tả dịch vụ và quy tắc tư vấn trong kho. Giá/tồn sản phẩm, cước, lịch vận chuyển, ưu đãi và điều khoản thay đổi phải tra nguồn hiện hành kèm thời điểm/phiên bản. Xác nhận đơn vị, tiền tệ và phạm vi trước khi dùng số.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Nguồn hiện hành nếu bản ghi đã cũ hoặc mất hiệu lực.

**Điều cần tránh**

- Dùng câu trả lời cũ làm nguồn giá mới.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="governance-approved-sop"></a>

## SOP nội bộ lấy từ nguồn được duyệt

Mã: `governance-approved-sop` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đối chiếu code; không xác nhận năng lực kinh doanh hay production. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Training cảnh báo HTML tĩnh có thể lệch tài liệu quản lý. Code export hiện đọc docs/doc_contents theo phòng ban và trang CMS đã publish; THG_tool có ingestor signed export.

**Cách tư vấn / xử lý đề xuất**

- Lấy SOP từ nguồn quản lý đúng scope, đối chiếu hiệu lực và duyệt trước khi dùng. Nhánh docs cũ trong database không được export kèm trạng thái publish/approval, nên không tự coi mọi bản là chính sách được duyệt. Không đưa quiz/ghi chú nhân sự vào tri thức tư vấn.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Bản SOP sales/service hiện hành và đầu mối phê duyệt.

**Điều cần tránh**

- Lấy SOP HTML cũ hoặc nhãn internal làm bằng chứng chính sách đã được duyệt.

Nguồn: THG Training: nguồn tài liệu quản lý và signed export (repo-training, THG_Training@ec170a949623); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

<a id="governance-workspace-audience"></a>

## Đúng workspace và đúng người nhận

Mã: `governance-workspace-audience` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- workspace scope
- internal customer visibility

**Cách tư vấn / xử lý đề xuất**

- Bộ này dành cho THGFulfill; khi nạp phải gắn org đúng và duyệt theo mục. Nhãn audience trong file là metadata cho biên tập; chưa phải một cơ chế phân quyền runtime. Nội dung công ty khác và thông tin khách riêng phải có ranh giới rõ.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Org đích, quyền truy xuất và cách lọc nội dung trước khi tạo lời nhắn.

**Điều cần tránh**

- Tự đưa toàn bộ tài liệu nội bộ vào prompt gửi khách hoặc chia sẻ giữa các org.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).
