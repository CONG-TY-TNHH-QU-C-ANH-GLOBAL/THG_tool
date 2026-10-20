---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# Thông tin cần bổ sung hoặc xác nhận

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

Có thể tiếp tục biên soạn và kiểm thử với dữ liệu đã có. Các mục dưới đây chỉ chặn phát ngôn/cam kết tương ứng, không chặn toàn bộ công việc. Owner là vai trò đề xuất, chưa phân công một cá nhân.

## brand-name

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Tên dùng trong câu trả lời là THG Fulfill hay TH Group? Có quy tắc theo thương hiệu/dịch vụ không?

**Đầu mối phù hợp:** Leader/Marketing.

**Vì sao:** Các trang hiện hiển thị hai cách gọi.

**Trong lúc chưa có:** Dùng tên trong profile workspace đã được duyệt; không tự đổi tên trên hệ thống.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi); [Dịch vụ kho Mỹ](https://thgfulfill.com/vi/thg-warehouse).

## company-profile

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Có company profile mới nhất về pháp nhân, lịch sử, địa điểm và chứng nhận được phép công bố không?

**Đầu mối phù hợp:** Leader/Administration.

**Vì sao:** Đã đọc tổng quan Training trong repo; số kho và cách mô tả mạng lưới chưa thống nhất với phần website đọc được.

**Trong lúc chưa có:** Không chọn một số kho để quảng bá hoặc suy quyền sở hữu/công suất khi chưa thống nhất nguồn.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi); Training: công ty, khách hàng, bốn mảng dịch vụ (training-company-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

## service-lanes

**Ưu tiên:** Xác nhận trước khi dùng cho cam kết vận hành tương ứng.

**Cần xác nhận:** Danh sách dịch vụ/tuyến đang nhận, giới hạn và điều kiện theo loại hàng nào là bản được vận hành duyệt?

**Đầu mối phù hợp:** Operations.

**Vì sao:** Có trang quảng bá nhiều thị trường nhưng chưa xác nhận mọi SKU/đích giao đều được hỗ trợ.

**Trong lúc chưa có:** Thu yêu cầu, kiểm tra tuyến hiện hành trước khi hứa nhận hàng.

Nguồn: [Dịch vụ THG Express](https://thgfulfill.com/vi/thg-express); [Dịch vụ kho Mỹ](https://thgfulfill.com/vi/thg-warehouse).

## policy-full-text

**Ưu tiên:** Xác nhận trước khi dùng cho cam kết vận hành tương ứng.

**Cần xác nhận:** Bản điều khoản mới nhất có text hoặc PDF gốc và ngày hiệu lực là bản nào?

**Đầu mối phù hợp:** Operations/Customer Service.

**Vì sao:** Chính sách nhiều trang dạng ảnh chưa được đọc/đối chiếu đầy đủ; trang vận chuyển có mốc cũ.

**Trong lúc chưa có:** Không báo hạn khiếu nại, hạn bồi thường, điều kiện hàng nhạy cảm hay nghĩa vụ thuế từ suy đoán.

Nguồn: [Mục chính sách](https://thgfulfill.com/vi/policy); [Chính sách vận chuyển theo tuyến](https://thgfulfill.com/vi/shipping-policy).

## compensation-scope

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Bồi thường POD, Dropship, hàng lô và từng line áp dụng mức/điều kiện nào?

**Đầu mối phù hợp:** Operations/Customer Service.

**Vì sao:** Thông điệp giới thiệu chung và FAQ dịch vụ không đủ để xác nhận cùng một quyền lợi cho mọi đơn.

**Trong lúc chưa có:** Nhận mã đơn/tracking, thông tin sự cố và chuyển kiểm tra điều khoản.

Nguồn: [Giới thiệu THG Fulfill](https://thgfulfill.com/vi); [Dịch vụ THG Fulfill](https://thgfulfill.com/vi/thg-fulfill); [Mục chính sách](https://thgfulfill.com/vi/policy).

## order-and-payment

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Quy trình lên đơn, thanh toán và đối soát chuẩn theo từng dịch vụ là gì?

**Đầu mối phù hợp:** Sales Operations/Finance.

**Vì sao:** Website mô tả Hub, ERP và file chung; chưa xác định cách dùng cho từng loại khách.

**Trong lúc chưa có:** Hướng dẫn nhận tư vấn và dùng kênh được cấp; không gửi tài khoản thanh toán chưa xác thực.

Nguồn: [Dịch vụ THG Fulfill](https://thgfulfill.com/vi/thg-fulfill); [Dịch vụ THG Express](https://thgfulfill.com/vi/thg-express); THG Training: nguồn tài liệu quản lý và signed export (repo-training, THG_Training@ec170a949623).

## catalog-current

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Ai duyệt catalog/biến thể, khả năng in, giá và thời gian sản xuất? Dữ liệu nào hiện được đồng bộ?

**Đầu mối phù hợp:** Product/POD Operations.

**Vì sao:** Đã thấy contract nhưng chưa xác nhận snapshot sản phẩm live trong lần xây pack.

**Trong lúc chưa có:** Tra SKU thật qua nguồn catalog hiện hành; không tạo sản phẩm/giá mẫu để AI dùng như dữ kiện thật.

Nguồn: [Catalog công ty](https://thgfulfill.com/vi/catalog); Contract catalog công khai trong landing page (repo-catalog, THG_landingpage@4fd55f1).

## customer-price-policy

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Giá nguồn được phép ghi thế nào trong bản sale gửi khách?

**Đầu mối phù hợp:** Leader/Sales.

**Vì sao:** Đã chốt hiển thị nội bộ THG_Sale_Lead; chưa có quy tắc công khai cho mọi khách và mọi lớp chi phí.

**Trong lúc chưa có:** Giữ nhãn nội bộ, phân biệt nguồn tham khảo và báo giá khách; không mở rộng quyền dữ liệu CRM.

Nguồn: Quyết định của người dùng chuyển tiếp từ leader (owner-task).

## pricing-authority

**Ưu tiên:** Xác nhận trước khi dùng cho cam kết vận hành tương ứng.

**Cần xác nhận:** Nguồn được duyệt cho giá số, phiên bản, phụ phí và hiệu lực của từng bảng hiện là đâu?

**Đầu mối phù hợp:** Operations/Pricing.

**Vì sao:** Trang web là điểm tham chiếu; cần nguồn máy đọc và đủ điều kiện để phát ra một con số.

**Bằng chứng và điểm chưa thống nhất**

- FAQ sale Express mô tả basecost như giá cuối đã gồm một số phí; phần tính giá Dropship tách nhiều cấu phần. Chưa có đủ điều kiện để kết luận đây là cùng gói dịch vụ hoặc một tài liệu sai.
- Phần sale Express yêu cầu kiểm giá mới và điều khoản của line trước mỗi lần tư vấn; không lấy đơn cũ làm căn cứ.

**Mục bị ảnh hưởng**

- [Các thành phần cần kiểm tra trong báo giá Dropship](topics/commercial.md#dropship-quote-components)
- [Giá nguồn, phí dịch vụ, cước và giá bán](topics/commercial.md#commercial-price-layers)
- [Kiểm tra bảng giá mới và trường hợp line chưa có giá](topics/shipping.md#shipping-rate-refresh)
- [Khách thấy giá cao: so cùng phạm vi trước khi đề xuất](topics/commercial.md#commercial-quote-comparison)

**Trong lúc chưa có:** Không chép giá quảng bá hay giá cũ thành báo giá; chỉ dùng bảng đúng chặng và xác nhận số hiện hành.

Nguồn: [Bảng giá quốc tế](https://thgfulfill.com/vi/international-pricing); [Biểu phí 3PL Mỹ](https://thgfulfill.com/vi/domestic-pricing); [Bảng giá chính ngạch Việt Nam–Mỹ](https://thgfulfill.com/vi/chinh-ngach-pricing); Training sale Express: tuyến, giá và FAQ (training-sale-express-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 3: cấu phần báo giá (training-dropship-pricing-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

## training-live-export

**Ưu tiên:** Xác nhận trước khi dùng cho cam kết vận hành tương ứng.

**Cần xác nhận:** Những SOP sales/service nào hiện hành, được duyệt và nằm trong scope xuất sang THG_tool?

**Đầu mối phù hợp:** Training/Sales Operations.

**Vì sao:** Export gồm docs quản lý cũ và CMS published; nhánh docs không có trạng thái duyệt trong payload. Người dùng xác nhận có quyền truy cập Training; đang đối chiếu các tài liệu trong repo theo chỉ dẫn của người dùng.

**Trong lúc chưa có:** Giữ pack chờ duyệt; kiểm tra hiệu lực của từng SOP, không coi internal là approved.

Nguồn: THG Training: nguồn tài liệu quản lý và signed export (repo-training, THG_Training@ec170a949623); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

## approval-maintenance

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Ai duyệt nội dung công ty, ai cập nhật dịch vụ/cước và ai duyệt câu trả lời mẫu?

**Đầu mối phù hợp:** Leader.

**Vì sao:** Cần chủ sở hữu theo nhóm để sửa thông tin khi thay đổi.

**Trong lúc chưa có:** Không gán tên cá nhân hoặc ghi approved thay người có thẩm quyền.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

## runtime-scope

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Org đích và cơ chế lọc tri thức nội bộ trước khi tạo bản nháp khách là gì?

**Đầu mối phù hợp:** Workspace administrator/Engineering.

**Vì sao:** Nhãn audience trong pack chưa được runtime thực thi; nhập tài liệu chưa đủ để mọi nhánh reply sử dụng.

**Trong lúc chưa có:** Chỉ kiểm thử local/review; chưa nhập/duyệt/bật pack trên production.

Nguồn: THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

## customer-service-scope

**Ưu tiên:** Bổ sung để hoàn thiện nội dung.

**Cần xác nhận:** Phạm vi nhận khách cá nhân, khách order hộ và seller theo từng dịch vụ hiện là gì? Nguyên tắc nhận hàng trong tổng quan Training có điều kiện nào?

**Đầu mối phù hợp:** Leader/Sales Operations.

**Vì sao:** Training tổng quan hướng tới seller, trong khi trang order nói tới người Việt tại Mỹ mua hàng. Câu khái quát hàng bán trên sàn trong snapshot không đủ thay điều kiện tuyến.

**Trong lúc chưa có:** Ghi nhận đúng nhu cầu và kiểm tra dịch vụ/line; không tự loại mọi cá nhân hoặc hứa mọi hàng bán trên sàn đều ship được.

Nguồn: Training: công ty, khách hàng, bốn mảng dịch vụ (training-company-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); [Đặt hàng Taobao/1688 giao Mỹ](https://thgfulfill.com/vi/thg-order); [Chính sách vận chuyển theo tuyến](https://thgfulfill.com/vi/shipping-policy).

## basecost-inclusions

**Cách tiếp tục:** Đã có hướng dẫn tạm thời theo ủy quyền của người dùng; việc biên soạn và kiểm thử tiếp tục. Điều khoản kinh doanh vẫn chưa được xác nhận; trạng thái này không duyệt asset hay bật production.

**Hướng dẫn tạm thời**

- [Cách xử lý tạm thời khi basecost chưa rõ phạm vi](topics/commercial.md#commercial-basecost-interpretation)

**Ưu tiên:** Xác nhận trước khi dùng cho cam kết vận hành tương ứng.

**Cần xác nhận:** Theo từng dịch vụ, basecost là giá nguồn, tổng chi phí nội bộ hay giá cuối cho khách? Khoản nào đã gồm và khoản nào tính thêm?

**Đầu mối phù hợp:** Operations/Pricing.

**Vì sao:** Cách gọi trong các snapshot khác nhau; dùng một công thức chung có thể thiếu phí hoặc tính trùng.

**Bằng chứng và điểm chưa thống nhất**

- FAQ Express rút gọn tiền hàng và vận chuyển thành basecost cuối; bài tính Dropship còn phân tách nội địa nguồn, mua hộ/quy đổi và quốc tế. Có thể khác gói hoặc khác thời điểm, cần owner xác nhận.
- Người dùng cho phép tiếp tục bằng phán đoán. Đã chọn hướng dẫn tạm thời có điều kiện; chưa xác nhận điều khoản kinh doanh hiện hành.

**Mục bị ảnh hưởng**

- [Các thành phần cần kiểm tra trong báo giá Dropship](topics/commercial.md#dropship-quote-components)
- [Giá nguồn, phí dịch vụ, cước và giá bán](topics/commercial.md#commercial-price-layers)
- [Khách thấy giá cao: so cùng phạm vi trước khi đề xuất](topics/commercial.md#commercial-quote-comparison)

**Trong lúc chưa có:** Áp dụng hướng dẫn tạm thời theo từng báo giá: phân biệt đã gồm/tính riêng/không áp dụng/chưa rõ; không cộng trùng hoặc báo tổng cuối khi thiếu khoản. Việc biên soạn và kiểm thử tiếp tục, không cần chờ định nghĩa chung.

Nguồn: Training sale Express: tuyến, giá và FAQ (training-sale-express-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 3: cấu phần báo giá (training-dropship-pricing-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Người dùng cho phép dùng phán đoán để tiếp tục biên soạn (user-interim-judgment).

## tracking-activation-scope

**Cách tiếp tục:** Đã có hướng dẫn tạm thời theo ủy quyền của người dùng; việc biên soạn và kiểm thử tiếp tục. Điều khoản kinh doanh vẫn chưa được xác nhận; trạng thái này không duyệt asset hay bật production.

**Hướng dẫn tạm thời**

- [Cách xử lý tạm thời khi chưa có SLA active tracking](topics/shipping.md#shipping-tracking-milestones)

**Ưu tiên:** Xác nhận trước khi dùng cho cam kết vận hành tương ứng.

**Cần xác nhận:** Theo line/nền tảng hiện hành, tạo tracking, hãng nhận kiện và tracking đủ điều kiện sàn là các mốc nào? Có SLA được duyệt cho từng mốc không?

**Đầu mối phù hợp:** Operations/Customer Service.

**Vì sao:** Snapshot FAQ nêu 48 giờ còn tình huống Dropship nhắc 24 giờ cho TikTok; chưa đủ dữ kiện về điểm bắt đầu, loại đơn và thời điểm áp dụng.

**Bằng chứng và điểm chưa thống nhất**

- Các mốc trong tài liệu lịch sử không được dùng làm lời hứa hiện hành. Khác thời gian chưa tự chứng minh tài liệu nào đúng nếu chưa rõ cùng phạm vi.
- Người dùng cho phép tiếp tục bằng phán đoán. Đã chọn hướng dẫn tạm thời có điều kiện; chưa xác nhận điều khoản kinh doanh hiện hành.

**Mục bị ảnh hưởng**

- [Trạng thái hệ thống, label và sự kiện giao nhận](topics/commercial.md#warehouse-status-interpretation)
- [Phân biệt Ship By Seller và Ship By TikTok](topics/shipping.md#training-warehouse-label-mode)
- [Chính sách và thời gian phải theo đúng tuyến](topics/shipping.md#shipping-policy-scope)

**Trong lúc chưa có:** Áp dụng hướng dẫn tạm thời theo sự kiện thực tế: tách tạo mã, bàn giao/scan và trạng thái nền tảng; không tự đặt SLA. Việc biên soạn và kiểm thử tiếp tục, không cần chờ mốc 24/48 giờ.

Nguồn: Training sale Express: tuyến, giá và FAQ (training-sale-express-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 5: tình huống thường gặp (training-dropship-exceptions-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Người dùng cho phép dùng phán đoán để tiếp tục biên soạn (user-interim-judgment).
