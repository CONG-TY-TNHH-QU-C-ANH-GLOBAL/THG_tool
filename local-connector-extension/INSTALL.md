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
9. Mo `facebook.com` tren Chrome do va bam Dong bo neu dashboard chua nhan tin hieu.

Luu y bao mat:

- Khong nhap mat khau Facebook vao THG.
- Extension chi gui trang thai tab Facebook, user id tu cookie `c_user`, va anh tab dang active khi workspace dang chay.
- Neu Facebook hien checkpoint/CAPTCHA, nguoi dung tu xu ly truc tiep tren Chrome. THG khong bypass checkpoint.
