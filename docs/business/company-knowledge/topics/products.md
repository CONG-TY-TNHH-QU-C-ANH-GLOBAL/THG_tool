---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# 04. Sản phẩm POD và đặc tính

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

<a id="product-live-catalog"></a>

## Nguồn chuẩn của sản phẩm POD

Mã: `product-live-catalog` · Loại: `faq` · Nội bộ sale

Trạng thái: Đã đối chiếu code; không xác nhận năng lực kinh doanh hay production. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- POD catalog
- product variants
- catalog SKU

**Dữ kiện có nguồn**

- Contract catalog khai báo tên, SKU, ảnh, biến thể, vật liệu, giá và thông tin cập nhật; các trường có thể thiếu.

**Cách tư vấn / xử lý đề xuất**

- Dùng catalog được đồng bộ và duyệt thay vì tạo SKU giả trong tài liệu công ty. Trang catalog động chưa đọc ra dữ liệu không đồng nghĩa catalog không có sản phẩm.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Bản catalog/biến thể mới nhất phù hợp với yêu cầu.

**Điều cần tránh**

- Biến dữ liệu schema thành khẳng định có hàng thực tế.

Nguồn: Contract catalog công khai trong landing page (repo-catalog, THG_landingpage@4fd55f1); [Catalog công ty](https://thgfulfill.com/vi/catalog).

<a id="product-required-attributes"></a>

## Khớp loại sản phẩm và đặc tính bắt buộc

Mã: `product-required-attributes` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- material requirements
- product specifications

**Cách tư vấn / xử lý đề xuất**

- So loại sản phẩm, đối tượng sử dụng, vật liệu, model và đặc tính bắt buộc. Cùng vài từ trong tên chưa đủ; cotton khác polyester, stocking khác T-shirt. Phân biệt điều bắt buộc với sở thích có thể thương lượng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách có chấp nhận khác vật liệu/model khi cần đề xuất thay thế?

**Điều cần tránh**

- Giới thiệu ứng viên sai đặc tính như đúng mẫu khách yêu cầu.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="product-customization"></a>

## Khả năng in phải được xác nhận

Mã: `product-customization` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Người dùng/leader đã chốt trong phạm vi task. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Người dùng cho phép đề xuất nguồn sàn có ghi rõ khi nhu cầu POD không có sản phẩm catalog phù hợp.

**Cách tư vấn / xử lý đề xuất**

- Giữ riêng phôi hàng và dịch vụ in. Ưu tiên catalog công ty khi đúng nhu cầu; listing hàng trơn chưa chứng minh nhà cung cấp nhận in theo thiết kế.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Kỹ thuật/vị trí in và yêu cầu thiết kế nếu ảnh hưởng lựa chọn.

**Điều cần tránh**

- Hứa một sản phẩm mua ngoài chắc chắn có thể tùy chỉnh.

Nguồn: Quyết định của người dùng chuyển tiếp từ leader (owner-task).

<a id="product-negative-printing"></a>

## Nhận biết yêu cầu không in logo

Mã: `product-negative-printing` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- no logo
- blank products
- without printing

**Cách tư vấn / xử lý đề xuất**

- Có chữ logo/POD chưa đủ để chuyển dịch vụ. Cụm không in logo, không cần tùy chỉnh hoặc no custom printing phải được hiểu là phủ định trong phạm vi câu. Giữ trạng thái có yêu cầu, không yêu cầu và chưa rõ riêng nhau.

**Điều cần tránh**

- Xin mẫu thiết kế khi khách đã nói cần hàng trơn.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="product-images-and-model"></a>

## Ảnh catalog, ảnh bài đăng và model

Mã: `product-images-and-model` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Đọc ảnh bài gốc để hiểu mẫu khách nói tới; dùng ảnh catalog/nguồn để đối chiếu ứng viên. Logo, nhãn, hình dáng và mã nhìn rõ là dữ kiện; ảnh giống chưa xác nhận đúng SKU, chất liệu bên trong, xuất xứ hay trọng lượng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Ảnh nhãn/mã hoặc link khi cần phân biệt model.

**Điều cần tránh**

- Đánh đồng ảnh xem trước Telegram với việc AI đã phân tích ảnh.
- Không nói đã xem/đối chiếu ảnh hoặc khẳng định đúng mẫu khi luồng tạo câu trả lời chỉ nhận text.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality).

<a id="product-availability"></a>

## Tồn kho và giá sản phẩm là dữ liệu thay đổi

Mã: `product-availability` · Loại: `banned_claim` · Nội bộ sale

Trạng thái: Hướng dẫn đề xuất, chờ owner duyệt. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Cách tư vấn / xử lý đề xuất**

- Kiểm tra biến thể, tồn/cung ứng, thời gian sản xuất, tiền tệ và thời điểm cập nhật trước khi hứa giao. Giá từ một mức thấp nhất không mặc định áp dụng cho mọi size, màu, số lượng hoặc phương thức ship.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Biến thể và tình trạng cung ứng hiện tại.

**Điều cần tránh**

- Cam kết còn hàng hoặc giá cuối cùng từ một snapshot cũ.

Nguồn: Yêu cầu về chất lượng gợi ý (user-quality); Contract catalog công khai trong landing page (repo-catalog, THG_landingpage@4fd55f1).

<a id="training-pod-catalog-boundary"></a>

## Danh mục Training và catalog bán hiện tại

Mã: `training-pod-catalog-boundary` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Bản POD trong repo giới thiệu các nhóm apparel, phụ kiện, đồ gia dụng và dẫn tới catalog/template nội bộ.

**Cách tư vấn / xử lý đề xuất**

- Dùng các nhóm này để tìm nội dung/catalog liên quan, không tạo SKU bán hàng từ tên trong bài training. Kiểm tra catalog hiện tại cho basecost, sản xuất, template, biến thể và khả năng in.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Mã sản phẩm, template và thông số hiện hành phù hợp thiết kế.

**Điều cần tránh**

- Khẳng định mọi sản phẩm từng có trong training hiện còn nhận đơn.

Nguồn: Training: catalog POD (training-pod-snapshot, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Contract catalog công khai trong landing page (repo-catalog, THG_landingpage@4fd55f1).

<a id="pod-template-and-design"></a>

## Template, file thiết kế và phạm vi hỗ trợ design

Mã: `pod-template-and-design` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Từ khóa tra cứu; không mở rộng phạm vi dịch vụ**

- design file
- print template
- mockup requirements

**Dữ kiện có nguồn**

- Phần template trong Training phân biệt cấp template với làm thiết kế; mô tả seller chuẩn bị design và mockup, sản phẩm chưa có giá chuyển Ops báo giá.

**Áp dụng khi**

- Khách hỏi làm POD, xin template hoặc chưa có file in.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Mã/mẫu sản phẩm
- Biến thể và vị trí in
- File thiết kế/mockup nếu đã có

**Cách tư vấn / xử lý đề xuất**

- Xác định khách cần in từ file sẵn hay cần người thiết kế. Kiểm tra template đúng sản phẩm/biến thể và phạm vi hỗ trợ hiện hành trước khi trả lời.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Xác định sản phẩm phù hợp.
- Lấy template đúng loại từ nguồn quản lý/đầu mối Ops.
- Xác nhận trách nhiệm chuẩn bị thiết kế và kiểm tra tính phù hợp trước khi nhận đơn.

**Đầu ra cần có**

- Template đúng mẫu hoặc yêu cầu kiểm tra file/thiết kế rõ ràng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách đã có file thiết kế/mockup chưa, và cần in trên mẫu nào?

**Điều cần tránh**

- Hứa THG thiết kế miễn phí hoặc hứa in được trước khi kiểm tra file và phôi.

**Mục liên quan**

- [Dịch vụ POD](services.md#service-pod)
- [Khả năng in phải được xác nhận](products.md#product-customization)
- [Nguồn chuẩn của sản phẩm POD](products.md#product-live-catalog)

Nguồn: Training POD phần 2: catalog và template (training-pod-catalog-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="pod-mockup-qc"></a>

## Đối chiếu sản phẩm in với mockup

Mã: `pod-mockup-qc` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Tài liệu Ops nhấn mạnh không nhầm thiết kế giữa đơn POD; đối chiếu mockup về màu sắc, kích thước và vị trí in, rồi chọn bao bì phù hợp.

**Áp dụng khi**

- Khách hỏi độ giống mẫu, thay thiết kế hoặc báo lỗi in.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Mockup đã xác nhận
- SKU/biến thể
- Ảnh thực tế nếu có lỗi

**Cách tư vấn / xử lý đề xuất**

- Giữ file/mẫu đã xác nhận và điều kiện thay đổi. Nếu chưa có bằng chứng QC của đơn, chỉ nêu bước cần kiểm tra; không nói đơn đã đạt chất lượng.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Đối chiếu đúng đơn, đúng file và đúng biến thể.
- Ghi nhận khác biệt cụ thể về vị trí, kích thước, màu hoặc nội dung.
- Chuyển Ops/QC xác nhận nguyên nhân và phương án.

**Đầu ra cần có**

- Yêu cầu QC rõ đối tượng và điểm cần xác nhận.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Phiên bản thiết kế hoặc mockup nào đã được khách xác nhận?

**Điều cần tránh**

- Tự cam kết màu hoặc chất lượng tuyệt đối từ một ảnh màn hình.

**Mục liên quan**

- [Khớp loại sản phẩm và đặc tính bắt buộc](products.md#product-required-attributes)
- [Đền bù, hoàn tiền và gửi lại](commercial.md#commercial-compensation)
- [Tiếp nhận trouble có bằng chứng](commercial.md#training-cskh-trouble-intake)

Nguồn: Training: dịch vụ theo góc nhìn vận hành (training-ops-services-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="catalog-destination-conditions"></a>

## Giá catalog cần gắn thị trường và phạm vi ship

Mã: `catalog-destination-conditions` · Loại: `pricing_rule` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Bản POD phần 2 mô tả giá catalog theo điều kiện đi Mỹ và yêu cầu kiểm tra với Ops cho thị trường khác.

**Áp dụng khi**

- Khách hỏi giá catalog hoặc giao khác thị trường giá đang áp dụng.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- SKU/biến thể
- Xuất phát/fulfillment location
- Nước nhận
- Cấu phần giá hiện hành

**Cách tư vấn / xử lý đề xuất**

- Đối chiếu cấu phần giá hiện hành trước khi dùng, vì snapshot không chứng minh mọi SKU live có cùng cơ sở giá. Giữ riêng giá phôi/in, cước, nơi sản xuất và biến thể.

**Các bước tham chiếu; cần xác nhận SOP hiện hành**

- Đọc điều kiện của giá đúng sản phẩm.
- Kiểm tra điểm đến và các khoản đã bao gồm.
- Chuyển Ops xác nhận khi phạm vi khác hoặc giá thiếu.

**Đầu ra cần có**

- Giá có cơ sở áp dụng hoặc yêu cầu báo giá bổ sung.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Giá đang xem đã gồm phần nào, áp dụng SKU và điểm nhận nào?

**Điều cần tránh**

- Áp nguyên giá đi Mỹ cho Canada/Úc/EU hoặc cộng trùng cước đã có trong giá.

**Mục liên quan**

- [Nguồn chuẩn của sản phẩm POD](products.md#product-live-catalog)
- [Giá nguồn, phí dịch vụ, cước và giá bán](commercial.md#commercial-price-layers)
- [Ba bảng giá có mục đích riêng](shipping.md#shipping-three-tables)

Nguồn: Training POD phần 2: catalog và template (training-pod-catalog-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).

<a id="warehouse-packaging-agreement"></a>

## Xác nhận bao bì trước khi xử lý

Mã: `warehouse-packaging-agreement` · Loại: `sales_playbook` · Nội bộ sale

Trạng thái: Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành. Mọi mục vẫn chờ duyệt trước khi nạp runtime.

**Dữ kiện có nguồn**

- Training yêu cầu trao đổi rõ bao bì khi onboard, ghi vào hồ sơ; tài liệu Ops liên hệ lựa chọn bao bì với bảo vệ sản phẩm và trọng lượng.

**Áp dụng khi**

- Hàng dễ hỏng, cần branding hoặc khách hỏi vì sao trọng lượng/cước khác.

**Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay**

- Đặc tính hàng
- Yêu cầu bảo vệ
- Yêu cầu thương hiệu
- Quy cách đóng kiện

**Cách tư vấn / xử lý đề xuất**

- Hỏi yêu cầu đóng gói khi ảnh hưởng chất lượng, hình ảnh thương hiệu hoặc cước. Phân biệt bao bì theo mặc định với vật tư/dịch vụ tùy chỉnh chưa được duyệt.

**Đầu ra cần có**

- Quy cách bao bì được xác nhận để đối chiếu phí và chất lượng.

**Chỉ hỏi khi thiếu hoặc cần xác nhận**

- Khách cần túi/hộp, chống sốc, chèn thiệp hay yêu cầu nhãn riêng nào?

**Điều cần tránh**

- Hứa vật tư/đóng gói thương hiệu miễn phí hoặc bỏ qua chi phí tăng theo kiện.

**Mục liên quan**

- [Tách phí kho, bao bì và vận chuyển nội địa](shipping.md#warehouse-fee-components)
- [Sản lượng tháng, một lô và một kiện](shipping.md#shipping-period-and-parcel)
- [Đối chiếu sản phẩm in với mockup](products.md#pod-mockup-qc)

Nguồn: Training Warehouse phần 4: xác nhận bao bì (training-warehouse-packaging-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4); Training: dịch vụ theo góc nhìn vận hành (training-ops-services-detail, THG_Training@ec170a949623f96229db58d21c6b9ca7791826d4).
