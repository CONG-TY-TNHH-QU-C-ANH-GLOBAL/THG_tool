# Cai THG Chrome Extension noi bo

Ban zip nay dung cho giai doan noi bo truoc khi publish Chrome Web Store.

1. Tai `thg-chrome-extension.zip` tu Browser dashboard.
2. Giai nen file zip ra mot thu muc rieng.
3. Mo Chrome ca nhan dang dang nhap Facebook.
4. Vao `chrome://extensions`.
5. Bat `Developer mode`.
6. Bam `Load unpacked`.
7. Chon dung thu muc vua giai nen co file `manifest.json` nam truc tiep ben trong.
   Chrome se mo cua so chon thu muc nen co the khong hien file `manifest.json`.
   Day la binh thuong. Chi can chon thu muc do va bam `Select Folder`.
   Neu thu muc giai nen co them mot lop con, hay mo vao lop con cho den khi thay
   `manifest.json` bang File Explorer, roi chon dung thu muc do trong Chrome.
8. Bam icon THG Chrome Helper, dan ma ket noi tu dashboard, bam Ket noi.
9. Neu can Browser stream tap trung tren dashboard, tai va chay them THG Local Runtime.
   Runtime se dung ma ket noi moi va mo Chrome profile rieng tren may user de stream ve dashboard.

Luu y bao mat:

- Khong nhap mat khau Facebook vao THG.
- Extension chi gui trang thai tab Facebook va user id tu cookie `c_user`.
- Browser stream nhieu tai khoan duoc xu ly boi THG Local Runtime, khong phai tab Chrome ca nhan.
- Neu Facebook hien checkpoint/CAPTCHA, nguoi dung tu xu ly truc tiep tren Chrome. THG khong bypass checkpoint.
