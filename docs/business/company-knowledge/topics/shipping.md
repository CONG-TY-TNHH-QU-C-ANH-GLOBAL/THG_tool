---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 06. Vận chuyển và giá tham chiếu

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="shipping-three-tables"></a>

## Ba bảng giá có mục đích riêng

Mã: `shipping-three-tables` · Loại: `shipping_policy` · Nội bộ sale

Trạng thái: Người dùng/leader đã chốt trong phạm vi task. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Leader xác nhận dùng cả ba bảng giá và chọn theo nhu cầu, số lượng, nơi gửi/nhận và điều kiện vận chuyển.

**Cách tư vấn / xử lý đề xuất**

- Lưu link nguồn riêng cho từng bảng; chọn theo hành trình hàng thực tế, không chọn bảng rẻ nhất hoặc ghép các chặng không có căn cứ.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Hàng ở đâu, đi đâu, đi từng kiện hay một lô?

**Điều cần tránh**

- Đánh đồng toàn bộ bảng giá thành một mức cước chung.

Nguồn: Quyết định của người dùng chuyển tiếp từ leader (owner-task); [Bảng giá quốc tế](https://thgfulfill.com/vi/international-pricing); [Biểu phí 3PL Mỹ](https://thgfulfill.com/vi/domestic-pricing); [Bảng giá chính ngạch Việt Nam–Mỹ](https://thgfulfill.com/vi/chinh-ngach-pricing).

<a id="shipping-international"></a>

## Tra cước quốc tế

Mã: `shipping-international` · Loại: `shipping_policy` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- international rates
- shipping origin destination

**Dữ kiện có nguồn**

- Trang quốc tế cho tra theo nước gửi, nước nhận, dịch vụ, loại hàng và cân nặng; có phân biệt xuất phát VN/CN.

**Cách tư vấn / xử lý đề xuất**

- Xác nhận line nhận loại hàng, cân nặng đóng kiện và quy tắc thể tích. Chỉ trích cước còn hiệu lực cho đúng tổ hợp dữ kiện; giữ các khoản chưa bao gồm.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Loại hàng, nơi gửi/nhận, cân nặng/kích thước còn thiếu.

**Điều cần tránh**

- Chuyển giá VN–US sang CN–US hoặc áp dụng một loại hàng cho loại khác.

Nguồn: [Bảng giá quốc tế](https://thgfulfill.com/vi/international-pricing).

<a id="shipping-domestic"></a>

## Tra phí 3PL và giao nội địa Mỹ

Mã: `shipping-domestic` · Loại: `shipping_policy` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- US domestic shipping
- 3PL fees
- postage

**Dữ kiện có nguồn**

- Biểu phí nội địa tách khoản xử lý tại kho, lưu kho và giao nội địa. Có điều kiện áp dụng ưu đãi và mức phí theo hàng/kiện.

**Cách tư vấn / xử lý đề xuất**

- Cần kế hoạch hàng trong kho Mỹ, đặc tính kiện và vùng giao để kiểm tra. Tách phí thao tác kho, bao bì và bưu phí; không gộp giá khởi điểm thành giá trọn gói.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Kho xuất, ZIP/zone, quy cách kiện và nhu cầu xử lý.

**Điều cần tránh**

- Khẳng định ưu đãi lưu kho áp dụng mọi khách, mọi thể tích và mọi thời điểm.

Nguồn: [Biểu phí 3PL Mỹ](https://thgfulfill.com/vi/domestic-pricing).

<a id="shipping-formal"></a>

## Chính ngạch VN–Mỹ

Mã: `shipping-formal` · Loại: `shipping_policy` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- Vietnam US formal shipping
- formal shipment pricing

**Dữ kiện có nguồn**

- Trang chính ngạch mô tả VN–Mỹ theo Sea/Air, có thành phần chưa bao gồm như trucking, thủ tục và giao cuối chặng.

**Cách tư vấn / xử lý đề xuất**

- Xác nhận nơi xuất, đích giao, phương thức và quy mô lô trước khi chọn bảng. Kiểm tra đơn vị tính, lịch booking và khoản loại trừ hiện hành.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Điểm lấy/giao, trọng lượng/thể tích, loại hàng và phạm vi báo giá.

**Điều cần tránh**

- Dùng bảng này làm cước trực tiếp từ nhà cung cấp Trung Quốc khi chưa có chặng VN phù hợp.

Nguồn: [Bảng giá chính ngạch Việt Nam–Mỹ](https://thgfulfill.com/vi/chinh-ngach-pricing).

<a id="shipping-geography"></a>

## Phân biệt nơi nhà cung cấp, nơi xuất hàng và nơi nhận

Mã: `shipping-geography` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Giữ riêng nhà cung cấp đặt tại đâu, hàng hiện ở đâu, kho định dùng và điểm nhận cuối. Supplier based in UAE, delivery to Canada có nơi nhận là Canada. Người đăng ở Mỹ chưa đủ xác nhận giao Mỹ.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Điểm nhận cuối hoặc nơi lấy hàng thật sự nếu chưa nói.

**Điều cần tránh**

- Lấy bất kỳ tên quốc gia đầu tiên làm destination.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="shipping-period-and-parcel"></a>

## Sản lượng tháng, một lô và một kiện

Mã: `shipping-period-and-parcel` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Lưu số lượng cùng đơn vị và kỳ: 300 hộp/tháng là sản lượng, có thể mua/gửi nhiều lần. Muốn tính cước cần quy cách từng kiện hoặc tổng lô. Nếu có cước một kiện tham khảo, ghi rõ cơ sở; không nhân thành báo giá lô khi chưa xác nhận.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mỗi lần gửi bao nhiêu, bao nhiêu kiện, mỗi kiện chứa bao nhiêu sản phẩm?

**Điều cần tránh**

- Lấy trọng lượng một sản phẩm làm trọng lượng của toàn bộ lô.
- Không lấy số cân nặng hoặc sản lượng tháng làm số ngày giao hàng chỉ vì cùng một con số.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="shipping-sensitive-goods"></a>

## Loại hàng cần kiểm tra riêng

Mã: `shipping-sensitive-goods` · Loại: `shipping_policy` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- restricted goods
- batteries liquids supplements

**Cách tư vấn / xử lý đề xuất**

- Với thực phẩm bổ sung, mỹ phẩm, chất lỏng, pin/điện và hàng có nhãn hiệu, kiểm tra thông số và chính sách tuyến. Danh mục tổng quát chỉ là dấu hiệu cần kiểm tra; chưa xác nhận một SKU cụ thể được nhận hoặc cần giấy tờ gì.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Thành phần/thông số, tình trạng pin/chất lỏng và chứng từ theo yêu cầu line thực tế.

**Điều cần tránh**

- Áp cước hàng thường hoặc hứa thông quan từ tên sản phẩm/ảnh bao bì.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); [Chính sách vận chuyển theo tuyến](https://thgfulfill.com/vi/shipping-policy).

<a id="shipping-policy-scope"></a>

## Chính sách và thời gian phải theo đúng tuyến

Mã: `shipping-policy-scope` · Loại: `banned_claim` · Nội bộ sale

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Trang chính sách vận chuyển chia theo xuất phát, điểm đến, loại hàng và nhóm dịch vụ; nội dung đọc được có cả mốc thời gian cũ.

**Cách tư vấn / xử lý đề xuất**

- Đối chiếu phiên bản/chính sách hiện hành trước khi đưa hạn giao, công thức thể tích hay điều kiện khai báo. Tách thời gian mua hàng, sản xuất, về kho và vận chuyển.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Phiên bản điều khoản áp dụng cho line đang chọn.

**Điều cần tránh**

- Dùng một thời gian giao hoặc một công thức tính cho tất cả các tuyến.

Nguồn: [Chính sách vận chuyển theo tuyến](https://thgfulfill.com/vi/shipping-policy).

<a id="training-express-inputs"></a>

## Đầu vào và các chặng giao nhận Express

Mã: `training-express-inputs` · Loại: `shipping_policy` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Quy trình snapshot đề cập cả gửi qua kho trung chuyển và gửi thẳng; nhấn mạnh trọng lượng kiện, địa chỉ nhận, nội dung hàng và bàn giao giữa sale/CSKH, finance, ops, kho.

**Cách tư vấn / xử lý đề xuất**

- Xác nhận hành trình thực tế và phạm vi báo giá. Giữ trạng thái đã có thông tin, đã xác nhận thanh toán, kho đã nhận và hãng đã nhận riêng nhau; không mang hạn xử lý cũ thành SLA hiện hành.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Điểm lấy, quy cách kiện và địa chỉ nhận đủ để tra tuyến.

**Điều cần tránh**

- Mặc định hàng VN luôn đi vòng CN hoặc luôn gửi thẳng.

Nguồn: Training: quy trình gửi hàng Express (training-express-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="training-warehouse-label-mode"></a>

## Phân biệt Ship By Seller và Ship By TikTok

Mã: `training-warehouse-label-mode` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Snapshot fulfillment mô tả hai quy trình khác nhau về bên cung cấp/mua label và theo dõi tracking.

**Cách tư vấn / xử lý đề xuất**

- Hỏi phương thức label khi ảnh hưởng xử lý đơn và phí. Đối chiếu quy định nền tảng và SOP hiện hành cho đúng khách, không suy TikTok Shop luôn dùng một phương thức duy nhất.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách tự có label hay cần THG cung cấp, theo phương thức nào?

**Điều cần tránh**

- Tính thêm bưu phí khi phương án khách đã có label mà chưa đối chiếu phạm vi.

Nguồn: Training: fulfillment và phương thức label (training-warehouse-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="warehouse-fee-components"></a>

## Tách phí kho, bao bì và vận chuyển nội địa

Mã: `warehouse-fee-components` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- warehouse handling fees
- packaging fee
- pick pack postage

**Dữ kiện có nguồn**

- Training phân biệt phí ship nội địa, phí xử lý/fulfill và vật tư đóng gói. Phạm vi thao tác cần đối chiếu bảng hiện hành.

**Áp dụng khi**

- Khách hỏi tổng chi phí xử lý/giao hàng từ kho Mỹ.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- SKU/số item trong đơn
- Bao bì
- Trọng lượng/kích thước kiện
- ZIP/zone
- Nhu cầu thao tác phụ

**Cách tư vấn / xử lý đề xuất**

- Kiểm tra dịch vụ đã bao gồm thao tác nào và loại bao bì nào. Tính lưu kho/inbound/return theo điều kiện riêng khi phát sinh, không tự gộp vào một mức phí đầu trang.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Xác định phần việc tại kho.
- Tra vật tư/bao bì phù hợp.
- Xác định khách có label hay cần mua.
- Tra cước và các phí phát sinh có điều kiện.

**Đầu ra cần có**

- Các khoản phí tách rõ, không tính trùng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Số sản phẩm/đơn, quy cách kiện, bao bì và phương thức label.

**Điều cần tránh**

- Nói phí pick-pack đã bao gồm mọi bưu phí và bao bì.

**Mục liên quan**

- [Tra phí 3PL và giao nội địa Mỹ](shipping.md#shipping-domestic)
- [Phân biệt Ship By Seller và Ship By TikTok](shipping.md#training-warehouse-label-mode)
- [Xác nhận bao bì trước khi xử lý](products.md#warehouse-packaging-agreement)

Nguồn: Training Warehouse phần 2: nhóm phí và xử lý chênh lệch (training-warehouse-fees-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); [Biểu phí 3PL Mỹ](https://thgfulfill.com/vi/domestic-pricing).

<a id="shipping-rate-refresh"></a>

## Kiểm tra bảng giá mới và trường hợp line chưa có giá

Mã: `shipping-rate-refresh` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Snapshot sale Express yêu cầu kiểm tra bảng giá mới khi lên đơn, không sao chép giá đơn cũ; line chưa có giá hoặc yêu cầu đặc biệt cần chuyển Ops kiểm tra.

**Áp dụng khi**

- Khách xin cước hoặc có yêu cầu vận chuyển đặc biệt.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Điểm xuất/nhận
- Loại hàng
- Quy cách kiện
- Nguồn giá và hiệu lực

**Cách tư vấn / xử lý đề xuất**

- Tra nguồn được duyệt và hiệu lực cho đúng tuyến/loại hàng/kiện. Thiếu giá cần tạo yêu cầu báo giá có dữ kiện, không dùng tuyến gần giống để lấp số. Tần suất cập nhật trong bản cũ không tự xác nhận bảng hiện tại còn hiệu lực.

**Đầu ra cần có**

- Giá có điều kiện hoặc yêu cầu Ops báo giá rõ phần thiếu.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Phiên bản/ngày hiệu lực và điều kiện của mức cước đang tham chiếu.

**Điều cần tránh**

- Sao chép cước đã trả tháng trước thành cước mới
- Nói không nhận hàng chỉ vì bảng chưa có giá

**Mục liên quan**

- [Tra cước quốc tế](shipping.md#shipping-international)
- [Chính sách và thời gian phải theo đúng tuyến](shipping.md#shipping-policy-scope)
- [Tách kiến thức ổn định và dữ liệu phải tra mới](governance.md#governance-dynamic-data)

Nguồn: Training sale Express: tuyến, giá và FAQ (training-sale-express-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); [Bảng giá quốc tế](https://thgfulfill.com/vi/international-pricing).

<a id="shipping-tracking-milestones"></a>

## Cách xử lý tạm thời khi chưa có SLA active tracking

Mã: `shipping-tracking-milestones` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- tracking activation
- carrier acceptance
- label created

**Áp dụng khi**

- Khách hỏi tracking chưa chạy, đã gửi chưa hoặc thời gian active tracking.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Nhu cầu: kiểm tiến độ hay hỏi SLA
- Nguồn và thời điểm sự kiện tracking hiện hành
- Line/nền tảng nếu cần đối chiếu điều kiện
- SLA và mốc tính đã được duyệt khi phát sinh cam kết

**Cách tư vấn / xử lý đề xuất**

- Tách các bằng chứng cần kiểm tra: tạo đơn/label và mã tracking; xác nhận bàn giao/nhận kiện của hãng; sự kiện di chuyển; trạng thái đáp ứng yêu cầu nền tảng nếu có. Không dùng một chữ active để thay tất cả các mốc này.
- Chỉ mô tả sự kiện đã xác minh cùng thời điểm và nguồn. Có mã không đủ chứng minh hãng đã nhận; chưa thấy scan cũng chưa đủ kết luận chưa bàn giao vì sự kiện có thể thiếu hoặc cập nhật chậm.
- Khi cần nêu hạn xử lý, phải có SLA còn hiệu lực cho đúng line/nền tảng, sự kiện bắt đầu, múi giờ và cách tính ngày làm việc/lịch nếu có. Không tự chọn mốc 24 hoặc 48 giờ từ Training làm mặc định, kể cả chọn mốc ngắn hơn.
- Khách hỏi đơn cụ thể thì kiểm tra mã và sự kiện đơn; khách hỏi dịch vụ thì giải thích cách theo dõi và phần cần xác nhận theo line. Chỉ hỏi mã qua kênh riêng khi chưa có, không hỏi lại nơi nhận hoặc sản phẩm đã rõ.
- Trong lúc chưa có SLA được duyệt, vẫn có thể soạn lời đáp nêu đúng trạng thái và bước kiểm tra tiếp theo, không hứa thời hạn cập nhật hay đạt policy sàn.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Xác định mốc khách đang hỏi.
- Đối chiếu sự kiện và bằng chứng bàn giao, giữ riêng phần chưa xác minh.
- Chỉ nêu thời hạn nếu đủ điều kiện SLA; nếu thiếu thì nêu bước kiểm tra.

**Đầu ra cần có**

- Lời giải thích đúng mốc đã biết và dữ kiện/bước kiểm tra còn cần.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mã/sự kiện của đơn nếu đang hỏi tiến độ; line/nền tảng và SLA có hiệu lực nếu đang hỏi cam kết.

**Điều cần tránh**

- Gọi label created là hãng đã nhận kiện
- Khẳng định chưa gửi chỉ vì chưa có scan
- Hứa active trong một số giờ mặc định
- Đồng nhất mã hợp lệ với đáp ứng mọi điều kiện của sàn

**Mục liên quan**

- [Trạng thái hệ thống, label và sự kiện giao nhận](commercial.md#warehouse-status-interpretation)
- [Phân biệt Ship By Seller và Ship By TikTok](shipping.md#training-warehouse-label-mode)
- [Chính sách và thời gian phải theo đúng tuyến](shipping.md#shipping-policy-scope)
- [Tiếp nhận trouble có bằng chứng](commercial.md#training-cskh-trouble-intake)

Nguồn: Người dùng cho phép dùng phán đoán để tiếp tục biên soạn (user-interim-judgment); Training sale Express: tuyến, giá và FAQ (training-sale-express-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 5: tình huống thường gặp (training-dropship-exceptions-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Warehouse phần 3: nhập kho, đơn ra, hàng hoàn (training-warehouse-flow-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).
