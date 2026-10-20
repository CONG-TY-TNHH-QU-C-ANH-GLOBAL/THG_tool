---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 05. Tìm nguồn 1688/Taobao

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="sourcing-routing"></a>

## Chọn POD hay tìm nguồn theo quyết định task

Mã: `sourcing-routing` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Người dùng/leader đã chốt trong phạm vi task. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Ưu tiên POD phù hợp cho nhu cầu in/cá nhân hóa; yêu cầu tìm nguồn/nhập số lượng được xử lý sourcing. Không có catalog phù hợp được đề xuất nguồn sàn ghi rõ.

**Cách tư vấn / xử lý đề xuất**

- Quyết định theo nhu cầu thực và phần phủ định trong bài. Không luân phiên POD/Dropship theo lượt; không chèn một sản phẩm catalog không liên quan để đủ khối sản phẩm.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Dịch vụ khách thực sự đang tìm nếu bài mơ hồ.

**Điều cần tránh**

- Ép bài tìm nguồn vào POD chỉ vì catalog có tiêu đề gần giống.

Nguồn: Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="sourcing-provided-link"></a>

## Khách đã cung cấp link sản phẩm

Mã: `sourcing-provided-link` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- customer product link
- exact listing

**Cách tư vấn / xử lý đề xuất**

- Ưu tiên tra listing khách đưa trước khi tìm món khác. Kiểm tra đúng item ID và biến thể; không mặc định ảnh khách đăng khớp toàn bộ biến thể trong listing. Không hỏi lại link đã có.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Phân loại/biến thể cần mua nếu listing có nhiều lựa chọn.

**Điều cần tránh**

- Tự thay một listing khác mà không nêu là phương án thay thế.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

<a id="sourcing-similar"></a>

## Gắn nhãn mẫu tương tự

Mã: `sourcing-similar` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đối chiếu code; không xác nhận năng lực kinh doanh hay production. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- similar model
- similar product
- manual comparison

**Dữ kiện có nguồn**

- Code hiện đánh dấu ứng viên nguồn sàn tìm theo tên là mẫu tương tự; không có bước đối chiếu ảnh bài gốc trong builder gợi ý.

**Cách tư vấn / xử lý đề xuất**

- Giữ nhãn trong thẻ nội bộ và lời nhắn dự kiến gửi khách khi chưa xác nhận đúng mẫu. Chứng cứ hình ảnh và đặc tính phải được xem cùng nhau.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách cần đúng model hay chấp nhận tương đương?

**Điều cần tránh**

- Dùng từ đúng mẫu hoặc chính hãng khi chưa có bằng chứng.

Nguồn: THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf); Yêu cầu về chất lượng gợi ý (user-quality).

<a id="sourcing-china-alternative"></a>

## Khách muốn nguồn châu Âu nhưng tìm được nguồn Trung Quốc

Mã: `sourcing-china-alternative` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Người dùng/leader đã chốt trong phạm vi task. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Người dùng cho phép hiển thị phương án nguồn Trung Quốc thay thế, có ghi rõ xuất xứ.

**Cách tư vấn / xử lý đề xuất**

- Không mô tả nhà cung cấp Trung Quốc là ở châu Âu. Nêu đây là lựa chọn khác và kiểm tra khách có chấp nhận; điều kiện địa lý khách yêu cầu vẫn phải được giữ lại.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách có chấp nhận nguồn Trung Quốc thay thế không?

**Điều cần tránh**

- Xem nơi giao châu Âu như bằng chứng nhà cung cấp đặt tại châu Âu.

Nguồn: Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="sourcing-price-and-moq"></a>

## Giá nguồn, MOQ và giá theo bậc

Mã: `sourcing-price-and-moq` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- MOQ
- quantity pricing
- supplier price

**Cách tư vấn / xử lý đề xuất**

- Giữ nguyên tiền tệ và đơn vị gốc. Kiểm tra MOQ, SKU/biến thể và bậc giá ứng với lượng đặt mỗi lần. Sản lượng tháng không tự chứng minh một lần mua đạt bậc giá. Gắn nhãn giá nguồn tham khảo và ngày tra cứu.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Lượng đặt mỗi lần, đơn vị bán và biến thể.

**Điều cần tránh**

- Dùng giá thấp nhất của listing như giá đã chốt cho khách.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

<a id="sourcing-lookup-failure"></a>

## Không tìm thấy nguồn và lỗi tra cứu

Mã: `sourcing-lookup-failure` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Phân biệt mô tả thiếu, đã tìm nhưng không khớp, API lỗi, hết lượt và quá thời gian. Sale nhận trạng thái phù hợp; khách nhận câu hỏi cần thiết hoặc bước kiểm tra tiếp theo. Đã có đủ mô tả thì không tự xin lại toàn bộ dữ kiện.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Chỉ hỏi chi tiết giúp phân biệt sản phẩm hoặc báo giá.

**Điều cần tránh**

- Nói không có hàng chỉ vì dịch vụ tra cứu lỗi.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).

<a id="sourcing-platform-scope"></a>

## Nguồn mua ngoài phạm vi công bố

Mã: `sourcing-platform-scope` · Loại: `faq` · Tham chiếu công khai

Trạng thái: Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- FAQ dịch vụ nêu chưa hỗ trợ mua trên AliExpress/SHEIN, đề xuất tìm mẫu tương tự trên Taobao/1688.

**Cách tư vấn / xử lý đề xuất**

- Trình bày theo phạm vi đang được xác nhận; nếu khách đưa link ngoài nguồn hỗ trợ, có thể dùng làm tham chiếu mẫu và hỏi có chấp nhận nguồn khác.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Owner xác nhận nếu phạm vi nền tảng đã thay đổi.

**Điều cần tránh**

- Hứa đặt hộ mọi sàn thương mại.

Nguồn: [Dịch vụ THG Fulfill](https://thgfulfill.com/vi/thg-fulfill).

<a id="training-sourcing-discovery"></a>

## Bộ dữ kiện tìm nguồn trong Training

Mã: `training-sourcing-discovery` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Các phần đã đọc yêu cầu xác định sản phẩm, thị trường giao, mô hình kinh doanh, ngân sách và thời gian mong muốn; SOP sale bổ sung nền tảng bán và sản lượng tháng.

**Cách tư vấn / xử lý đề xuất**

- Đưa thành bản nhu cầu dùng chung, nhưng chỉ hỏi điều đang thiếu. Ngân sách có ích khi so ứng viên; nền tảng bán chỉ hỏi khi ảnh hưởng label/quy trình. Không gửi nguyên một bảng câu hỏi dài trong phản hồi đầu tiên.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Ưu tiên sản phẩm, lượng mua và nơi nhận; hỏi ngân sách/thời gian nếu cần chọn phương án.

**Điều cần tránh**

- Suy giá mục tiêu từ doanh thu hay tên nền tảng khách bán.

Nguồn: Training: tìm nguồn và đánh giá sản phẩm (training-sourcing-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training: SOP sale và bàn giao (training-sales-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="training-supplier-assessment"></a>

## Đánh giá ứng viên nguồn thay vì chỉ lấy link đầu tiên

Mã: `training-supplier-assessment` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Bản training hướng dẫn xem lượng bán, review, đặc tính sản phẩm, nơi xuất và loại shop; đồng thời khuyên tìm hiểu kỹ nhà cung cấp trước hợp tác.
- Phần Dropship mới còn hướng dẫn so nhiều supplier theo giá bậc, MOQ, ship nội địa, review và thời gian gửi.

**Cách tư vấn / xử lý đề xuất**

- Kiểm tra review liên quan chất lượng hay vận chuyển, mức phù hợp đặc tính, MOQ và quy cách. Xem chỉ số theo thời điểm, không coi badge hoặc doanh số là bảo đảm. Không sao chép chuẩn badge hoặc danh sách NCC riêng thành dữ kiện công khai.
- Các từ hiện hàng hoặc miễn ship trên listing chỉ là đầu mối kiểm tra đúng biến thể, địa chỉ và điều kiện; không chứng minh tồn thực hay ship miễn phí tới mọi nơi. Không khẳng định 1688 luôn rẻ nhất hoặc nguồn được bảo đảm chỉ theo điểm shop.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Thông số/biến thể và bằng chứng chất lượng cần thiết để sale cân nhắc.

**Điều cần tránh**

- Gọi NCC uy tín hoặc đã kiểm định chỉ dựa một badge.

Nguồn: Training: tìm nguồn và đánh giá sản phẩm (training-sourcing-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 2: tra nguồn và đánh giá (training-dropship-search-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="sourcing-qc-and-packing-scope"></a>

## Tìm đúng hàng, kiểm hàng và đóng gói cần thống nhất phạm vi

Mã: `sourcing-qc-and-packing-scope` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Các phần Ops mô tả tìm/đặt hàng, kiểm tra tại kho, đóng gói và chọn line theo loại hàng/địa chỉ.

**Áp dụng khi**

- Khách lo hàng không giống ảnh hoặc cần kiểm trước khi ship.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Mẫu/biến thể cần đối chiếu
- Tiêu chí kiểm
- Số lượng và cách đóng gói
- Phạm vi dịch vụ được nhận

**Cách tư vấn / xử lý đề xuất**

- Tách đánh giá listing với kiểm hàng thực tế. Xác nhận cần kiểm ngoại quan, số lượng, mẫu/biến thể hay chức năng, và đầu mối có thể thực hiện; không biến mô tả QC chung thành bảo đảm mọi lỗi đều được phát hiện.

**Đầu ra cần có**

- Yêu cầu kiểm hàng và đóng gói có tiêu chí rõ.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Tiêu chí kiểm nào quan trọng nhất với sản phẩm này?

**Điều cần tránh**

- Nói đã kiểm hàng từ kết quả tìm kiếm API hoặc hứa kiểm định chuyên ngành chưa xác nhận.

**Mục liên quan**

- [Đánh giá ứng viên nguồn thay vì chỉ lấy link đầu tiên](sourcing.md#training-supplier-assessment)
- [Gắn nhãn mẫu tương tự](sourcing.md#sourcing-similar)
- [Xác nhận bao bì trước khi xử lý](products.md#warehouse-packaging-agreement)

Nguồn: Training: dịch vụ theo góc nhìn vận hành (training-ops-services-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training: tìm nguồn và đánh giá sản phẩm (training-sourcing-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="sourcing-platform-scope-conflict"></a>

## Nguồn mua được nhắc trong Training chưa đồng nghĩa phạm vi hiện hành

Mã: `sourcing-platform-scope-conflict` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Một số tài liệu Ops liệt kê thêm nền tảng/nguồn ngoài Taobao và 1688; phạm vi này rộng hơn phần hỗ trợ được xác nhận trong task.
- Hai phần Dropship mới mô tả dùng ảnh từ link sàn khác làm mẫu tham chiếu và tìm mẫu tương tự trên 1688/Taobao, thay vì mặc định nhận mua trực tiếp mọi sàn.

**Áp dụng khi**

- Khách đưa link ngoài phạm vi provider/tài liệu đang được duyệt.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Nền tảng/link
- Mẫu cần mua
- Phạm vi hỗ trợ đã xác nhận

**Cách tư vấn / xử lý đề xuất**

- Giữ Taobao/1688 là phạm vi sourcing đã chốt cho task. Khi khách yêu cầu nền tảng khác, kiểm tra hỗ trợ hiện hành và nêu rõ nếu chỉ dùng link làm mẫu tham chiếu.

**Đầu ra cần có**

- Xác nhận có hỗ trợ hoặc đề xuất nguồn thay thế minh bạch.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Owner dịch vụ xác nhận có nhận mua từ nền tảng đó không?

**Điều cần tránh**

- Suy công ty hiện nhận mọi nền tảng được nhắc trong tài liệu lịch sử.

**Mục liên quan**

- [Nguồn mua ngoài phạm vi công bố](sourcing.md#sourcing-platform-scope)
- [Dịch vụ tìm nguồn và mua hàng](services.md#service-sourcing)
- [Khách đã cung cấp link sản phẩm](sourcing.md#sourcing-provided-link)

Nguồn: Training: dịch vụ theo góc nhìn vận hành (training-ops-services-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); [Dịch vụ THG Fulfill](https://thgfulfill.com/vi/thg-fulfill); [Đặt hàng Taobao/1688 giao Mỹ](https://thgfulfill.com/vi/thg-order); Training Dropship phần 2: tra nguồn và đánh giá (training-dropship-search-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training Dropship phần 5: tình huống thường gặp (training-dropship-exceptions-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="training-visual-search-boundary"></a>

## Tìm bằng ảnh trong Training và khả năng của hệ thống

Mã: `training-visual-search-boundary` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Snapshot Dropship hướng dẫn nhân viên lấy ảnh rõ từ seller, tìm bằng camera của sàn rồi so các nhà cung cấp. Đây là mô tả thao tác của người dùng trên sàn.

**Áp dụng khi**

- Khách đưa ảnh mẫu hoặc yêu cầu đúng model.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Ảnh gốc của khách
- Đặc tính bắt buộc
- Bằng chứng năng lực tìm kiếm của provider

**Cách tư vấn / xử lý đề xuất**

- Dùng ảnh để lập yêu cầu đặc tính khi đã thực sự đọc được ảnh. Nếu provider chỉ tìm theo tên, giữ nhãn mẫu tương tự và yêu cầu đối chiếu; chỉ gọi là tìm/khớp bằng ảnh khi luồng đó đã có bằng chứng thực thi.

**Đầu ra cần có**

- Mức độ khớp rõ: tham chiếu hình dáng, mẫu tương tự hoặc model đã xác minh.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Ảnh gốc đủ rõ hoặc link/mã nếu cần đúng model.

**Điều cần tránh**

- Suy hệ thống đã tìm bằng ảnh chỉ vì Training có hướng dẫn camera
- Dùng ảnh catalog trả về thay bằng chứng đã hiểu ảnh bài gốc

**Mục liên quan**

- [Ảnh catalog, ảnh bài đăng và model](products.md#product-images-and-model)
- [Hiểu cùng văn bản và ảnh gốc](replies.md#reply-multimodal)
- [Gắn nhãn mẫu tương tự](sourcing.md#sourcing-similar)

Nguồn: Training Dropship phần 2: tra nguồn và đánh giá (training-dropship-search-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); THG_tool Knowledge OS và luồng gợi ý (repo-tool, THG_tool@b2a3cf6dcf21c88277875960b746a8258db98adf).
