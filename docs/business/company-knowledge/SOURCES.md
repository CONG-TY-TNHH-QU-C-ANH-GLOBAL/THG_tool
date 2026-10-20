---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# Nguồn, thời điểm đối chiếu và giới hạn

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

## web-home — Giới thiệu THG Fulfill

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi)

Hồ sơ công bố, nhóm dịch vụ, nhóm khách hàng. Không xác nhận độc lập năng lực vận hành hay các cam kết quảng bá.

## web-fulfill — Dịch vụ THG Fulfill

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/thg-fulfill)

Phạm vi POD và mô tả quy trình. Có nhiều cách lên đơn được mô tả; chưa xác nhận quy trình áp dụng cho từng khách.

## web-express — Dịch vụ THG Express

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/thg-express)

Dịch vụ vận chuyển, xử lý tại kho và tracking. Thời gian phụ thuộc tuyến, không phải một SLA chung.

## web-warehouse — Dịch vụ kho Mỹ

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/thg-warehouse)

Kho PA/NC, nhập kho, xử lý đơn. Trang dùng tên TH Group; tên thương hiệu chuẩn cần chốt với owner.

## web-order — Đặt hàng Taobao/1688 giao Mỹ

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/thg-order)

Luồng gửi link, kiểm tra, báo giá và giao hàng; không coi nội dung quảng bá là hợp đồng áp dụng mọi đơn.

## web-international — Bảng giá quốc tế

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/international-pricing)

Các chiều tra cứu quốc gia gửi/nhận, dịch vụ, loại hàng, khối lượng. Không chụp giá cố định vào pack.

## web-domestic — Biểu phí 3PL Mỹ

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/domestic-pricing)

Phí tại kho, bao bì, giao nội địa, điều kiện ưu đãi. Không dùng giá khởi điểm làm tổng phí.

## web-formal — Bảng giá chính ngạch Việt Nam–Mỹ

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/chinh-ngach-pricing)

Phạm vi tuyến và thành phần chưa bao gồm. Giá/lịch phải kiểm tra lại theo ngày và booking.

## web-policy — Mục chính sách

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/policy)

Xác nhận có các nhóm chính sách. Bản đọc web hiển thị ảnh nhiều trang; chưa đọc và duyệt trọn bộ điều khoản.

## web-shipping-policy — Chính sách vận chuyển theo tuyến

Loại: `official_public_page` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/shipping-policy)

Nhận diện cấu trúc chính sách theo tuyến. Có nội dung với mốc thời gian cũ; không chuyển thành tư vấn pháp lý/thuế hiện hành.

## web-catalog — Catalog công ty

Loại: `official_public_pointer` · Ngày đối chiếu: 2026-10-18.

[Mở nguồn](https://thgfulfill.com/vi/catalog)

Trang động; công cụ đọc web không trả danh sách sản phẩm. Không xác nhận SKU, giá hay tồn kho từ trang rỗng.

## owner-task — Quyết định của người dùng chuyển tiếp từ leader

Loại: `task_owner_decision` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: task_owner_decision

POD/Dropship, ba bảng giá, giá nguồn trong nhóm Telegram, nguồn Trung Quốc thay thế và POD thiếu catalog. Được xác nhận trong cuộc hội thoại; không phải SOP chung đã xuất bản.

## user-quality — Yêu cầu về chất lượng gợi ý

Loại: `user_requirement` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: user_requirement

Ngắn, trọng tâm, đúng dữ kiện, hiểu chung chữ và ảnh. Các hướng dẫn triển khai chi tiết vẫn là đề xuất của bộ tri thức.

## repo-tool — THG_tool Knowledge OS và luồng gợi ý

Loại: `repository_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_tool @ b2a3cf6dcf21c88277875960b746a8258db98adf

Các loại asset, trạng thái duyệt và hành vi code. Không chứng minh cấu hình/phiên bản production hiện tại.

- `internal/workspace_knowledge/assets/types.go`
- `internal/workspace_knowledge/runtime/lead_candidates.go`
- `internal/services/facebook/lead_suggestion_build.go`
- `docs/engineering/SUPPLIER_SOURCING.md`

## repo-training — THG Training: nguồn tài liệu quản lý và signed export

Loại: `repository_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623

Không dùng HTML tĩnh cũ làm SOP chuẩn. Export đọc docs/doc_contents của hệ thống cũ và trang CMS published, theo scope sale/marketing. Nhánh docs không mang trạng thái duyệt trong export. Chưa đọc nội dung live.

- `ASSISTANT.md`
- `src/index.js`

## repo-catalog — Contract catalog công khai trong landing page

Loại: `repository_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_landingpage @ 4fd55f1

Các trường sản phẩm/biến thể và endpoint được khai báo. Không phải ảnh chụp trạng thái catalog live.

- `src/lib/catalogApi.ts`

## training-company-snapshot — Training: công ty, khách hàng, bốn mảng dịch vụ

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc phần tổng quan/khách hàng. Có số kho và nguyên tắc nhận hàng cần đối chiếu; không tự coi snapshot là phạm vi dịch vụ hiện hành.

- `1.1-cong-ty-khach-hang-4-mang-dich-vu.html`

## training-sales-snapshot — Training: SOP sale và bàn giao

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc xác định nhu cầu, hồ sơ và vai trò thương mại. Không sao chép thông tin khách hay record meeting.

- `sop-sale-clean.html`

## training-sourcing-snapshot — Training: tìm nguồn và đánh giá sản phẩm

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc yêu cầu đầu vào, chỉ số sản phẩm, phân loại NCC. Không coi badge nền tảng cũ là chứng nhận hiện hành; không đưa danh sách NCC riêng vào pack.

- `dropship.html`

## training-pod-snapshot — Training: catalog POD

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc nhóm sản phẩm và đầu mối catalog/template. Không nhập danh mục cũ như SKU có hàng; bỏ qua quiz và trouble cụ thể.

- `pod.html`

## training-express-snapshot — Training: quy trình gửi hàng Express

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc yêu cầu kiện và luồng giao nhận. Tuyến/hạn xử lý phải đối chiếu bản hiện hành; không lấy số cũ làm cước/SLA.

- `express.html`

## training-warehouse-snapshot — Training: fulfillment và phương thức label

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc Ship By TikTok / Ship By Seller. Chỉ mô tả snapshot; chưa xác nhận chính sách nền tảng/khách hiện tại.

- `fulfill.html`

## training-cskh-snapshot — Training: tiếp nhận yêu cầu và trouble

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đã đọc thu bằng chứng, phân loại lỗi và đầu vào request. Không sao chép hạn xử lý, mức bồi thường, dữ liệu đơn hay quiz làm chính sách mới.

- `sop-cskh.html`

## training-pod-catalog-detail — Training POD phần 2: catalog và template

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc hai phần trên; không xác nhận sản phẩm/tồn kho/giá live hoặc lịch mùa bán.

- `pod-moi-phan02.html`

**Các phần đã đọc chọn lọc**

- 1 Danh mục sản phẩm POD
- 2 Template — lấy ở đâu gửi khách?

## training-pod-workflow-detail — Training POD phần 3: chuỗi xử lý đơn

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc chuỗi POD và vai trò; không lấy thông tin thanh toán, cashback hoặc địa chỉ cũ làm hiện hành.

- `pod-moi-phan03.html`

**Các phần đã đọc chọn lọc**

- 1 Quy trình POD đơn giản — 7 bước

## training-dropship-pricing-detail — Training Dropship phần 3: cấu phần báo giá

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc cấu phần và phần nguyên tắc báo giá trước bài tập. Không nạp hệ số/tỷ giá mẫu, số bài tập hoặc đáp án làm giá áp dụng.

- `dropship-moi-phan03.html`

**Các phần đã đọc chọn lọc**

- 3.1 Cách tính basecost 1 đơn Dropship
- 3.3 Nguyên tắc báo giá chuyên nghiệp (bổ sung)

## training-dropship-workflow-detail — Training Dropship phần 4: quy trình năm bước

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc năm bước; chưa xác nhận kênh nhập đơn và thanh toán áp dụng cho từng khách hiện tại.

- `dropship-moi-phan04.html`

**Các phần đã đọc chọn lọc**

- Bước 1: Tiếp nhận nhu cầu khách hàng
- Bước 2: Tìm kiếm & Đề xuất nguồn hàng
- Bước 3: Tính giá & Báo giá
- Bước 4: Chốt đơn & Xác nhận chính sách
- Bước 5: Lên đơn & Theo dõi vận hành

## training-warehouse-fees-detail — Training Warehouse phần 2: nhóm phí và xử lý chênh lệch

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc cách tách phí và chuyển Ops khi lệch; không dùng mức phí/ưu đãi trong snapshot làm báo giá mới.

- `warehouse-full-phan02.html`

**Các phần đã đọc chọn lọc**

- 1 3 loại phí cần nhớ
- 5 Khi thấy giá "lệch" — phải làm gì?

## training-warehouse-flow-detail — Training Warehouse phần 3: nhập kho, đơn ra, hàng hoàn

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc các phần trên trước bài thực hành; không xác nhận cutoff/SLA hoặc mapping status live.

- `warehouse-full-phan03.html`

**Các phần đã đọc chọn lọc**

- 4 Bước 2 — Quy trình & Điều kiện nhập kho
- 5 Bước 3 — Nhận đơn & Chuyển bộ phận liên quan
- Trạng thái đơn hàng cần nắm
- 7 Bước 5 — Xử lý Return / Refund

## training-warehouse-packaging-detail — Training Warehouse phần 4: xác nhận bao bì

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc nguyên tắc xác nhận bao bì; không nạp case trouble hoặc dữ liệu đơn cụ thể.

- `warehouse-full-phan04.html`

**Các phần đã đọc chọn lọc**

- 📦 Trao đổi rõ về bao bì với khách

## training-ops-services-detail — Training: dịch vụ theo góc nhìn vận hành

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc các phần trên. Chưa xác nhận phạm vi sàn mua hộ, đối tác xưởng hoặc quy định nền tảng trong snapshot còn hiệu lực.

- `van-hanh-dich-vu-clean.html`

**Các phần đã đọc chọn lọc**

- Tìm & Đặt Nguồn Hàng
- Kiểm Tra – Đóng Gói – Dán Nhãn
- Chọn Line Vận Chuyển Phù Hợp
- Dịch Vụ POD – Print on Demand
- 🏬 3.3.1 – Hàng Sẵn Tại Kho US
- 📋 3.3.2 – Fulfillment Tại Kho US

## training-ops-culture-detail — Training: xác nhận thông tin khi phối hợp

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc nguyên tắc xác nhận thông tin; không sao chép thông tin nhân sự hoặc kênh riêng.

- `van-hanh-quy-tac-clean.html`

**Các phần đã đọc chọn lọc**

- 🔑 Nguyên Tắc Bắt Buộc: Tính Rõ Ràng & Xác Nhận Thông Tin

## training-cross-roles-detail — Training: vai trò phòng ban và bàn giao

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc phân công theo vai trò và bàn giao. Không dùng tên nhân sự, lịch hoặc hạn tiếp nhận cũ làm phân công hiện tại.

- `sop-cross-clean.html`

**Các phần đã đọc chọn lọc**

- Sale & Marketing
- CSKH
- Vận Hành (Ops)
- Finance / Kế Toán
- Kho (TQ & US)
- Onboard khách hàng mới từ Sale sang CSKH/Ops

## training-sale-express-detail — Training sale Express: tuyến, giá và FAQ

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc ba phần này. Thấy cách mô tả basecost và tracking chưa đồng nhất với tài liệu khác. Không nạp giá, thời hạn, địa chỉ return hoặc điều khoản đền bù cũ làm cam kết hiện hành.

- `sale-express-clean.html`

**Các phần đã đọc chọn lọc**

- Các Line Vận Chuyển THG Cung Cấp
- Lưu Ý Quan Trọng Về Giá Vận Chuyển
- Các Thắc Mắc Thường Gặp Của Khách Hàng

## training-pod-onboarding-detail — Training POD phần 4: tiếp nhận khách

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc bước nhận khách, gửi tài liệu và tạo tài khoản. Không sao chép thông tin nhóm/kênh riêng, tên ví dụ hoặc ưu đãi thành hướng dẫn hiện hành.

- `pod-moi-phan04.html`

**Các phần đã đọc chọn lọc**

- B Tiếp nhận khách hàng mới

## training-dropship-search-detail — Training Dropship phần 2: tra nguồn và đánh giá

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc mô tả nền tảng, tìm bằng ảnh và checklist trước phần thực hành. Không coi thứ hạng giá, ngưỡng rating hoặc nhãn có sẵn là bảo đảm. Hướng dẫn thao tác thủ công không chứng minh backend có tìm ảnh.

- `dropship-moi-phan02.html`

**Các phần đã đọc chọn lọc**

- So sánh nhanh các sàn (kiến thức bổ sung)
- 2.2 Cách tìm kiếm nguồn hàng Trung Quốc (Sàn 1688/Taobao)
- Tìm bằng hình ảnh
- 2.3 Checklist đánh giá supplier uy tín

## training-dropship-exceptions-detail — Training Dropship phần 5: tình huống thường gặp

Loại: `legacy_training_snapshot` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: THG_Training @ ec170a949623f96229db58d21c6b9ca7791826d4

Đọc bốn tình huống trước bài kiểm tra. Chỉ dùng logic xác minh, so sánh cấu phần và cập nhật chậm hàng; không dùng phí mua hộ, cashback, deadline hay thời gian tracking cũ làm chính sách hiện hành.

- `dropship-moi-phan05.html`

**Các phần đã đọc chọn lọc**

- 5.3 Xử lý 4 tình huống Fresher chắc chắn sẽ gặp

## user-interim-judgment — Người dùng cho phép dùng phán đoán để tiếp tục biên soạn

Loại: `user_requirement` · Ngày đối chiếu: 2026-10-18.

Repo/trao đổi: user_requirement

Người dùng trả lời “Bạn nghĩ theo hướng bạn cho là đúng được nhé” cho câu hỏi về basecost và active tracking. Đây là ủy quyền chọn cách xử lý tạm thời; không cung cấp giá, cấu phần gói hay SLA hiện hành, không tự duyệt asset production.
