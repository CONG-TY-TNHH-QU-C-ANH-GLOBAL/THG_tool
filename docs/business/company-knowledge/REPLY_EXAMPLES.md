---
doc_type: business
status: draft
owner: platform
last_reviewed: 2026-10-19
related_pr_or_issue: Add Taobao sourcing to replies
---

# Câu trả lời mẫu để góp ý và đánh giá

> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.

Các đầu vào dưới đây là tình huống tổng hợp, không phải lead production hoặc câu trả lời đã được công ty duyệt. Mô tả ảnh là giả định cho bài kiểm tra, không chứng minh hệ thống đã đọc ảnh. Lời đáp chỉ là một cách viết phù hợp; đánh giá ý nghĩa và căn cứ, không so khớp từng chữ.

## winter-supplier

**Bài đăng giả định:** Hello, I am looking for a reliable supplier for winter products. What can you offer?

**Nhu cầu cần hiểu:** Tìm nhà cung cấp; chưa biết nhóm hàng, không tự chọn một sản phẩm chỉ vì đang mùa đông.

**Đã biết từ đầu vào**

- Khách tìm nguồn hàng mùa đông; dùng tiếng Anh.

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Nhóm hàng hoặc mẫu quan tâm
- Số lượng dự kiến và thị trường nhận hàng

**Lời đáp đề xuất:** THG can help source winter products from China. Which products are you considering, and roughly how many do you need for which destination?

**Vì sao:** Trả lời khả năng tìm nguồn trước, hỏi dữ kiện mở khóa sourcing; không quảng cáo một sản phẩm chưa liên quan.

**Tránh**

- Gắn áo POD ngẫu nhiên
- Tuyên bố có sẵn hàng hoặc báo giá khi chưa có kết quả tra nguồn

Tri thức tham chiếu: [Dịch vụ tìm nguồn và mua hàng](topics/services.md#service-sourcing); [Tình huống: tìm nhà cung cấp hàng mùa đông](topics/situations.md#case-seasonal-supplier); [Chỉ hỏi điều chưa biết](topics/replies.md#reply-known-vs-missing).

## supplier-uae

**Bài đăng giả định:** LOOKING FOR A RELIABLE SUPPLIER/AGENT SHIPPING TO UAE

**Nhu cầu cần hiểu:** Cần nguồn/agent và giao UAE; nơi nhận đã rõ, sản phẩm chưa rõ.

**Đã biết từ đầu vào**

- Đích nhận UAE
- Chưa có sản phẩm, số lượng hoặc kiện hàng

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Loại sản phẩm/mẫu
- Số lượng dự kiến

**Lời đáp đề xuất:** THG can help look for suppliers in China and check shipping options to the UAE. What products and quantities are you looking for?

**Vì sao:** Nhắc đúng UAE và dùng 'check' vì chưa có bằng chứng line phù hợp; không hỏi lại nước nhận.

**Tránh**

- Khẳng định đã có tuyến UAE phù hợp
- Hỏi khách ship đi đâu khi họ đã nêu UAE

Tri thức tham chiếu: [Tình huống: supplier ship UAE](topics/situations.md#case-uae-supplier); [Phân biệt nơi nhà cung cấp, nơi xuất hàng và nơi nhận](topics/shipping.md#shipping-geography); [Dịch vụ tìm nguồn và mua hàng](topics/services.md#service-sourcing).

## plain-hoodie

**Bài đăng giả định:** Cần nhập 300 hoodie trơn không in logo, giao TP.HCM, có nguồn nào không?

**Nhu cầu cần hiểu:** Tìm hoodie trơn, không yêu cầu POD; số lượng và nơi nhận đã có.

**Đã biết từ đầu vào**

- 300 hoodie trơn
- Không in logo
- Giao TP.HCM
- Chưa có yêu cầu vải/định lượng

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Chất vải hoặc mẫu tham chiếu cần để lọc nguồn

**Lời đáp đề xuất:** Với 300 hoodie trơn giao TP.HCM, bên mình có thể tìm nguồn 1688/Taobao theo mẫu bạn cần. Bạn ưu tiên chất vải, định lượng nào, hay có ảnh mẫu để đối chiếu không?

**Vì sao:** Giữ yêu cầu phủ định, hỏi đặc tính còn thiếu; chưa tìm thấy offer nên chưa chèn link/giá.

**Tránh**

- Đưa sản phẩm in POD
- Tự coi các hoodie khác chất vải là cùng mẫu

Tri thức tham chiếu: [Nhận biết yêu cầu không in logo](topics/products.md#product-negative-printing); [Chọn POD hay tìm nguồn theo quyết định task](topics/sourcing.md#sourcing-routing); [Bộ dữ kiện tìm nguồn trong Training](topics/sourcing.md#training-sourcing-discovery).

## cotton-pod-alternative

**Bài đăng giả định:** Cần túi cotton in logo, 200 chiếc, giao Mỹ. Có mẫu không? Giả định catalog hiện chỉ có túi polyester, chưa tra được nguồn sàn đúng yêu cầu.

**Nhu cầu cần hiểu:** POD có yêu cầu vật liệu bắt buộc; catalog không khớp, được tìm nguồn thay thế có ghi rõ.

**Đã biết từ đầu vào**

- Cotton, in logo
- 200 chiếc đi Mỹ
- Polyester không đạt yêu cầu cotton
- Chưa có offer thay thế được xác minh

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Ảnh mẫu hoặc kích thước túi để tìm nguồn phôi phù hợp

**Lời đáp đề xuất:** Catalog hiện chưa có túi cotton đúng yêu cầu của bạn. Bên mình có thể tìm nguồn phôi thay thế từ 1688/Taobao và kiểm tra khả năng in logo; bạn gửi thêm ảnh mẫu hoặc kích thước túi nhé.

**Vì sao:** Nêu thẳng điểm chưa khớp và hướng xử lý; không thay chất liệu hay hứa nguồn phôi chắc chắn nhận in.

**Tránh**

- Gọi polyester là cotton
- Hứa in được trên mọi phôi
- Tạo link nguồn minh họa

Tri thức tham chiếu: [Tình huống: cần túi cotton in logo, catalog chỉ có polyester](topics/situations.md#case-pod-alternative); [Khả năng in phải được xác nhận](topics/products.md#product-customization); [Khớp loại sản phẩm và đặc tính bắt buộc](topics/products.md#product-required-attributes).

## dog-supplement-monthly

**Bài đăng giả định:** Cần nhập viên bổ khớp cho chó từ 1688 về Mỹ khoảng 300 hộp/tháng, báo giúp cả tiền hàng và ship.

**Nhu cầu cần hiểu:** Sourcing và vận chuyển sản phẩm bổ sung cho thú cưng; kiểm tra điều kiện nhận hàng trước khi báo cước.

**Đã biết từ đầu vào**

- Nguồn 1688, đích Mỹ
- 300 hộp mỗi tháng là sản lượng theo kỳ
- Chưa biết công thức, nhãn, đóng gói hoặc cách chia lô

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Link/nhãn và thành phần sản phẩm
- Gửi nguyên lô hay chia đơn, trọng lượng/kích thước đóng gói

**Lời đáp đề xuất:** Bạn gửi link hoặc ảnh nhãn viên bổ khớp nhé, bên mình cần kiểm tra điều kiện nhận loại hàng này trước khi báo tiền hàng và cước đi Mỹ. Với 300 hộp/tháng, bạn định gửi nguyên lô hay chia từng đơn, và đã có cân nặng/kích thước đóng gói chưa?

**Vì sao:** Đáp đúng nhu cầu báo tổng chi phí, ưu tiên điểm chặn và không biến sản lượng tháng thành một kiện giả định.

**Tránh**

- Gán cước 0,4kg của ví dụ cũ cho toàn bộ 300 hộp
- Tự khẳng định tuyến nhận thực phẩm bổ sung
- Tự đưa kết luận pháp lý hoặc giấy phép

Tri thức tham chiếu: [Tình huống: viên bổ khớp cho chó, 300 hộp/tháng đi Mỹ](topics/situations.md#case-dog-supplement); [Loại hàng cần kiểm tra riêng](topics/shipping.md#shipping-sensitive-goods); [Sản lượng tháng, một lô và một kiện](topics/shipping.md#shipping-period-and-parcel); [Các thành phần cần kiểm tra trong báo giá Dropship](topics/commercial.md#dropship-quote-components).

## shipping-monthly-weight

**Bài đăng giả định:** Mình cần ship hoodie sang Mỹ, khoảng 200kg/tháng, báo cước giúp nhé.

**Nhu cầu cần hiểu:** Khách cần vận chuyển hàng đã xác định; chưa yêu cầu tìm mua hoodie.

**Đã biết từ đầu vào**

- Hoodie, đi Mỹ
- Sản lượng 200kg/tháng
- Chưa rõ nơi xuất, mỗi lô/kiện và điểm giao

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Xuất từ VN hay CN, giao kho hay từng địa chỉ
- Cách chia lô/kiện và kích thước

**Lời đáp đề xuất:** Để tính cước hoodie đi Mỹ sát hơn, bạn cho mình biết hàng xuất từ Việt Nam hay Trung Quốc và giao vào kho hay từng địa chỉ nhé. Sản lượng 200kg/tháng sẽ chia thành các lô/kiện khoảng bao nhiêu, kích thước thế nào?

**Vì sao:** Giữ trọng tâm cước vận chuyển và các biến còn thiếu; không ép sang nguồn POD.

**Tránh**

- Hỏi lại loại hàng
- Coi 200kg là cân nặng một kiện
- Chèn áo catalog vào lời tư vấn ship
- Đổi 200 kg/tháng thành 200 ngày vận chuyển hoặc báo thời gian giao khi chưa có line/quote được xác thực.

Tri thức tham chiếu: [Tình huống: ship hoodie đi Mỹ, đã nêu 200kg/tháng](topics/situations.md#case-shipping-known-product); [Phân biệt nơi nhà cung cấp, nơi xuất hàng và nơi nhận](topics/shipping.md#shipping-geography); [Ba bảng giá có mục đích riêng](topics/shipping.md#shipping-three-tables).

## warehouse-stock-plan

**Bài đăng giả định:** Muốn dùng kho Mỹ để giao đơn nhanh hơn, hàng mình vẫn ở nhà cung cấp Trung Quốc. Bắt đầu sao?

**Nhu cầu cần hiểu:** Lập phương án đưa hàng vào kho Mỹ rồi xử lý đơn; chưa có hàng sẵn trong kho.

**Đã biết từ đầu vào**

- Hàng ở nhà cung cấp Trung Quốc
- Mục tiêu kho Mỹ/giao nội địa

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Loại hàng, số SKU và lượng dự kiến nhập kho

**Lời đáp đề xuất:** Hàng của bạn đang ở Trung Quốc nên cần kiểm tra phương án đưa hàng vào kho Mỹ trước, sau đó mới xử lý đơn nội địa. Bạn gửi loại hàng, số SKU và lượng dự kiến nhập kho để bên mình đối chiếu điều kiện tiếp nhận và các khoản phí nhé.

**Vì sao:** Giải thích bước tiếp theo và tách các chặng; không hứa thời gian giao khi chưa có tồn.

**Tránh**

- Dùng cước nội địa như tổng chi phí CN–US
- Hứa kho đã sẵn sàng nhận loại hàng bất kỳ

Tri thức tham chiếu: [Hàng đã ở Mỹ và kế hoạch đưa hàng vào Mỹ](topics/services.md#warehouse-stock-vs-import-plan); [Nhập hàng vào kho Mỹ](topics/services.md#warehouse-inbound-procedure); [Kho Mỹ và fulfillment nội địa](topics/services.md#service-warehouse).

## warehouse-fee-total

**Bài đăng giả định:** Phí pick-pack ghi trong bảng kho Mỹ là đã bao gồm toàn bộ tiền ship chưa?

**Nhu cầu cần hiểu:** Giải thích phạm vi một loại phí, không cần mở đầu bằng form hỏi sản phẩm.

**Đã biết từ đầu vào**

- Khách hỏi phí xử lý kho và tổng phí giao hàng
- Chưa nêu một bảng/báo giá cụ thể

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Dòng phí hoặc báo giá khách đang xem để kiểm tra phạm vi hiện hành

**Lời đáp đề xuất:** Chưa thể xem phí pick-pack là toàn bộ tiền giao hàng, vì phí xử lý kho, bao bì và cước nội địa có thể là các khoản riêng. Bạn gửi dòng phí hoặc báo giá đang xem, mình đối chiếu giúp phần nào đã bao gồm nhé.

**Vì sao:** Giải thích khái niệm từ snapshot có điều kiện và yêu cầu đúng tài liệu cần kiểm tra; không đưa mức phí cũ.

**Tránh**

- Khẳng định mọi gói đều tách phí giống nhau
- Tự tính tổng mà chưa có điều khoản và kiện hàng

Tri thức tham chiếu: [Tách phí kho, bao bì và vận chuyển nội địa](topics/shipping.md#warehouse-fee-components); [Tra phí 3PL và giao nội địa Mỹ](topics/shipping.md#shipping-domestic); [Giá nguồn, phí dịch vụ, cước và giá bán](topics/commercial.md#commercial-price-layers).

## pod-no-design-file

**Bài đăng giả định:** Muốn làm áo POD mà chưa có file thiết kế, THG có thiết kế luôn không?

**Nhu cầu cần hiểu:** Hỏi dịch vụ thiết kế và chuẩn bị file, không phải tìm nhà cung cấp áo trơn.

**Đã biết từ đầu vào**

- Khách chưa có file thiết kế
- Training snapshot phân biệt sản xuất và thiết kế; phạm vi hiện hành chưa được xác nhận

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Loại áo và vị trí in để kiểm tra template phù hợp

**Lời đáp đề xuất:** Mình cần kiểm tra phạm vi hỗ trợ thiết kế hiện tại trước khi xác nhận với bạn. Bạn muốn làm loại áo nào, in ở vị trí nào để bên mình kiểm tra template và yêu cầu file phù hợp nhé?

**Vì sao:** Trả lời đúng phần chưa xác nhận và cho bước tiếp; không biến chính sách design trong bản Training cũ thành lời hứa hiện hành.

**Tránh**

- Hứa thiết kế miễn phí
- Khẳng định chắc chắn từ chối design theo snapshot cũ

Tri thức tham chiếu: [Template, file thiết kế và phạm vi hỗ trợ design](topics/products.md#pod-template-and-design); [Dịch vụ POD](topics/services.md#service-pod); [Giá catalog cần gắn thị trường và phạm vi ship](topics/products.md#catalog-destination-conditions).

## tracking-no-movement

**Bài đăng giả định:** Đơn có tracking rồi mà chưa thấy chạy, có phải đã giao hãng vận chuyển rồi không?

**Nhu cầu cần hiểu:** Kiểm tra tiến độ cụ thể; có mã không đủ chứng minh hãng đã nhận kiện.

**Đã biết từ đầu vào**

- Có tracking
- Khách chưa thấy sự kiện vận chuyển

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Mã đơn/tracking qua kênh riêng

**Lời đáp đề xuất:** Có tracking chưa đủ để xác nhận hãng đã nhận kiện; cần xem lần quét nhận hàng và trạng thái xử lý đơn. Bạn gửi mã đơn hoặc tracking qua tin nhắn riêng để bên mình kiểm tra đúng đơn nhé.

**Vì sao:** Giải đáp thắc mắc trước và xin dữ kiện kiểm tra qua kênh phù hợp; không suy từ label thành giao nhận thực tế.

**Tránh**

- Khẳng định hàng đã lên máy bay hoặc đã giao hãng
- Yêu cầu đăng địa chỉ người nhận công khai

Tri thức tham chiếu: [Trạng thái hệ thống, label và sự kiện giao nhận](topics/commercial.md#warehouse-status-interpretation); [Tiếp nhận trouble có bằng chứng](topics/commercial.md#training-cskh-trouble-intake).

## return-request

**Bài đăng giả định:** Khách muốn trả hàng lại kho Mỹ thì có tự được hoàn tiền luôn không?

**Nhu cầu cần hiểu:** Phân biệt tiếp nhận hàng trả và quyết định hoàn tiền.

**Đã biết từ đầu vào**

- Khách hỏi return và refund
- Chưa có chính sách/đơn cụ thể

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Mã đơn và lý do trả qua kênh riêng để đối chiếu chính sách

**Lời đáp đề xuất:** Nhận hàng trả về kho và hoàn tiền là hai việc cần xử lý riêng, nên chưa thể xác nhận tự động hoàn tiền khi hàng về. Bạn gửi mã đơn và lý do trả qua tin nhắn riêng để bên mình kiểm tra điều kiện áp dụng nhé.

**Vì sao:** Làm rõ điều khách đang nhầm và không tự cấp quyền lợi chưa được duyệt.

**Tránh**

- Hứa hoàn 100%
- Tự nêu số ngày hoàn tiền hoặc địa chỉ kho chưa xác nhận

Tri thức tham chiếu: [Return, refund và reship là ba việc khác nhau](topics/commercial.md#warehouse-return-refund-reship); [Đền bù, hoàn tiền và gửi lại](topics/commercial.md#commercial-compensation).

## europe-supplier-alternative

**Bài đăng giả định:** I'm looking for a European dropshipping supplier for this hand massager. Giả định chưa có offer được xác minh hoặc địa chỉ nhận hàng.

**Nhu cầu cần hiểu:** Tìm nguồn đúng loại sản phẩm tại châu Âu; được nêu nguồn Trung Quốc như một phương án thay thế.

**Đã biết từ đầu vào**

- Hand massager
- Ưu tiên nhà cung cấp châu Âu
- Chưa có offer, giá hay nơi giao

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Có cân nhắc nguồn Trung Quốc không
- Nước nhận hàng nếu chấp nhận kiểm tra phương án

**Lời đáp đề xuất:** THG can check Chinese suppliers for a similar hand massager as an alternative. Would you consider sourcing from China, and which country would the orders ship to?

**Vì sao:** Nguồn khác khu vực được nói rõ ngay; chưa có offer thì không nói đã tìm thấy hay có kho châu Âu.

**Tránh**

- Nhận là nhà cung cấp châu Âu
- Gọi mẫu tương tự là đúng model
- Tạo giá, link hoặc ảnh chứng minh giả

Tri thức tham chiếu: [Khách muốn nguồn châu Âu nhưng tìm được nguồn Trung Quốc](topics/sourcing.md#sourcing-china-alternative); [Gắn nhãn mẫu tương tự](topics/sourcing.md#sourcing-similar); [Phân biệt nơi nhà cung cấp, nơi xuất hàng và nơi nhận](topics/shipping.md#shipping-geography).

## photo-without-model

**Bài đăng giả định:** Cần tìm mẫu này, 100 cái đi Mỹ.

**Ảnh giả định:** Ảnh giả định có hình một máy massage tay, không nhìn rõ thương hiệu hoặc mã; chưa có kiểm thử vision thực tế.

**Nhu cầu cần hiểu:** Tìm sản phẩm dựa trên ảnh; dùng đặc tính quan sát được, không đoán model.

**Đã biết từ đầu vào**

- 100 cái, đi Mỹ
- Trong fixture ảnh, có hình dáng máy massage tay; chưa rõ model

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Link hoặc ảnh rõ mã nếu khách yêu cầu đúng model

**Lời đáp đề xuất:** Bạn cần đúng model trong ảnh hay có thể dùng mẫu máy massage tay tương tự? Nếu cần đúng model, bạn gửi thêm link hoặc ảnh rõ mã để bên mình đối chiếu nguồn cho 100 cái đi Mỹ nhé.

**Vì sao:** Chỉ dùng mô tả ảnh đã được fixture cung cấp; làm rõ độ khớp cần thiết, không hỏi lại số lượng/đích.

**Tránh**

- Gửi lời đáp này khi hệ thống chưa thực sự đọc được ảnh
- Đoán thương hiệu, pin hay thông số kỹ thuật từ hình dáng
- Khẳng định đã kiểm tra ảnh hoặc đúng model nếu model chưa nhận ảnh gốc.

Tri thức tham chiếu: [Tình huống: cần mẫu này kèm ảnh](topics/situations.md#case-photo-only-product); [Hiểu cùng văn bản và ảnh gốc](topics/replies.md#reply-multimodal); [Gắn nhãn mẫu tương tự](topics/sourcing.md#sourcing-similar).

## text-photo-conflict

**Bài đăng giả định:** Tìm túi cotton theo ảnh, 200 cái đi Mỹ.

**Ảnh giả định:** Ảnh giả định thấy nhãn polyester rõ ràng; đây là thông tin fixture, không phải kết quả OCR thực tế.

**Nhu cầu cần hiểu:** Làm rõ mâu thuẫn vật liệu trước khi sourcing.

**Đã biết từ đầu vào**

- Text yêu cầu cotton
- Ảnh fixture có nhãn polyester
- 200 cái đi Mỹ

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Cotton là bắt buộc hay chỉ cần kiểu dáng như ảnh

**Lời đáp đề xuất:** Bạn muốn túi cotton có kiểu dáng như ảnh, đúng không? Nhãn trong ảnh ghi polyester, nên mình muốn xác nhận chất liệu bạn cần trước khi tìm nguồn cho 200 chiếc đi Mỹ.

**Vì sao:** Nêu mâu thuẫn cụ thể và hỏi đúng một điểm ảnh hưởng quyết định; chỉ dùng khi nhãn thật sự đọc rõ.

**Tránh**

- Tự ưu tiên ảnh rồi bỏ yêu cầu cotton
- Khẳng định nhãn khi ảnh mờ hoặc chưa đọc được ảnh

Tri thức tham chiếu: [Chữ và ảnh mâu thuẫn](topics/replies.md#reply-conflicting-evidence); [Khớp loại sản phẩm và đặc tính bắt buộc](topics/products.md#product-required-attributes).

## hiring-virtual-assistant

**Bài đăng giả định:** Looking for a virtual assistant to manage my Amazon listings and reimbursements.

**Nhu cầu cần hiểu:** Tuyển nhân sự hỗ trợ vận hành tài khoản; không phải nhu cầu POD/sourcing/vận chuyển/kho trong task.

**Đã biết từ đầu vào**

- Tuyển VA
- Có từ Amazon không đồng nghĩa cần fulfillment

**Lời đáp đề xuất:** Không tạo lời chào dịch vụ/sourcing tự động; bài ngoài phạm vi nhu cầu đang xử lý.

**Vì sao:** Ghi trạng thái ngoài phạm vi cho sale thay vì tạo câu chào dịch vụ lệch nhu cầu.

**Tránh**

- Ép mọi bài chứa Amazon thành lead sourcing
- Hỏi model/số lượng/nơi nhận cho bài tuyển VA

Tri thức tham chiếu: [Bài không phải nhu cầu THG trong task này](topics/customers.md#customer-out-of-scope); [Phân biệt người cần dịch vụ và người đang chào bán](topics/customers.md#customer-demand-vs-offer).

## source-lookup-failure

**Bài đăng giả định:** Cần tìm máy massage tay 50 cái đi UAE. Giả định lần tra nguồn gặp timeout, chưa có offer hợp lệ.

**Nhu cầu cần hiểu:** Có nhu cầu rõ nhưng provider lỗi; vẫn hỗ trợ bằng lời đáp có căn cứ và trạng thái nội bộ chính xác.

**Đã biết từ đầu vào**

- Máy massage tay, 50 cái, đi UAE
- Tra cứu timeout không phải kết luận không có nguồn

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Link hoặc mã nếu cần đúng model

**Lời đáp đề xuất:** Bên mình đang kiểm tra nguồn máy massage tay cho 50 cái đi UAE, hiện chưa có giá để xác nhận. Nếu bạn cần đúng model, gửi thêm link hoặc mã sản phẩm để bên mình đối chiếu nhé.

**Vì sao:** Chỉ phù hợp khi đã có quy trình kiểm tra lại; nội bộ cần ghi lỗi tra nguồn/timeout và người xử lý, không báo 'không có sản phẩm' hay lộ tên lỗi cho khách.

**Tránh**

- Hỏi lại loại hàng, số lượng và nơi nhận
- Bịa offer để lấp chỗ trống
- Nói đang kiểm tra nếu không có bước xử lý tiếp

Tri thức tham chiếu: [Không tìm thấy nguồn và lỗi tra cứu](topics/sourcing.md#sourcing-lookup-failure); [Bản nháp cho sale và trạng thái xử lý](topics/replies.md#reply-draft-and-status); [Chỉ hỏi điều chưa biết](topics/replies.md#reply-known-vs-missing).

## basecost-inclusions-question

**Bài đăng giả định:** Basecost trong báo giá này là tổng tiền mình phải trả rồi hay còn phí nào nữa?

**Nhu cầu cần hiểu:** Hỏi phạm vi giá cuối; từ basecost một mình chưa đủ xác định các khoản đã gồm.

**Đã biết từ đầu vào**

- Khách đang xem một báo giá nhưng chưa cung cấp dòng phí hoặc điều kiện.

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Dòng báo giá/điều kiện đã bao gồm để đối chiếu

**Lời đáp đề xuất:** Mình cần xem phần đã bao gồm trong báo giá để xác nhận còn khoản nào phải trả. Bạn gửi giúp dòng giá và điều kiện đi kèm nhé, bên mình sẽ làm rõ tiền hàng, phí dịch vụ và cước trước khi chốt.

**Vì sao:** Đáp trực tiếp chỗ chưa thể xác nhận và xin đúng bằng chứng; không chọn ngẫu nhiên một định nghĩa basecost trong các snapshot khác nhau.

**Tránh**

- Khẳng định mọi basecost đều all-in
- Tự cộng thêm phí đã nằm trong giá

Tri thức tham chiếu: [Các thành phần cần kiểm tra trong báo giá Dropship](topics/commercial.md#dropship-quote-components); [Giá nguồn, phí dịch vụ, cước và giá bán](topics/commercial.md#commercial-price-layers); [Khách thấy giá cao: so cùng phạm vi trước khi đề xuất](topics/commercial.md#commercial-quote-comparison).

## supplier-not-dispatched

**Bài đăng giả định:** Đơn hoodie của mình shop Trung Quốc chưa gửi mà đã qua ngày dự kiến, giờ xử lý sao? Hàng sẽ đi Mỹ.

**Nhu cầu cần hiểu:** Xử lý chậm ở nhà cung cấp trước khi về kho, chưa phải chậm vận chuyển quốc tế.

**Đã biết từ đầu vào**

- Hoodie sẽ đi Mỹ
- Khách nói supplier chưa gửi và quá ngày dự kiến

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Mã đơn qua kênh riêng để kiểm tra với bộ phận xử lý

**Lời đáp đề xuất:** Theo thông tin bạn gửi, đơn đang chậm ở khâu nhà cung cấp chưa gửi hàng. Bạn nhắn riêng mã đơn để bên mình kiểm tra nguyên nhân và lịch gửi mới; nếu cần đổi nguồn, bên mình sẽ trao đổi lại mẫu, giá và thời gian trước khi bạn quyết định.

**Vì sao:** Giữ đúng chặng bị chậm, xin đúng dữ kiện và giữ quyền quyết định thay nguồn của khách; không đưa ngày giao giả định.

**Tránh**

- Quy lỗi hải quan hoặc hãng quốc tế khi hàng chưa được gửi
- Tự hủy/đổi supplier
- Hứa hoàn tiền hoặc deadline chưa duyệt

Tri thức tham chiếu: [Hàng chậm từ supplier về kho: xác minh đúng chặng](topics/commercial.md#commercial-supplier-delay); [Tiếp nhận trouble có bằng chứng](topics/commercial.md#training-cskh-trouble-intake); [Rõ ràng và xác nhận khi thông tin thay đổi](topics/company.md#company-confirmation-culture).

## basecost-inclusions-explicit

**Bài đăng giả định:** Báo giá đã được duyệt, còn hiệu lực và đúng đơn của mình ghi rõ đã gồm tiền hàng, mua hộ và cước quốc tế; phí lưu kho không áp dụng. Vậy có cộng thêm phí mua hộ nữa không?

**Nhu cầu cần hiểu:** Giải thích khoản đã nằm trong báo giá cụ thể, không yêu cầu khách gửi lại thông tin đã có.

**Đã biết từ đầu vào**

- Đầu vào giả định đã xác nhận đúng báo giá và phạm vi
- Phí mua hộ đã bao gồm; lưu kho không áp dụng

**Lời đáp đề xuất:** Theo báo giá bạn nêu, phí mua hộ đã được tính trong giá nên không cộng thêm lần nữa. Phí lưu kho không áp dụng cho đơn này.

**Vì sao:** Trả lời thẳng theo điều kiện đầu vào đã xác nhận, không suy sang mọi gói và không hỏi lại dữ kiện có đủ.

**Tránh**

- Hỏi lại toàn bộ báo giá khi đã được cung cấp đủ
- Tự thêm phí mua hộ theo phần trăm trong tài liệu cũ
- Khẳng định không thể phát sinh bất kỳ khoản nào ngoài phạm vi đã xác nhận

Tri thức tham chiếu: [Cách xử lý tạm thời khi basecost chưa rõ phạm vi](topics/commercial.md#commercial-basecost-interpretation); [Giá nguồn, phí dịch vụ, cước và giá bán](topics/commercial.md#commercial-price-layers).

## basecost-missing-fee

**Bài đăng giả định:** Mình đã có giá hàng và cước quốc tế nhưng phí mua hộ chưa được xác nhận. Cộng hai khoản đó rồi báo tổng cuối được chưa?

**Nhu cầu cần hiểu:** Phân biệt tổng các khoản đã biết với tổng thanh toán cuối.

**Đã biết từ đầu vào**

- Có giá hàng và cước quốc tế
- Phí mua hộ còn chưa xác nhận

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Phí mua hộ có áp dụng và đã bao gồm ở khoản nào chưa

**Lời đáp đề xuất:** Chưa nên gọi đó là tổng tiền cuối, vì phí mua hộ còn chưa rõ. Mình cần kiểm tra khoản này có áp dụng và đã nằm trong giá hàng hay chưa để tránh báo thiếu hoặc cộng hai lần.

**Vì sao:** Nêu đúng điểm chặn; không xem phí chưa có số là bằng 0, cũng không tự áp một tỷ lệ.

**Tránh**

- Gọi phần cộng đã biết là all-in
- Tự đặt phí bằng 0 hoặc lấy phần trăm Training cũ

Tri thức tham chiếu: [Cách xử lý tạm thời khi basecost chưa rõ phạm vi](topics/commercial.md#commercial-basecost-interpretation); [Các thành phần cần kiểm tra trong báo giá Dropship](topics/commercial.md#dropship-quote-components).

## tracking-activation-no-sla

**Bài đăng giả định:** Bên bạn có bảo đảm đơn TikTok active tracking trong 24 giờ từ lúc mình gửi đơn không? Chưa có line hoặc SLA hiện hành trong ngữ cảnh.

**Nhu cầu cần hiểu:** Hỏi cam kết dịch vụ; cần giữ riêng thời gian tạo mã và điều kiện tracking nền tảng.

**Đã biết từ đầu vào**

- Khách nêu TikTok và mong muốn 24 giờ
- Chưa có SLA hiện hành hoặc line xác nhận

**Chỉ hỏi phần còn thiếu cần cho bước tiếp**

- Line và điều kiện xử lý dự kiến để đối chiếu SLA

**Lời đáp đề xuất:** Mình chưa có đủ căn cứ để bảo đảm mốc 24 giờ cho đơn của bạn. Bên mình cần đối chiếu line và điều kiện xử lý, vì tạo mã tracking và có cập nhật từ hãng là các mốc khác nhau.

**Vì sao:** Trả lời trực tiếp yêu cầu bảo đảm và giải thích lý do có ích; không đổi sang lời hứa 48 giờ hoặc nói đã đạt điều kiện sàn.

**Tránh**

- Hứa 24/48 giờ từ snapshot
- Xem tạo mã là đã được hãng nhận hoặc sàn công nhận

Tri thức tham chiếu: [Cách xử lý tạm thời khi chưa có SLA active tracking](topics/shipping.md#shipping-tracking-milestones); [Chính sách và thời gian phải theo đúng tuyến](topics/shipping.md#shipping-policy-scope).
