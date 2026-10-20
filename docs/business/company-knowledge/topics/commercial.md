---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 07. Quy trình, cam kết và chính sách

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="commercial-order-process"></a>

## Quy trình bắt đầu dịch vụ sourcing

Mã: `commercial-order-process` · Loại: `faq` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Trang đặt hàng mô tả gửi link, kiểm tra/báo giá, xác nhận mua, kiểm hàng và giao Mỹ.

**Cách tư vấn / xử lý đề xuất**

- Trong lời nhắn đầu tiên, nêu bước khách cần làm tiếp theo. Trước khi đặt hàng/chuyển tiền phải có phương án và thông tin giao dịch được người phụ trách xác nhận.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Thông tin sản phẩm và phạm vi cần hỗ trợ.

**Điều cần tránh**

- Tự xác nhận đơn, đặt hàng hoặc thu tiền chỉ từ gợi ý AI.

Nguồn: [Đặt hàng Taobao/1688 giao Mỹ](https://thgfulfill.com/vi/thg-order).

<a id="commercial-price-layers"></a>

## Giá nguồn, phí dịch vụ, cước và giá bán

Mã: `commercial-price-layers` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Mỗi con số cần nhãn, tiền tệ, đơn vị và phần bao gồm/loại trừ. Giá nguồn không phải giá THG bán; cước tham chiếu không phải báo giá trọn gói. Không tự chuyển tiền tệ khi thiếu tỷ giá và quy tắc được duyệt.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mức giá cuối cùng và thời hạn được người phụ trách duyệt.

**Điều cần tránh**

- Tự cộng margin/phí hoặc báo landed cost khi thiếu thành phần.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="commercial-source-price-audience"></a>

## Giá nguồn trong nhóm sale và lời nhắn gửi khách

Mã: `commercial-source-price-audience` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Người dùng/leader đã chốt trong phạm vi task. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Người dùng xác nhận giá nguồn hiển thị trong câu trả lời đổ về nhóm Telegram THG_Sale_Lead.

**Cách tư vấn / xử lý đề xuất**

- Phạm vi xác nhận là thông báo nội bộ task này. Xác định phần thông tin cho sale và bản nháp khách nhận; không mở quyền xem toàn bộ dữ liệu vốn/NCC của CRM từ quyết định này.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Quy tắc được duyệt về giá nguồn trong lời nhắn sale gửi ra ngoài.

**Điều cần tránh**

- Suy mọi thông tin chi phí nội bộ đều được công khai.

Nguồn: Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="commercial-compensation"></a>

## Đền bù, hoàn tiền và gửi lại

Mã: `commercial-compensation` · Loại: `banned_claim` · Nội bộ sale

Trạng thái: Chưa đủ căn cứ xác nhận điều khoản. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Các trang giới thiệu và FAQ trình bày quyền lợi ở mức khác nhau; chưa đọc trọn điều khoản dạng ảnh để xác định ngoại lệ. Khi có khiếu nại, thu mã đơn/tracking và tình huống rồi chuyển người phụ trách đối chiếu chính sách dịch vụ.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Bản chính sách đang hiệu lực, phạm vi bảo hiểm, bằng chứng và thời hạn khiếu nại.

**Điều cần tránh**

- Hứa hoàn toàn bộ hoặc gửi lại miễn phí khi chưa xác nhận điều kiện.

Nguồn: [Mục chính sách](https://thgfulfill.com/vi/policy); [Giới thiệu THG Fulfill](https://thgfulfill.com/vi); [Dịch vụ THG Fulfill](https://thgfulfill.com/vi/thg-fulfill).

<a id="commercial-onboarding"></a>

## Lên đơn, thanh toán và tích hợp

Mã: `commercial-onboarding` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Chưa đủ căn cứ xác nhận điều khoản. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Website mô tả nhiều kênh làm việc như hệ thống và file chung. Chưa xác định kênh chuẩn cho từng dịch vụ; gửi hướng dẫn theo SOP hiện hành sau khi được cấp tài khoản/đầu mối. Chỉ xác nhận tích hợp nền tảng khi có hỗ trợ đúng chức năng cần dùng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- SOP lên đơn, thanh toán, đầu mối và quy trình onboarding theo dịch vụ.

**Điều cần tránh**

- Tự gửi tài khoản thanh toán hoặc khẳng định đã đồng bộ cửa hàng của khách.

Nguồn: [Dịch vụ THG Fulfill](https://thgfulfill.com/vi/thg-fulfill); [Dịch vụ THG Express](https://thgfulfill.com/vi/thg-express); THG Training: nguồn tài liệu quản lý và signed export (repo-training, THG_Training@ec170a949623).

<a id="training-sales-handover"></a>

## Thông tin tư vấn phải giữ được khi bàn giao

Mã: `training-sales-handover` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Phần bàn giao trong SOP snapshot nêu dịch vụ đã chốt, sản phẩm, điều khoản riêng, hợp đồng và vai trò sale làm đầu mối thương mại.

**Cách tư vấn / xử lý đề xuất**

- Giữ nội dung khách yêu cầu, phương án và điều kiện đã trao đổi để Ops/CSKH không phải hỏi lại. Phân biệt giá/điều kiện đang đề xuất với thỏa thuận đã được duyệt. Chưa đưa thông tin liên hệ cá nhân hoặc hồ sơ khách thật vào kho dùng chung.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Phần nào đã được chốt và ai xác nhận?

**Điều cần tránh**

- Coi câu gợi ý AI là điều khoản hợp đồng hoặc booking đã xác nhận.

Nguồn: Training: SOP sale và bàn giao (training-sales-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="training-cskh-trouble-intake"></a>

## Tiếp nhận trouble có bằng chứng

Mã: `training-cskh-trouble-intake` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Bản CSKH hướng dẫn lấy mã đơn, mô tả, thông tin sản phẩm, ảnh/video/tracking và phân biệt lỗi sản xuất, vận chuyển, thông tin seller.

**Cách tư vấn / xử lý đề xuất**

- Khi bài là khiếu nại, trả lời theo nhu cầu xử lý sự cố thay vì tìm sản phẩm mới. Chỉ xin bằng chứng chưa có; chuyển người phụ trách đối chiếu điều khoản, chưa quy trách nhiệm hoặc hứa bồi thường.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mã đơn/tracking, loại sự cố và bằng chứng còn thiếu.

**Điều cần tránh**

- Phán lỗi bên nào chỉ từ một ảnh hay lời kể ngắn.

Nguồn: Training: tiếp nhận yêu cầu và trouble (training-cskh-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="dropship-quote-components"></a>

## Các thành phần cần kiểm tra trong báo giá Dropship

Mã: `dropship-quote-components` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Training tách tiền hàng, ship nội địa Trung Quốc, phí dịch vụ/mua hộ, tỷ giá và ship quốc tế; mô tả tỷ giá do kế toán cập nhật.

**Áp dụng khi**

- Sale đã có ứng viên nguồn và cần giải thích cấu phần giá.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Giá và biến thể đúng listing
- MOQ/lượng mua mỗi lần
- Phí nội địa nguồn
- Tỷ giá và phí được duyệt
- Cước đúng tuyến/kiện

**Cách tư vấn / xử lý đề xuất**

- Giữ từng cấu phần cùng đơn vị/tiền tệ và thời điểm. Kiểm tra tiền hàng đã gồm ship nội địa chưa. Hệ số, tỷ giá, phụ phí và thời gian trong bài training cần xác nhận mới; không lấy bài tập làm dữ liệu báo giá.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Xác minh giá nguồn và biến thể.
- Kiểm tra phí nội địa có nằm trong giá chưa.
- Lấy quy tắc quy đổi/phí đang hiệu lực từ đầu mối được duyệt.
- Tính cước theo quy cách thực.
- Nêu phần bao gồm, phần chưa gồm và hiệu lực báo giá.

**Đầu ra cần có**

- Bảng cấu phần để người có thẩm quyền duyệt.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Phí nội địa, đơn vị bán, lượng mua, tỷ giá/phí hiện hành và quy cách ship.

**Điều cần tránh**

- Chốt landed cost từ giá listing hoặc dùng tỷ giá/hệ số ví dụ như quy định hiện tại.

**Mục liên quan**

- [Giá nguồn, MOQ và giá theo bậc](sourcing.md#sourcing-price-and-moq)
- [Giá nguồn, phí dịch vụ, cước và giá bán](commercial.md#commercial-price-layers)
- [Giá nguồn trong nhóm sale và lời nhắn gửi khách](commercial.md#commercial-source-price-audience)
- [Cách xử lý tạm thời khi basecost chưa rõ phạm vi](commercial.md#commercial-basecost-interpretation)

Nguồn: Training Dropship phần 3: cấu phần báo giá (training-dropship-pricing-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="warehouse-status-interpretation"></a>

## Trạng thái hệ thống, label và sự kiện giao nhận

Mã: `warehouse-status-interpretation` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Bản warehouse phân biệt trạng thái đã mua label nhưng chưa xuất với đã xử lý/bàn giao. Tên và cách ánh xạ trạng thái là mô tả của snapshot.

**Áp dụng khi**

- Khách hỏi đơn đã gửi chưa hoặc tracking chưa cập nhật.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Trạng thái đơn
- Sự kiện nhận hàng của hãng
- Thời điểm cập nhật

**Cách tư vấn / xử lý đề xuất**

- Dựa vào dữ liệu sự kiện hiện hành để nói đúng việc đã xảy ra. Nếu tracking đứng, kiểm tra hãng đã nhận hàng hay mới tạo label trước khi quy lỗi hoặc hứa hạn xử lý.

**Đầu ra cần có**

- Giải thích đúng chặng và bước kiểm tra tiếp theo.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mã đơn/tracking và trạng thái hiện tại nếu chưa có.

**Điều cần tránh**

- Đọc chữ Ready/Processed thành đã giao thành công mà không kiểm tra nghĩa trong hệ thống đang dùng.

**Mục liên quan**

- [Chuỗi xử lý một đơn POD](services.md#pod-order-workflow)
- [Điều kiện xử lý đơn xuất kho](services.md#warehouse-outbound-procedure)
- [Tiếp nhận trouble có bằng chứng](commercial.md#training-cskh-trouble-intake)
- [Cách xử lý tạm thời khi chưa có SLA active tracking](shipping.md#shipping-tracking-milestones)

Nguồn: Training Warehouse phần 3: nhập kho, đơn ra, hàng hoàn (training-warehouse-flow-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training POD phần 3: chuỗi xử lý đơn (training-pod-workflow-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="warehouse-return-refund-reship"></a>

## Return, refund và reship là ba việc khác nhau

Mã: `warehouse-return-refund-reship` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- returns refunds reshipment
- returned inventory

**Dữ kiện có nguồn**

- Training phân biệt nhận hàng hoàn để kiểm/tồn/hủy, hoàn tiền theo điều kiện và tạo đơn gửi thay thế; nguyên nhân quyết định hướng xử lý.

**Áp dụng khi**

- Khách hỏi hàng hoàn, đổi hàng, gửi lại hoặc hoàn tiền.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Mã đơn và lý do
- Tình trạng/bằng chứng
- Điều khoản áp dụng
- Phương án khách mong muốn

**Cách tư vấn / xử lý đề xuất**

- Ghi nhận khách muốn hoàn hàng, hoàn tiền hay gửi lại. Đối chiếu nguyên nhân và thỏa thuận hiện hành; việc kho nhận hàng return chưa đồng nghĩa được refund hoặc được bán lại.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Phân loại yêu cầu và nguyên nhân.
- Kiểm tra hàng/bằng chứng và chính sách.
- Xác nhận phương án cùng người phụ trách.
- Ghi nhận giao dịch hoặc đơn thay thế nếu được duyệt.

**Đầu ra cần có**

- Phương án đúng quyền lợi và trách nhiệm đã xác nhận.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Tình trạng hàng, mã đơn, lý do return và mong muốn xử lý.

**Điều cần tránh**

- Hứa refund/reship hoặc nhập lại tồn bán được chỉ từ việc khách nói sẽ trả hàng.

**Mục liên quan**

- [Đền bù, hoàn tiền và gửi lại](commercial.md#commercial-compensation)
- [Tiếp nhận trouble có bằng chứng](commercial.md#training-cskh-trouble-intake)
- [Nhập hàng vào kho Mỹ](services.md#warehouse-inbound-procedure)

Nguồn: Training Warehouse phần 3: nhập kho, đơn ra, hàng hoàn (training-warehouse-flow-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="warehouse-price-discrepancy"></a>

## Phí thực tế khác bảng hoặc khác dự kiến

Mã: `warehouse-price-discrepancy` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Phần giá lệch trong Training yêu cầu báo Ops kiểm tra trước khi tự giải thích với khách.

**Áp dụng khi**

- Khách phản ánh lệch phí hoặc sale thấy bảng/đơn không khớp.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Báo giá và điều kiện trước đó
- Dữ liệu tính phí thực
- Phiên bản bảng
- Khoản khác biệt

**Cách tư vấn / xử lý đề xuất**

- Thu đủ dữ kiện của phép so sánh: ngày/phiên bản bảng, đơn, kiện, zone, label và phạm vi phí. Nói rõ đang kiểm tra điểm chênh thay vì kết luận lỗi hãng hoặc tự hứa hoàn chênh lệch.

**Đầu ra cần có**

- Yêu cầu đối soát cụ thể cho Ops/Pricing.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Bảng/mức phí đang so, quy cách kiện thực và khoản chênh cụ thể.

**Điều cần tránh**

- Tự điều chỉnh tỷ giá, giảm phí hoặc quy trách nhiệm khi chưa đối chiếu.

**Mục liên quan**

- [Tách phí kho, bao bì và vận chuyển nội địa](shipping.md#warehouse-fee-components)
- [Giá nguồn, phí dịch vụ, cước và giá bán](commercial.md#commercial-price-layers)
- [Tách kiến thức ổn định và dữ liệu phải tra mới](governance.md#governance-dynamic-data)

Nguồn: Training Warehouse phần 2: nhóm phí và xử lý chênh lệch (training-warehouse-fees-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="commercial-quote-comparison"></a>

## Khách thấy giá cao: so cùng phạm vi trước khi đề xuất

Mã: `commercial-quote-comparison` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Training xử lý phản hồi giá cao bằng việc hỏi nguồn so sánh, làm rõ khoản đã gồm, tách chi phí và cân nhắc phương án nguồn/cách gửi khác.

**Áp dụng khi**

- Khách phản hồi giá cao hoặc hỏi vì sao khác nhà cung cấp khác.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Hai phương án được so
- Sản phẩm/biến thể
- Số lượng
- Điểm giao và phạm vi phí

**Cách tư vấn / xử lý đề xuất**

- So cùng biến thể, chất lượng, lượng mua, điểm giao và cấu phần phí trước khi kết luận. Chỉ đề xuất đổi supplier, điều chỉnh lượng hoặc gộp hàng nếu phù hợp nhu cầu và đã kiểm điều kiện; không tự giảm giá hay hứa tiết kiệm.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Làm rõ cơ sở so sánh.
- Tách khoản có/không có ở mỗi bên.
- Đề xuất thay đổi phù hợp rồi kiểm tra giá mới.

**Đầu ra cần có**

- Giải thích khoản khác biệt hoặc phương án cần báo lại.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Báo giá đang so đã gồm khoản nào và áp dụng cho quy cách nào?

**Điều cần tránh**

- Tự hứa giảm giá, cashback hoặc ưu đãi từ ví dụ Training
- Đổi sang sản phẩm kém hơn mà không nêu rõ khác biệt

**Mục liên quan**

- [Các thành phần cần kiểm tra trong báo giá Dropship](commercial.md#dropship-quote-components)
- [Giá nguồn, phí dịch vụ, cước và giá bán](commercial.md#commercial-price-layers)
- [Phí thực tế khác bảng hoặc khác dự kiến](commercial.md#warehouse-price-discrepancy)

Nguồn: Training Dropship phần 5: tình huống thường gặp (training-dropship-exceptions-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 3: cấu phần báo giá (training-dropship-pricing-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="commercial-supplier-delay"></a>

## Hàng chậm từ supplier về kho: xác minh đúng chặng

Mã: `commercial-supplier-delay` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- supplier delay
- supplier not dispatched

**Dữ kiện có nguồn**

- Training mô tả kiểm nguyên nhân chậm với Ops, chủ động cập nhật seller và cân nhắc nguồn dự phòng khi nhà cung cấp không đáp ứng.

**Áp dụng khi**

- Khách hỏi tiến độ hàng về kho nguồn hoặc supplier giao trễ.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Đơn/link mua
- Sự kiện gửi/nhận thực tế
- Cam kết trước đó

**Cách tư vấn / xử lý đề xuất**

- Phân biệt supplier chưa gửi, đang đi nội địa nguồn và kho đã nhận nhưng chưa xử lý. Chỉ cập nhật mốc mới sau khi có xác nhận; đổi nguồn phải được khách chấp nhận về mẫu, giá, số lượng và thời gian.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Kiểm tra trạng thái đúng chặng.
- Xác minh nguyên nhân với đầu mối xử lý.
- Thông báo phần đã biết và mốc tiếp theo có căn cứ.
- Xin xác nhận nếu cần thay phương án.

**Đầu ra cần có**

- Cập nhật tiến độ cụ thể, không quy lỗi khi thiếu bằng chứng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mã đơn và trạng thái/ước tính mới đã được xác nhận.

**Điều cần tránh**

- Báo hàng đang vận chuyển quốc tế khi vẫn chưa về kho nguồn
- Hứa giao lại trong ngày hoặc tự đổi nhà cung cấp

**Mục liên quan**

- [Tiếp nhận trouble có bằng chứng](commercial.md#training-cskh-trouble-intake)
- [Trạng thái hệ thống, label và sự kiện giao nhận](commercial.md#warehouse-status-interpretation)
- [Rõ ràng và xác nhận khi thông tin thay đổi](company.md#company-confirmation-culture)

Nguồn: Training Dropship phần 5: tình huống thường gặp (training-dropship-exceptions-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="commercial-basecost-interpretation"></a>

## Cách xử lý tạm thời khi basecost chưa rõ phạm vi

Mã: `commercial-basecost-interpretation` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- basecost inclusions
- included excluded fees
- quote scope

**Áp dụng khi**

- Khách/sale hỏi basecost đã bao gồm những gì hoặc cần tổng hợp một báo giá.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Nguồn báo giá và thẩm quyền/phạm vi dùng
- Dịch vụ, sản phẩm/biến thể và tuyến áp dụng
- Đơn vị, tiền tệ, lượng mua/kiện và hiệu lực
- Khoản đã gồm, tính riêng, không áp dụng hoặc chưa rõ

**Cách tư vấn / xử lý đề xuất**

- Đọc điều kiện của báo giá được duyệt, còn hiệu lực và đúng dịch vụ trước khi giải thích basecost. Từ basecost tự nó không xác định được đó là giá nguồn, chi phí nội bộ hay giá bán.
- Với từng khoản liên quan, phân biệt đã bao gồm, tính riêng, không áp dụng và chưa xác định. Không biến một ô trống thành phí bằng 0; không tự thêm phí chỉ vì checklist liệt kê khoản đó.
- Nếu báo giá đã ghi rõ khoản nào bao gồm thì giữ nguyên phạm vi đó, tránh cộng lại. Khi có khoản chưa rõ, chỉ nêu phần đã biết cùng điều kiện và ghi phần cần kiểm tra; không gọi tổng phần đã biết là tổng thanh toán cuối.
- Chỉ cộng các khoản khi cùng cơ sở đơn vị, lượng mua/kiện, tiền tệ hoặc có quy tắc quy đổi được duyệt; giữ giá nguồn, chi phí nội bộ và giá bán riêng. Thông báo sale trong task có thể có giá nguồn, bản nháp khách chỉ dùng phần được phép.
- Hỏi đúng dòng báo giá hoặc điều kiện còn thiếu, tận dụng dữ kiện đã có. Có thể tiếp tục soạn lời đáp và kiểm thử với cách xử lý này; không cần chờ chốt một định nghĩa basecost toàn công ty.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Xác định đúng báo giá và người nhận thông tin.
- Đọc phạm vi, đánh dấu trạng thái từng khoản liên quan.
- Giữ nguyên phần đã gồm; tính riêng chỉ với khoản được xác nhận.
- Nêu mức giá theo đúng phạm vi; ghi phần chưa xác định và bước kiểm tra.

**Đầu ra cần có**

- Giải thích có căn cứ về phần đã gồm/chưa gồm; tổng cuối chỉ khi đã đủ điều kiện.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Báo giá của dịch vụ nào, phiên bản/hiệu lực và khoản đã gồm còn chưa rõ?

**Điều cần tránh**

- Tự chọn định nghĩa giá all-in từ một snapshot Training
- Cộng hai lần cước hoặc phí đã nằm trong giá
- Khẳng định miễn phí vì dữ liệu không có số
- Công khai cấu phần vốn/phí nội bộ chưa được phép

**Mục liên quan**

- [Các thành phần cần kiểm tra trong báo giá Dropship](commercial.md#dropship-quote-components)
- [Giá nguồn, phí dịch vụ, cước và giá bán](commercial.md#commercial-price-layers)
- [Giá nguồn trong nhóm sale và lời nhắn gửi khách](commercial.md#commercial-source-price-audience)
- [Khách thấy giá cao: so cùng phạm vi trước khi đề xuất](commercial.md#commercial-quote-comparison)

Nguồn: Người dùng cho phép dùng phán đoán để tiếp tục biên soạn (user-interim-judgment); Training sale Express: tuyến, giá và FAQ (training-sale-express-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 3: cấu phần báo giá (training-dropship-pricing-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="commercial-quote-validity-scope"></a>

## Báo giá phải có phạm vi và thời điểm áp dụng

Mã: `commercial-quote-validity-scope` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Ngày hiệu lực hoặc mốc tra giá; quy cách kiện; phạm vi các chặng và khoản chưa bao gồm.

**Cách tư vấn / xử lý đề xuất**

- Mỗi báo giá cần giữ nguồn, thời điểm tra, chiều vận chuyển, đơn vị tính, thành phần gồm/chưa gồm và điều kiện áp dụng. Không suy hạn hiệu lực khi nguồn chưa nêu.
- Câu trả lời đầu tiên chỉ nhắc các điều kiện ảnh hưởng trực tiếp đến nhu cầu khách. Bảng đối chiếu chi tiết và ghi chú vận hành để ở phần nội bộ.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Đối chiếu bảng giá với nước xuất/nhận và loại hàng.
- Tách giá hàng, chi phí mua/kiểm/đóng gói, chặng quốc tế và nội địa.
- Nếu thiếu phạm vi/hiệu lực thì giữ là tham chiếu cần xác nhận, không gọi là báo giá trọn gói.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Ngày hiệu lực hoặc mốc tra giá; quy cách kiện; phạm vi các chặng và khoản chưa bao gồm.

**Điều cần tránh**

- Đưa giá ví dụ vào lời nhắn như báo giá hiện hành.
- Tự cộng phí hoặc quy đổi tiền khi chưa có nguồn/tỷ giá phù hợp.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); [Bảng giá quốc tế](https://thgfulfill.com/vi/international-pricing); [Biểu phí 3PL Mỹ](https://thgfulfill.com/vi/domestic-pricing); [Bảng giá chính ngạch Việt Nam–Mỹ](https://thgfulfill.com/vi/chinh-ngach-pricing).
