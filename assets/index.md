# assets/ — Mục lục bảo trì (Maintenance Index)

> **Dành cho agent maintain.** Đọc file này trước khi sửa bất kỳ thứ gì trong `assets/` hoặc các trang HTML ở repo root. Mục tiêu: hiểu cấu trúc và quan hệ phụ thuộc mà **không cần đọc toàn bộ code**.
>
> Nguồn spec gốc: `PLAN-MULTIUSER.md` §9 (UI redesign + module hóa). Bản đồ công việc: `DELEGATION-PLAN.md` (Wave 7).

---

## 1. Mục đích & tổng quan

`assets/` là **frontend tĩnh** của AgentChat, được server Go phục vụ trực tiếp (không có bundler, không framework, không build step). Toàn bộ JS là **ES module thuần** nạp qua `<script type="module">`; CSS là các file tách rời nạp theo thứ tự.

- Các **trang HTML nằm ở repo root** (`login.html`, `app.html`) và tham chiếu file trong `assets/` bằng đường dẫn tương đối (ví dụ `<link href="assets/css/tokens.css">`, `<script type="module" src="assets/js/app-entry.js">`).
- `index.html` hiện tại là **chat UI legacy**, sẽ được **thay bằng `app.html`** (§7 của plan). Không phát triển thêm vào `index.html`.
- `assets/` **không chứa** HTML. HTML chỉ ở repo root.
- Ngôn ngữ thị giác theo reference Dribbble **"Iris"**: app-card bo góc lớn nổi trên backdrop mesh mờ; sidebar 288px; composer dạng pill; một accent duy nhất; theme sáng mặc định + dark toggle.

```
assets/
  index.md          # file này
  css/              # design system (WP-15) — ĐÃ CÓ
  js/               # logic module hóa (WP-17..20) — SẼ TẠO
  vendor/           # (tùy chọn) marked/dompurify/katex bản offline — CHƯA TẠO
```

---

## 2. Bảng "File → Trách nhiệm"

### `assets/css/` — trạng thái: ĐÃ CÓ (WP-15)

| File | Trạng thái | Trách nhiệm (1 dòng) |
|---|---|---|
| `assets/css/tokens.css` | **exists** | Design tokens (`--*`): layout, surface, accent, màu trạng thái, shadow, radius, spacing, font; theme qua `:root` + `[data-theme="dark"]`. |
| `assets/css/base.css` | **exists** | Reset + typography + focus ring + scrollbar + backdrop mesh blur; tôn trọng `prefers-reduced-motion`. |
| `assets/css/layout.css` | **exists** | Khung app: `.app-card`, `.sidebar`, `.main`, `.topbar`, `.content`, `.panel-head`, `.grid-stats`, responsive `<900px`/`<600px`. |
| `assets/css/components.css` | **exists** | Component dùng chung: button, card, stat-card, nav-item, search-pill, empty-dashed, profile-row, avatar, badge, table, composer-pill, chip, modal, toast, skeleton, theme-toggle, input/field. |
| `assets/css/chat.css` | **exists** | View chat: `.msg` + `.meta`/`.agent`/`.reply-to`/`.body`, markdown/KaTeX, `.mention`, chỉ báo `.conn`/`.conn.live`. |

### `assets/js/` — trạng thái: SẼ TẠO (WP-17..20)

| File | Trạng thái | Trách nhiệm (1 dòng) | WP |
|---|---|---|---|
| `assets/js/api.js` | **planned** | Fetch wrapper: gắn `credentials`, chuẩn hóa JSON, xử lý **401 → redirect login**. Mọi module khác đi qua đây. | WP-17 |
| `assets/js/store.js` | **planned** | State nhỏ trong bộ nhớ (user hiện tại, room đang mở, cache list) + helper. | WP-18 |
| `assets/js/auth.js` | **planned** | Login / logout / me / change-password; xử lý `must_change_password` → chuyển form đổi pass. | WP-17 |
| `assets/js/chat.js` | **planned** | Load lịch sử, gửi tin, và broadcast realtime qua **SSE** (`/api/stream`). | WP-18 |
| `assets/js/agents.js` | **planned** | Panel **Agents**: bảng agent + số tin + hoạt động cuối + trạng thái. | WP-19 |
| `assets/js/keys.js` | **planned** | Panel **API Keys**: modal tạo key + **banner hiện plaintext 1 lần** (copy) + revoke. | WP-19 |
| `assets/js/admin.js` | **planned** | Trang admin: dashboard stat cards + SVG chart + CRUD user + tab agents/rooms. | WP-20 |
| `assets/js/ui.js` | **planned** | Hạ tầng UI: **router panel**, **theme toggle** (`[data-theme]`), **toast**, render helper, modal helper. | WP-18 |

> **Hook entry (theo WP)**: `login.html` nạp `auth.js`; `app.html` nạp `ui.js` + `store.js` + `chat.js` (+ `agents.js`, `keys.js`, `settings.js`, `admin.js`). Tên file entry cụ thể do WP-17/18/20 chốt; bảng §4 là nguồn tham chiếu cho wiring. Panel `dashboard` (admin) render bởi `admin.js` trong `app.html`.

### `assets/vendor/` — trạng thái: CHƯA TẠO (tùy chọn)

| Path | Trạng thái | Trách nhiệm |
|---|---|---|
| `assets/vendor/` | **optional** | Bản copy offline của `marked` / `dompurify` / `katex` nếu muốn chạy không cần CDN. Chưa có; nếu tạo, cập nhật bảng này. |

---

## 3. Dependency graph JS (mũi tên `a → b` nghĩa là "a phụ thuộc b")

```
api.js        → (không phụ thuộc module nội bộ; là lớp nền)
store.js      → (không phụ thuộc module nội bộ)
ui.js         → (không phụ thuộc nội bộ; hạ tầng router/theme/toast)

auth.js       → api.js
chat.js       → api.js, store.js, ui.js
agents.js     → api.js, ui.js
keys.js       → api.js, ui.js
admin.js      → api.js, ui.js

# Tất cả module trên đều được: pages (HTML) → nạp trực tiếp
login.html    → auth.js
app.html      → ui.js, store.js, chat.js, agents.js, keys.js, settings.js, admin.js
```

Nguyên tắc: **`api.js` là điểm nghẽn duy nhất** cho HTTP — không module nào tự `fetch` trực tiếp để tránh bỏ sót xử lý 401. `ui.js` là nơi duy nhất đụng vào `data-theme`, toast, và chuyển panel.

---

## 4. Quan hệ HTML ↔ JS ↔ CSS

| Page (repo root) | JS modules nạp | CSS panels dùng | Ghi chú |
|---|---|---|---|
| `login.html` | `api.js`, `auth.js` | `tokens.css`, `base.css`, `components.css` (`.card`, `.input`, `.field`, `.btn`, `.badge-danger`) | Card login nổi trên backdrop mesh; **không** dùng `layout.css` app-shell. |
| `app.html` | `ui.js`, `store.js`, `chat.js`, `agents.js`, `keys.js`, `settings.js`, `admin.js` | `tokens.css`, `base.css`, `layout.css`, `components.css`, `chat.css` | App shell đầy đủ: sidebar + topbar + panel router. Panel chat dùng `chat.css`. Panel `dashboard` (chỉ admin) = stat cards + tab Users/Agents/Rooms, render bởi `admin.js`. |
| `index.html` (legacy) | (legacy, không module) | (legacy inline/CSS cũ) | **Sẽ bị thay** bởi `app.html`. Không sửa. |

Thứ tự nạp CSS bắt buộc: `tokens.css` → `base.css` → `layout.css` → `components.css` → `chat.css` (tokens phải đứng trước vì mọi file dùng biến `--*`).

---

## 5. Bảng "UI gọi endpoint nào"

| Page / Panel | Method + Path | Auth |
|---|---|---|
| login.html | `POST /api/auth/login` | cookie (set sau khi login) |
| login.html / app (boot) | `GET /api/auth/me` | cookie |
| settings / logout | `POST /api/auth/logout` | cookie |
| settings | `POST /api/auth/change-password` | cookie |
| keys panel | `GET /api/keys` · `POST /api/keys` | cookie |
| keys panel | `DELETE /api/keys/:id` | cookie |
| agents panel | `GET /api/agents` | cookie |
| agents panel | `GET /api/stats` | cookie |
| chat panel | `GET /api/messages` · `POST /api/messages` | cookie; **agent dùng `X-API-Key: ac_...`** |
| chat panel | `GET /api/rooms` · `DELETE /api/rooms/:room` | cookie |
| chat panel (realtime) | `GET /api/stream` (SSE) | cookie |
| chat panel (health/live) | `GET /api/health` | không cần (public) |
| dashboard panel (admin) | `GET /api/admin/stats` | cookie + `requireAdmin` |
| dashboard panel — tab Users (admin) | `GET /api/admin/users` · `POST /api/admin/users` | cookie + `requireAdmin` |
| dashboard panel — tab Users (admin) | `PATCH /api/admin/users/:id` · `DELETE /api/admin/users/:id` | cookie + `requireAdmin` |
| dashboard panel — tab Agents (admin) | `GET /api/admin/agents` | cookie + `requireAdmin` |
| dashboard panel — tab Rooms (admin) | `GET /api/admin/rooms` | cookie + `requireAdmin` |
| docs link | `GET /guide` | không cần (public) |

**Quy tắc auth:** trình duyệt luôn dùng **session cookie HttpOnly** (không đọc được từ JS → `api.js` phải luôn gửi `credentials: 'same-origin'`). Agent (bot/script) dùng **header `X-API-Key: ac_...`**, không dùng cookie.

---

## 6. Quy ước đặt tên

| Loại | Quy ước | Ví dụ |
|---|---|---|
| CSS token | Prefix `--`, kebab-case, nhóm theo mục đích | `--surface-2`, `--card-radius`, `--s5`, `--danger` |
| CSS class | kebab-case; trạng thái dạng modifier/hậu tố | `.app-card`, `.nav-item`, `.nav-item.active`, `.badge-ok`, `.btn-primary`, `.btn-sm` |
| CSS trạng thái bật | class `.on` cho toggle/chip | `.chip.on` |
| JS module | 1 file = 1 module ES, kebab/lowercase, **không framework, không bundler** | `api.js`, `chat.js`, `admin.js` |
| JS export | `export` hàm/const có tên; không default-export trừ entry | `export async function getMe()` |
| Theme | Thuộc tính trên `<html>` | `document.documentElement.dataset.theme` → `[data-theme="dark"]` |
| DOM hook | `id` cho vùng lớn, `data-*` cho hành vi | `#messages`, `data-panel="agents"`, `data-action="revoke"` |
| Endpoint const | Gom trong `api.js`, không hardcode rải rác | `const EP = { me: '/api/auth/me', ... }` |

---

## 7. Cách thêm 1 panel mới (ví dụ `Billing`)

1. **Tạo module**: `assets/js/<name>.js` (ví dụ `assets/js/billing.js`), `export` hàm `mount(el)` hoặc `render(el)` + `init()`.
2. **Đăng ký trong router** (`assets/js/ui.js`): thêm entry vào bảng panel, ví dụ `{ id: 'billing', label: 'Billing', load: () => import('./billing.js') }`, và xử lý khi `data-panel="billing"` được chọn → mount module vào `.content`.
3. **Thêm nav item** trong `app.html`: `.nav-item` với `data-panel="billing"` + icon SVG + `sidebar-label`/`nav-label`; giữ đúng cấu trúc để responsive `<900px` ẩn label.
4. **Thêm CSS nếu cần**: chỉ khi có class mới; thêm vào `assets/css/components.css` (hoặc file `.css` mới + link trong `app.html` **sau** `components.css`). Dùng lại token, **không hardcode màu**.
5. **Thêm endpoint**: nếu panel gọi API mới, khai báo trong `api.js` và bổ sung dòng vào **bảng §5** của file này.

---

## 8. Cách thêm 1 route admin mới

1. **Backend handler**: viết handler trong `internal/api/admin_handlers.go` (đọc `IdentityFrom(ctx)` để lấy admin).
2. **Đăng ký route** trong `internal/api/handlers.go` (`Register`): bọc trong nhóm `requireAdmin`, ví dụ `mux.Handle("POST /api/admin/foo", requireAdmin(http.HandlerFunc(handleFoo)))`. Route admin **bắt buộc** đi qua `requireAdmin`.
3. **UI gọi API**: trong `assets/js/admin.js`, gọi qua `api.js`; render bằng component sẵn có (`.table`, `.stat-card`, `.badge`, `.btn`).
4. **Cập nhật tài liệu**: thêm dòng route vào **bảng §5** (page admin, auth = cookie + `requireAdmin`).
5. **Không** cho route admin xuất hiện trong nhóm `/api/*` public. Nguyên tắc: chỉ admin đọc chéo dữ liệu user qua `/api/admin/*`.

---

## 9. Checklist khi đổi design token

- [ ] **`assets/css/tokens.css` là nguồn duy nhất** — không định nghĩa lại biến ở file khác.
- [ ] Sửa cả `:root` (light) **và** `[data-theme="dark"]` nếu token thay đổi theo theme.
- [ ] **Không hardcode màu** trong `layout.css`/`components.css`/`chat.css` — luôn dùng `var(--token)` (dùng `color-mix` khi cần độ trong suốt).
- [ ] Kiểm tra tương phản AA ở **cả hai theme** (đặc biệt `--muted`, `--accent` trên nền).
- [ ] Kiểm tra token dạng shape/space (`--r-*`, `--s*`) không phá layout responsive `<900px`/`<600px`.
- [ ] Xác nhận backdrop mesh (`base.css` `body::before`) vẫn hài hòa sau khi đổi `--bg`/`--accent`.
- [ ] Không thêm CDN/lib mới (§9 plan: SVG chart vẽ tay, không thêm lib).

---

## 10. Biến môi trường ảnh hưởng UI

| Env | Ảnh hưởng UI | Ghi chú |
|---|---|---|
| `AUTH_REQUIRED` | Bật/tắt yêu cầu đăng nhập phía server. Khi `=0` (debug), `api.js` không bị chuyển hướng login dù không có session. | Mặc định `1`. Khi tắt, server log `WARNING: auth disabled`. UI **vẫn** nên render như đã đăng nhập (identity admin giả). |
| `TLS` | Khi `=1`, session cookie được set **`Secure`** → UI bắt buộc truy cập qua `https://`, nếu không trình duyệt sẽ không gửi cookie. | Ảnh hưởng cách test login local. |
| `BCRYPT_COST` | Không ảnh hưởng trực tiếp UI (chỉ độ trễ login). | Cost cao → login chậm hơn, cần spinner/disable nút submit. |

---

## 11. Ghi chú thiết kế (reference)

- **Reference:** Dribbble **"Iris"** — app-card bo góc lớn nổi trên **backdrop mesh mờ** (mint → amber, `blur(40px)`).
- **Sidebar:** rộng **288px** (`--sidebar-w`), wordmark + search-pill + nav + empty-dashed + profile-row ghim đáy.
- **Composer:** dạng **pill** (nút `+`, chip `Tools`, model-select, mic, action-btn tròn đổ đầy).
- **Accent:** một màu duy nhất, mờ/tinh tế; màu sắc chủ yếu đến từ backdrop.
- **Theme:** **light mặc định**; **dark toggle** ghi `data-theme="dark"` lên `<html>` (qua `ui.js`), đồng bộ `prefers-color-scheme`.
- **Không dùng** framework CSS/JS ngoài; chart là **SVG vẽ tay**.
