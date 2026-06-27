# Reel Tab — Frontend UX Spec (chốt trước khi code)

**Track:** KnowledgeOS / Content Studio (frontend). **PR type:** behavior-adding (additive).
**Backend:** đã xong (`/api/reels`, `/approve`, `/publish`, webhook). Frontend chỉ là transport + UI.

> Nguyên tắc bắt buộc: **mô phỏng y hệt design language sẵn có, KHÔNG đè/sửa view nào đang chạy.**
> Mọi thay đổi là *thêm mới*: 1 tab, 1 case trong `renderView()`, 1 nav label, các key i18n, và
> các file mới trong `components/views`, `components/reels`, `services`. Không file cũ nào đổi hành vi.

---

## 0. Design tokens tái dùng (trích từ PostingView / CommentingView / MissionsView)

| Thành phần | Class / style chuẩn (giữ NGUYÊN) |
|---|---|
| Header eyebrow | `<div className="eyebrow"><span className="dot" />NHÃN UPPERCASE</div>` |
| Tiêu đề | `<h2 style={{ fontSize: 28, marginTop: 8 }}>` |
| Subtitle | `<p style={{ color:'var(--text-mute)', fontSize:13.5, marginTop:6, maxWidth:640 }}>` |
| Nút phụ | `btn btn-ghost btn-sm` (RefreshCw + "Làm mới") |
| Nút chính | `btn btn-primary btn-sm` (Plus + "Tạo Reel") |
| Filter | `filter-pill` + `is-active` |
| Card | `className="card"`, grid `repeat(auto-fill, minmax(320px,1fr))`, gap 16 |
| Chip mono | `<span className="mono" …>` |
| Status tag | `tag tag-ok` (xanh) · `tag tag-warm` (cam) · `tag tag-cold` (xám) · `tag tag-hot` (đỏ) |
| Form label | `label` 12px `var(--text-faint)`, dấu `*` = `var(--hot)` |
| Input/Select | `className="input"` |
| Lỗi | `banner banner-hot` hoặc khối `AlertCircle` nền `rgba(220,40,40,.08)` |
| Xác nhận nguy hiểm | `window.confirm(...)` (giống "Xoá tất cả" của PostingView) |

→ Reel **không** giới thiệu component-style mới. Chỉ ghép các mảnh trên.

---

## 1. Vị trí tab (đặt cùng cấp Posting/Commenting)

`FacebookWorkspaceApp.tsx` — thêm **additive**:
- `Tab` type: thêm `| 'reel'`.
- `ADMIN_TABS`: chèn `{ id: 'reel', Icon: Clapperboard }` **ngay sau** `commenting` (vẫn nhóm CHÍNH).
  - Không thêm vào `STAFF_TABS` (reel = hành động tiêu tiền → admin, giống posting/commenting).
- `navLabel` map: `reel: t.nav.reel`.
- `renderView()`: `case 'reel': return <ReelView orgId={orgId} isAdmin={isAdmin} />;`
- lazy import: `const ReelView = lazy(() => import('./views/ReelView'));`

Sidebar sau update:
```
CHÍNH
  Leads · Chat AI · Kết nối Facebook · Inbox · Posting · Commenting · Reel   ← MỚI
TỰ ĐỘNG HÓA
  Nhiệm vụ · Leaderboard
HỆ THỐNG
  Settings
```

---

## 2. Màn chính ReelView — header copy y hệt MissionsView

```
● REEL STUDIO                                          [↻ Làm mới]  [+ Tạo Reel]
Reel Studio
AI viết kịch bản → render video ngắn → đăng lên trang qua kết nối Facebook có sẵn.

[Tất cả] [Nháp] [Đang render] [Sẵn sàng] [Đã đăng] [Lỗi]        … [🗑 Xoá tất cả]
────────────────────────────────────────────────────────────────────────────────
( khi bấm "+ Tạo Reel": hiện inline card CreateReelForm — giống MissionsView )
( danh sách: grid card auto-fill minmax(320px) )
```
- Header = đúng block `<header> … eyebrow + h2(28) + subtitle … nút phải`.
- Hàng filter = đúng `filter-pill` + "Làm mới" + "Xoá tất cả" (danger ghost, admin).
- Empty state = block `empty` (eyebrow + h3 + p) y như PostingView.

---

## 3. CreateReelForm (mô phỏng CreateMissionForm)

Chỉ thu các field **backend create nhận** (`POST /api/reels`): brief + keywords + duration.
Account + target_url thu ở **bước Đăng** (vì publish mới cần) → đúng contract, không phình form.

```
Ý tưởng / phong cách *           [textarea, ≥20 ký tự]            ← giống "prompt"
  vd: "Kể chuyện seller ship chậm mất khách, kết bằng giải pháp fulfill US 3-5 ngày"

Từ khoá (phẩy)                   [input]  vd: BUNG, fulfill US, 3-5 ngày

▸ Nâng cao                        (ChevronRight/Down — y như form Missions)
   Thời lượng                     [range 15–60s, step 5, default 25]  "≈ 6–8 shot 4–5s"

                                                  [Huỷ]   [⚡ Tạo & sinh kịch bản]
```
- `canSubmit` = brief.trim().length ≥ 20. Nút submit `btn btn-primary` + icon `Send`.
- Sau tạo: reel xuất hiện ở list với status **Chờ duyệt**, card tự mở panel kịch bản.

---

## 4. Reel card (mô phỏng card PostingView, giàu hơn vì có vòng đời)

```
┌──────────────────────────────────────────────────────────┐
│ reel #13   2026-06-25                        [Chờ duyệt]  │  mono chip + date + tag
│ Fulfill US 3-5 ngày — chốt đơn không lo trễ #thgfulfill   │  caption (content)
│ ──────────────────────────────────────────────────────── │
│ KỊCH BẢN v1 · 5 shot · 25s · $0.00                        │  tóm tắt script + cost
│ 1 talking_head · 2 product · 3 broll · 4 product · 5 broll│
│ ──────────────────────────────────────────────────────── │
│ ( vùng động theo state — xem §5 )                         │
│ ──────────────────────────────────────────────────────── │
│ [Xem/Sửa kịch bản]            [Duyệt → Render]            │  footer actions
└──────────────────────────────────────────────────────────┘
```
Footer dùng `borderTop: 1px solid var(--line)` + nút `btn btn-ghost btn-sm` / `btn btn-primary btn-sm` y như PostingView.

### Map status → tag (tái dùng class tag có sẵn)
| status backend | Nhãn VI | tag |
|---|---|---|
| draft / scripting | Nháp | `tag tag-cold` |
| script_ready | Chờ duyệt | `tag tag-cold` |
| rendering | Đang render | `tag tag-warm` |
| render_stuck | Render kẹt | `tag tag-hot` |
| assembled | Sẵn sàng đăng | `tag tag-warm` |
| posting | Đang đăng | `tag tag-warm` |
| published | Đã đăng | `tag tag-ok` |
| failed | Lỗi | `tag tag-hot` |

---

## 5. Vùng động + hành động theo state (vòng đời)

| State | Vùng động giữa card | Nút footer |
|---|---|---|
| **script_ready** | (ẩn) hoặc panel kịch bản khi mở | `[Xem/Sửa kịch bản]` · **`[Duyệt → Render]`** (primary) |
| **rendering** | thanh tiến độ `███░░ 3/5 · $0.18` (poll 2.5s) | *(không có nút Huỷ — money invariant)* `[Xem tiến độ]` |
| **render_stuck** | banner cảnh báo "render kẹt, cần người xử lý" | `[Báo xử lý]` (ghost) — KHÔNG tự render lại |
| **assembled** | dòng "Video sẵn sàng: renders/reel-13/final.mp4" | `[Xem trước]` · **`[Đăng]`** (primary) |
| **posting/published** | link tới outbound (`outbound_id`) | `[Đã đăng ✓]` (disabled ok) |
| **failed** | lý do lỗi (banner-hot) | `[Thử lại shot lỗi]` |

### 5a. Nút "Duyệt → Render" = spend gate (BẮT BUỘC confirm)
Dùng `window.confirm` (đúng pattern "Xoá tất cả"):
```
"Render sẽ TIÊU CHI PHÍ và KHÔNG THỂ HUỶ khi đã bắt đầu.
 Reel #13 · ~5 shot. Tiếp tục?"
```
OK → `POST /api/reels/:id/approve` → card chuyển **Đang render**, bắt đầu poll.
Bấm lại khi đang render = idempotent (backend trả state hiện tại, không tạo shot mới) → UI không nhân đôi.

### 5b. Panel "Xem/Sửa kịch bản" (mở trong card, không modal)
- Hiện `dialogue` + bảng shot (scene · kind · prompt · dur · voiceover) — read-only.
- 1 ô `input` **Caption** sửa được + nút `[Lưu (version +1)]` → `PATCH /api/reels/:id/script`.
- `verify_flags` (nếu có) hiện chip cảnh báo "cần xác minh: …" (đúng tinh thần grounding — không bịa số liệu).

### 5c. Bước "Đăng" (assembled → post_reel qua outbound spine)
Mở khối nhỏ trong card (giống advanced của form):
```
Account *   [select — TÁI DÙNG đúng danh sách activeAccounts của CreateMissionForm]
Target URL  [input url]  vd: https://facebook.com/me   (mỗi reel 1 đích để tránh trùng)
                                                   [Huỷ]  [📤 Đăng reel]
```
→ `POST /api/reels/:id/publish {account_id, target_url}`.
- `allowed=true` → toast "Đã đưa vào hàng đợi đăng", card → **Đang đăng**, hiện `outbound_id`.
- `allowed=false, reason=duplicate_outbound_target_race` → banner-warm:
  "Đích này vừa được đăng trong 24h (dedup của outbound). Đổi target/account rồi thử lại."
  → đây là **guard thật của spine**, không phải lỗi; UI giải thích đúng.

---

## 6. Polling (không có websocket cho reel)
- `ReelView` chạy 1 interval ~2.5s **chỉ khi** có ≥1 reel ở `rendering`/`posting`; dừng khi không còn.
- Mỗi tick: `GET /api/reels` (list) → cập nhật status/shots_done/cost. (Đủ cho demo; tối ưu sau.)
- Tôn trọng `render_stuck`: không tự gọi lại render — chỉ đổi badge.

---

## 7. i18n (additive — chỉ THÊM key, không sửa key cũ)

`strings.ts`:
- `nav.reel`: VI "Reel" · EN "Reel".
- `views.reelTitle`/`reelSub` + section mới `reelView` (eyebrow, filter*, create*, status*, action*, confirmApprove, publish*, errors). Đủ cặp VI/EN.

---

## 8. File mới (mỗi file ≤ 200 dòng — đúng guardrail)

| File | Vai trò |
|---|---|
| `services/reelsService.ts` | `listReels`, `getReel`, `createReel`, `updateScript`, `approveReel`, `publishReel` (pattern `outboxService`) + types |
| `components/views/ReelView.tsx` | view chính: header + filter + list + poll |
| `components/reels/CreateReelForm.tsx` | form tạo (mô phỏng CreateMissionForm) |
| `components/reels/ReelCard.tsx` | 1 card + vùng động theo state + footer actions |
| `components/reels/ReelScriptPanel.tsx` | panel xem/sửa kịch bản + caption |
| `components/reels/ReelProgress.tsx` | thanh tiến độ render + cost |
| `components/reels/PublishReelForm.tsx` | khối account + target khi đăng |

Sửa **additive** (không đè hành vi): `FacebookWorkspaceApp.tsx` (tab/case/label), `i18n/strings.ts` (key mới).

---

## 9. Checklist nghiệm thu UI (đối chiếu mentor)
- [ ] Header reel **trùng khít** block header MissionsView (eyebrow chấm, h2 28px, subtitle muted, nút phải).
- [ ] Filter pills, card grid, tag màu, form `input/label`, error banner = đúng class sẵn có.
- [ ] Spend gate có `window.confirm`; reel đang render KHÔNG có nút huỷ.
- [ ] `duplicate_outbound_target_race` hiển thị như guard, không như lỗi đỏ.
- [ ] Không file cũ nào đổi hành vi; `npm --prefix frontend run build` xanh.
- [ ] Mỗi file mới ≤ 200 dòng.
