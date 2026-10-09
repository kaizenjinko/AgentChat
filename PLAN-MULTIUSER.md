# Plan: Multi-user + Auth + Admin + UI Redesign — AgentChat (v2, chốt)

> Trạng thái: **PLAN ONLY (v2)**. Chưa có dòng code nào được thêm.
> v2 = v1 + quyết định đã chốt (§0) + gia cố giảm thiểu rủi ro (§10).

---

## 0. Quyết định đã chốt

| # | Vấn đề | Quyết định |
|---|---|---|
| 1 | Xóa user | **Cascade** (FK `ON DELETE CASCADE`, có `PRAGMA foreign_keys=ON`) — xóa kèm keys, sessions, agents, **và messages** của user |
| 2 | Đăng ký | **Không self-signup.** Chỉ admin tạo account |
| 3 | `AUTH_REQUIRED` | **Mặc định bật (=1).** Env chỉ để tạm tắt khi debug, mặc định luôn là bật |
| 4 | UI | **Tách file** trong `assets/`, code module hóa; thêm `assets/index.md` làm mục lục cho agent maintain |
| 5 | Password hash | **bcrypt** (`golang.org/x/crypto/bcrypt`, cost 12) |

---

## 1. Yêu cầu (requirements → acceptance)

| # | Yêu cầu | Tiêu chí nghiệm thu |
|---|---|---|
| R1 | Superuser `admin` / `123456` | Seed admin role=admin, cờ `must_change_password=1`; login OK; bị ép đổi pass lần đầu |
| R2 | Login username + password | `POST /api/auth/login` → session cookie HttpOnly; sai → 401 |
| R3 | User tự tạo `x-api-key` cho agent | `POST /api/keys` sinh key; agent dùng `X-API-Key` để POST/GET messages |
| R4 | Agent join đúng room của user (chống IDOR) | Mọi query/message scope theo `user_id`; truy cập chéo → 404 |
| R5 | User panel: đổi pass, list agent + số tin, quản lý hoạt động agent | `/app` có tab Agents / API Keys / Settings |
| R6 | Admin dashboard + quản trị user (create/delete/deactivate/change password) | `/admin`: dashboard + CRUD user + thống kê hệ thống |
| R7 | UI hiện đại (Dribbble ref) | Theme sáng + dark toggle, sidebar + dashboard cards + SVG charts |

**Ngoài phạm vi:** self-signup, OAuth/2FA, email, audit log xuất file.

---

## 2. Đánh giá hiện trạng (GitNexus + đọc code)

Repo `AgentChat`: **230 nodes, 417 edges, 7 clusters, 18 flows**. **Không có code auth nào.**

Luồng hiện tại: `handleMessages → handlePostMessage → filter.ParseMentions → Store.Insert → Store.GetByID → Hub.Broadcast`; `handleRooms → Store.ListRooms` / `Store.Query`.

Điểm khiến **không có scope theo user** (gốc IDOR):
- `Message` (`store.go:17`) không có `user_id`; bảng `messages` (`store.go:68`) không có cột `user_id`.
- `Store.Query` (`store.go:162`) build WHERE từ room/agent/mention → trả **mọi room**.
- `Store.ListRooms` (`store.go:121`) `GROUP BY room` không lọc chủ.
- `Store.Insert` (`store.go:91`) ghi không gắn chủ.
- `Hub` (`hub.go`) fan-out theo `room` string; `Client{Room, Ch}`.
- Route (`handlers.go:32-40`) public hoàn toàn, CORS `*`.

### Impact analysis (GitNexus, repo=AgentChat)

| Symbol | Risk | Direct callers | Ghi chú |
|---|---|---|---|
| `Store.Query` | LOW | `ListRooms`, `handleMessages` (d1); `handleRooms` (d2) | 3 call site cần thêm `userID` |
| `Store.Insert` | LOW | `handlePostMessage` (d1) | 1 call site |
| `handleRooms` / `handleMessages` | LOW | 0 | handler lá |
| `Broadcast` (hub) | LOW | `handlePostMessage` | 1 caller |

**Kết luận:** blast radius call-graph thấp → feature chủ yếu **mở rộng** struct/handler. Rủi ro thật là **đúng đắn bảo mật** (sót đường truy vấn), không phải regression.

---

## 3. Thiết kế dữ liệu

### 3.1 PRAGMA bắt buộc
Thêm `_pragma=foreign_keys(1)` vào DSN (`store.go:44`) **và** `PRAGMA foreign_keys=ON` trong `init()` — cần cho cascade §0.1.

### 3.2 Bảng mới

```sql
CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,                    -- bcrypt cost 12
  role          TEXT NOT NULL DEFAULT 'user',     -- 'admin' | 'user'
  status        TEXT NOT NULL DEFAULT 'active',   -- 'active' | 'disabled'
  must_change_password INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT DEFAULT (datetime('now')),
  last_login_at TEXT
);

CREATE TABLE IF NOT EXISTS api_keys (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  key_hash     TEXT UNIQUE NOT NULL,              -- sha256(key)
  prefix       TEXT NOT NULL,                     -- hiển thị "ac_ab12cd34…"
  agent_name   TEXT,
  status       TEXT NOT NULL DEFAULT 'active',
  created_at   TEXT DEFAULT (datetime('now')),
  last_used_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_keys_user ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,                    -- random 32 bytes hex
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT DEFAULT (datetime('now')),
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS agents (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  created_at TEXT DEFAULT (datetime('now')),
  UNIQUE(user_id, name)
);
```

### 3.3 Bảng `messages`
- Thêm `user_id INTEGER NOT NULL DEFAULT 0` + index `idx_user_room_id ON messages(user_id, room, id)`.
- Migration idempotent (`PRAGMA table_info` → `ALTER TABLE ADD COLUMN`), backfill `UPDATE messages SET user_id=1 WHERE user_id=0` (dữ liệu cũ về admin).
- **Cascade delete**: vì `messages` không dùng FK (giữ schema phẳng), việc xóa message của user được thực hiện **tường minh trong 1 transaction** `DELETE FROM messages WHERE user_id=?` (xem §4.4) — không phụ thuộc FK để đảm bảo tính đúng khi xóa user.

---

## 4. Xác thực & bảo mật (đã gia cố)

### 4.1 Hashing
- **bcrypt** (`x/crypto/bcrypt`), **cost 12** (cân bằng bảo mật/CPU; config qua env `BCRYPT_COST`, mặc định 12, min 10).
- **Chống username enumeration / timing:** khi login, nếu user không tồn tại → vẫn chạy `CompareHashAndPassword` với một **dummy hash** đã tính sẵn → thời gian phản hồi tương đương; luôn trả cùng thông điệp `invalid credentials`.
- **Password policy tối thiểu:** ≥ 8 ký tự; cảnh báo (không chặn) nếu trùng `123456`.
- **Constant-time:** mọi so sánh token/key dùng `crypto/subtle.ConstantTimeCompare` hoặc so hash bằng `bytes.Equal` trên **hash** (không so plaintext).

### 4.2 API key
- Sinh `ac_` + 32 bytes `crypto/rand`; **chỉ trả plaintext 1 lần**; lưu `sha256(key)` + `prefix`.
- Middleware verify: `sha256(provided)` tra bảng (index unique) → constant-time không cần thiết vì so hash, nhưng vẫn dùng oracle an toàn (không phân biệt "key sai" vs "key revoked" ở response ngoài — cùng 401).

### 4.3 Session
- `id` random 32 bytes hex; lưu DB; hết hạn **7 ngày**; sliding renewal tùy chọn (bỏ ở v1).
- Cookie: `HttpOnly; SameSite=Lax; Path=/`; `Secure` khi `TLS=1` (env).
- Logout: xóa row session + clear cookie. Logout-all (đổi pass) → **xóa toàn bộ session của user** (trừ session hiện tại, tùy chọn).

### 4.4 Cascade delete user (transaction)
```
BEGIN;
  DELETE FROM api_keys WHERE user_id=?;
  DELETE FROM sessions WHERE user_id=?;
  DELETE FROM agents   WHERE user_id=?;
  DELETE FROM messages WHERE user_id=?;   -- room của user biến mất hoàn toàn
  DELETE FROM users    WHERE id=?;
COMMIT;
```
- **Không cho xóa chính mình** và **không cho xóa admin cuối cùng** (`COUNT(*) role=admin` > 1) → trả 400.

### 4.5 Chống IDOR — checklist bắt buộc (defense in depth)
- [ ] `Store.Insert(userID, agent, room, …)` — ghi kèm chủ.
- [ ] `Store.Query(userID, params)` — `AND user_id = ?` là điều kiện **đầu tiên, không thể bỏ** (viết trong helper chung, không cho phép nil).
- [ ] `Store.ListRooms(userID)` — `WHERE user_id = ?`.
- [ ] `Store.DeleteRoom(userID, room)` — `AND user_id = ?`.
- [ ] SSE `handleStream`: match **cả** `userID` **và** `room`.
- [ ] `reply_to` phải thuộc cùng user (validate `SELECT user_id WHERE id=reply_to`).
- [ ] Admin đọc chéo **chỉ** qua `/api/admin/*` có `requireAdmin`.
- [ ] **Guardrail kỹ thuật (giảm rủi ro sót):** đổi signature các hàm store để **bắt buộc** `userID` là tham số đầu (không có overload); compiler sẽ chặn mọi call site cũ → không thể "quên" scope. Đây là biện pháp giảm rủi ro cốt lõi (§10).

---

## 5. Hub (SSE cô lập theo user)

- `Client` thêm `UserID int64`.
- `Add(userID int64, room string) *Client`.
- `send(userID, room, frame)`: match `c.UserID == userID && (c.Room == room || c.Room == "*")`.
- `Broadcast(userID, room, msg)`, `NotifyRoomDeleted(userID, room)`.
- **Admin realtime toàn hệ thống:** userID đặc biệt `0` (= wildcard), **chỉ** cấp qua endpoint admin; `send` coi `userID==0` là "mọi user" khi caller là admin.
- Call sites đổi: `handlePostMessage`, `handleRooms` DELETE, `handleStream` (3 nơi). Risk LOW.

---

## 6. API

### 6.1 Auth
| Method | Path | Mô tả |
|---|---|---|
| POST | `/api/auth/login` | set cookie; rate-limited (§6.5) |
| POST | `/api/auth/logout` | xóa session + cookie |
| GET | `/api/auth/me` | user hiện tại (id, username, role, must_change_password) |
| POST | `/api/auth/change-password` | `{old_password,new_password}`; xóa cờ must_change |

### 6.2 User self-service
| Method | Path | Mô tả |
|---|---|---|
| GET/POST | `/api/keys` | list / tạo key (plaintext 1 lần) |
| DELETE | `/api/keys/:id` | revoke key (scope user) |
| GET | `/api/agents` | agents + số tin đã gửi (theo user) |
| GET | `/api/stats` | rooms, messages, agents, keys của user |

### 6.3 Admin (`requireAdmin`)
| Method | Path | Mô tả |
|---|---|---|
| GET | `/api/admin/stats` | dashboard toàn hệ thống |
| GET | `/api/admin/users` | list users + thống kê mỗi user |
| POST | `/api/admin/users` | create user |
| PATCH | `/api/admin/users/:id` | disable/enable/set-role/set-password |
| DELETE | `/api/admin/users/:id` | cascade delete (§4.4) |
| GET | `/api/admin/agents` | hoạt động agent toàn hệ thống |
| GET | `/api/admin/rooms` | rooms toàn hệ thống (kèm chủ) |

### 6.4 Tương thích agent
- Giữ `POST/GET /api/messages` + SSE nhưng **bắt buộc** `X-API-Key` hoặc cookie.
- `room` mặc định `general` **trong không gian user** → user khác nhau = room riêng.
- Cập nhật `GUIDE.md` thêm `X-API-Key` vào mọi ví dụ + vòng lặp agent.
- **Cutover:** `AUTH_REQUIRED` mặc định `1`; nếu đặt `0` để debug thì log cảnh báo to rõ **mỗi lần khởi động** (`WARNING: auth disabled`) để không quên bật lại.

### 6.5 Rate-limit login (bắt buộc — chống brute-force `admin`/`123456`)
- In-memory limiter (không thêm lib): map khóa `username|ip` → đếm fail + mốc thời gian.
- Ngưỡng: **5 fail / 60s** mỗi khóa → khóa tạm **30s**; fail liên tục → backoff tăng (60s, 120s… tối đa 15'). Reset khi login thành công.
- Trả 429 với `Retry-After`. Dùng `sync.Mutex` guard; có GC định kỳ xóa entry hết hạn.

---

## 7. Thay đổi code theo file

| File | Loại | Nội dung |
|---|---|---|
| `go.mod`/`go.sum` | sửa | `golang.org/x/crypto` |
| `internal/store/store.go` | sửa | DSN `foreign_keys(1)`; schema; `Message.UserID`; `Insert/Query/ListRooms/DeleteRoom` nhận `userID` (bắt buộc) |
| `internal/store/auth.go` | **mới** | bcrypt hash/verify, session CRUD, key gen/verify, seed admin, dummy-hash |
| `internal/store/migrate.go` | **mới** | migration idempotent + backfill + seed |
| `internal/store/admin.go` | **mới** | CRUD user (transaction cascade), guard "last admin" |
| `internal/store/stats.go` | **mới** | stats user-level & system-level |
| `internal/hub/hub.go` | sửa | `Client.UserID`; `Add/Broadcast/send/NotifyRoomDeleted` theo userID |
| `internal/api/middleware.go` | **mới** | `requireUser`, `requireAdmin`, `resolveIdentity`, gắn context |
| `internal/api/ratelimit.go` | **mới** | login limiter in-memory |
| `internal/api/auth_handlers.go` | **mới** | login/logout/me/change-password |
| `internal/api/keys_handlers.go` | **mới** | keys/agents/stats (user) |
| `internal/api/admin_handlers.go` | **mới** | dashboard + CRUD users + system stats |
| `internal/api/handlers.go` | sửa | `Register()` route mới + middleware; messages/rooms/stream lấy `userID` từ context |
| `main.go` | sửa | env `AUTH_REQUIRED`/`BCRYPT_COST`/`TLS`; migrate + seed; log cảnh báo |
| `login.html` | **mới** | trang đăng nhập |
| `app.html` | **thay** | app shell: Chat / Agents / API Keys / Settings / Dashboard (admin-only panel) |
| `assets/` | **mới** | xem §9 |
| `GUIDE.md`, `README.md` | sửa | auth + setup + migration note |
| `smoke.sh` | sửa | case auth/IDOR/tenant (§8) |

---

## 8. Kiểm thử & nghiệm thu

### 8.1 Smoke mở rộng
1. login admin/123456 → 200 + cookie. 2. sai pass → 401. 3. `/api/auth/me` → role=admin.
4. admin tạo `u1` → 201; login u1 → 200. 5. u1 tạo key → plaintext 1 lần; list không lộ.
6. u1 agent POST bằng `X-API-Key` → 201. 7. **IDOR:** u1 không thấy tin admin qua `/api/*`.
8. **IDOR:** admin thấy room u1 qua `/api/admin/*`. 9. **IDOR:** u1 DELETE room admin → 404, dữ liệu còn.
10. **Tenant SSE:** stream u1 không nhận message admin. 11. Không key/cookie → 401.
12. admin disable u1 → login + key u1 → 401. 13. admin set-password u1 → pass mới OK.
14. admin delete u1 → cascade (keys/sessions/agents/messages/rooms) — login fail, room biến mất.
15. **Guard:** admin xóa chính mình → 400; xóa admin cuối → 400.
16. **Rate-limit:** 6 fail liên tiếp → 429 `Retry-After`. 17. `/api/admin/stats` khớp số liệu.

### 8.2 Regression
- 14 case cũ chỉnh qua auth (dùng API key) → vẫn PASS.
- `gofmt -l .` rỗng; `go vet ./...` sạch; `CGO_ENABLED=0 go build` OK.

### 8.3 Bảo mật thủ công
- Cookie HttpOnly/SameSite; không log plaintext key/password; key revoke hiệu lực ngay; timing login tương đương.

---

## 9. UI Redesign + `assets/` module hóa

### 9.0 Phân tích reference Dribbble (đã xem ảnh)

Ảnh: `Iris` — AI task tool, nền trắng tối giản, một **app card bo góc lớn nổi trên backdrop mờ nhiều màu (xanh mint → vàng nâu)**.

Các đặc trưng bắt buộc phải tái hiện:
- **App card nổi:** toàn app là 1 khối bo góc ~24px, có viền mảnh + shadow mềm, **cách đều mép viewport**; phía sau là nền blur gradient.
- **Sidebar trái (~280–300px):** wordmark góc trên + icon collapse; **ô search** dạng pill; nav item có **line-icon + label** (`New task`, `Library`, `Images`); nhóm có **section label** (`Projects`) kèm nút `+`; một **empty-state card viền nét đứt** ("Create a new task to get started"); **profile row ghim đáy** (avatar + tên + chevron).
- **Top-right:** pill badge (`Free Plan`) + pill accent (`Upgrade`).
- **Vùng chính:** nhiều khoảng trắng, **wordmark lớn căn giữa**, và **composer pill lớn** ở giữa-dưới: nút `+` tròn, chip `Tools`, selector model (`Flash ⌄`), mic, **nút action tròn đổ đầy** bên phải.
- **Ngôn ngữ thị giác:** surface gần trắng, viền rất nhạt, bo tròn lớn (pill cho input/button, 16–24px cho card), xám muted cho chữ phụ, **một accent duy nhất**, màu sắc chỉ đến từ backdrop.

**Ánh xạ sang AgentChat:** giữ nguyên ngôn ngữ trên nhưng đổi nội dung — sidebar trái = **rooms + nav** (`Chat`, `Agents`, `API Keys`, `Settings`, admin thấy thêm `Dashboard`); vùng giữa = **chat view** (thay wordmark + composer bằng danh sách message + composer pill tái dùng đúng style composer của reference); empty-state viền nét đứt = "Chưa có room / Tạo room để bắt đầu"; profile row đáy = **user hiện tại + logout**.

### 9.1 Cấu trúc `assets/`
```
assets/
  index.md          # MỤC LỤC cho agent: mô tả từng file, responsibility, API nội bộ, cách mở rộng
  css/
    tokens.css      # design tokens (§9.3) + theme sáng/tối + backdrop mesh
    base.css        # reset + typography + focus ring
    layout.css      # app-card, sidebar, topbar, grid, responsive
    components.css  # card, pill, button, badge, table, modal, toast, skeleton, sparkline (§9.6)
    chat.css        # view chat (giữ markdown/KaTeX/mention)
  js/
    api.js          # fetch wrapper + xử lý 401 → redirect login
    store.js        # state nhỏ + helpers
    auth.js         # login/logout/me/change-password
    chat.js         # load/broadcast/SSE
    agents.js       # panel agents
    keys.js         # panel API keys (modal tạo + copy 1 lần)
    admin.js        # dashboard + CRUD user
    ui.js           # theme toggle, toast, render helpers, router panel
  vendor/           # (tùy chọn) copy marked/dompurify/katex nếu muốn offline
```

### 9.2 `assets/index.md` (mục lục cho agent maintain)
Nội dung bắt buộc gồm: mục đích từng file, quan hệ phụ thuộc (dependency graph), quy ước đặt tên, cách thêm 1 panel mới, cách thêm 1 route admin mới, danh sách endpoint UI gọi, checklist khi đổi design token. → giúp agent sau này maintain không cần đọc toàn bộ.

### 9.3 Design tokens

```css
:root {
  /* layout */
  --app-radius: 24px;   --card-radius: 16px;  --pill: 999px;
  --sidebar-w: 288px;   --gutter: 24px;
  /* surfaces (light mặc định, dark theo [data-theme=dark]) */
  --bg: #eef1ec;                 /* base dưới app card */
  --surface: #ffffff;           /* app card + panel */
  --surface-2: #f4f5f3;         /* hover / chip / input nền */
  --border: #e7e9e6;            /* viền rất nhạt */
  --text: #14161a;  --muted: #8b9089;
  /* accent (một màu duy nhất, tinh tế) */
  --accent: #1f2937;            /* nút action đổ đầy (đen mềm như reference) */
  --accent-soft: #eef2ff;       /* badge Upgrade/Free Plan nền nhạt */
  --ok: #16a34a;  --warn: #d97706;  --danger: #dc2626;
  /* shape/space */
  --shadow: 0 24px 60px rgba(20,22,26,.16), 0 2px 8px rgba(20,22,26,.06);
  --r-sm: 8px; --r-md: 12px; --r-lg: 20px;
  --s1: 4px; --s2: 8px; --s3: 12px; --s4: 16px; --s5: 24px; --s6: 32px;
}
[data-theme="dark"] {
  --bg: #0b0d12; --surface: #14171f; --surface-2: #1b1f2a;
  --border: #262b38; --text: #e6e8ee; --muted: #8b93a7;
  --accent: #ffffff; --accent-soft: #1e2438;
}
```

- Font: **Inter / system-ui**; wordmark dùng weight 600, letter-spacing nhẹ.
- **Backdrop blur:** body có 1 lớp `radial-gradient`/mesh nhiều màu (mint→amber) + `filter: blur(40px)` làm nền; app card nổi trên đó (đúng chất reference). Dark mode dùng mesh tối hơn, độ bão hoà thấp.

### 9.4 Layout shell (dùng chung trong `app.html`)

```
┌ (viewport: backdrop mesh mờ) ──────────────────────────────┐
│  ┌──── app card (bo 24px, shadow, margin 24) ───────────┐  │
│  │ ┌ sidebar 288 ┐ ┌ main ───────────────────────────┐ │  │
│  │ │ wordmark  ☰ │ │ topbar: breadcrumb  🔍  ◐  👤 │ │  │
│  │ │ [search…]   │ │                              │ │  │
│  │ │ ○ Chat      │ │   (nội dung panel / chat)    │ │  │
│  │ │ ○ Agents    │ │                              │ │  │
│  │ │ ○ API Keys  │ │                              │ │  │
│  │ │ ○ Settings  │ │                              │ │  │
│  │ │ ROOMS    +  │ │                              │ │  │
│  │ │  #general   │ │                              │ │  │
│  │ │  #task-1    │ │ ┌ composer pill ───────────┐ │ │  │
│  │ │ ┌ nét đứt ┐  │ │ │ + │ Tools │ … │ 🎤 │ ● │ │ │  │
│  │ │ │ empty   │  │ │ └──────────────────────────┘ │ │  │
│  │ │ └─────────┘  │ └──────────────────────────────┘ │  │
│  │ │ 👤 user  ⌄  │                                    │  │
│  │ └─────────────┘                                    │  │
│  └────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### 9.5 Chi tiết từng màn hình

**`login.html`** — backdrop mesh mờ + **card trắng bo 24px** giữa màn hình (~380px): wordmark, tiêu đề nhỏ muted, input username/password dạng **ô bo 12px viền nhạt**, nút submit **pill đổ đầy**, dòng lỗi `--danger`. Nếu `must_change_password` → chuyển thẳng sang form đổi mật khẩu (cùng style).

**`app.html` — Chat** (màn mặc định):
- Main: dải message dùng **card trắng bo 16px** cho mỗi message (như hiện tại nhưng theo token mới), meta muted, agent name = màu accent; giữ **markdown/KaTeX/mention**.
- Dưới cùng: **composer pill** tái hiện reference — trái nút `+` tròn (menu: chèn room/mention nhanh), chip `Tools` (toggle lọc/markdown preview), giữa input trong suốt placeholder "Nhập tin… dùng @agent", phải nút **action tròn đổ đầy** (`--accent`) = Gửi.
- Topbar: breadcrumb `#room` + trạng thái kết nối (`● live` xanh) + theme toggle `◐` + avatar.
- Empty state: card **viền nét đứt** + icon + "Chưa có tin trong room này".

**Panel `Agents`** — bảng trong card trắng: cột `Agent · Số tin · Hoạt động cuối · Trạng thái`; hàng hover `--surface-2`; stat nhỏ trên đầu (tổng agent, tổng tin) dạng chip.

**Panel `API Keys`** — nút "Tạo key" (pill đổ đầy) mở modal; khi tạo xong hiện **banner key 1 lần** (nền `--accent-soft`, nút Copy). Bảng key: `prefix…`, tên, trạng thái (badge ok/disabled), `last_used_at`, nút Revoke (`--danger`).

**Panel `Settings`** — form đổi mật khẩu trong card: old/new/confirm, nút pill; hiển thị `username/role` readonly phía trên.

**Panel `Dashboard` (chỉ admin, trong `app.html`):**
- Hàng **stat cards** (bo 16px, viền nhạt, số lớn + label muted + sparkline SVG nhỏ): `Users`, `Rooms`, `Messages`, `Active keys`.
- **SVG chart tự vẽ** (bar/line) — không thêm CDN lib.
- Bảng `Users`: `username · role badge · status badge · #rooms · #messages · last_login`; action menu: Disable/Enable, Reset password (modal), Delete (confirm gõ tên).
- Tab `Agents` toàn hệ thống + tab `Rooms` (kèm chủ sở hữu).
- Tất cả theo cùng ngôn ngữ card/pill/viền nhạt của reference.

### 9.6 Components (trong `components.css`)
`app-card`, `sidebar`, `nav-item` (active = nền `--surface-2` + chữ đậm), `search-pill`, `empty-dashed`, `composer-pill`, `action-btn` (tròn đổ đầy), `stat-card`, `badge` (free/upgrade/role/status), `table`, `modal`, `toast`, `skeleton`, `sparkline` (SVG), `avatar`, `theme-toggle`.

### 9.7 Responsive & a11y
- `< 900px`: sidebar thu thành icon-rail, app card full-bleed (bỏ margin/bo góc), composer giữ pill.
- `< 600px`: stat cards xếp 1 cột; bảng chuyển card-list.
- Focus ring rõ, `aria-label` cho icon-button, contrast AA, tôn trọng `prefers-color-scheme` cho theme mặc định.

---

## 10. Giảm thiểu rủi ro (gia cố so với v1)

| Rủi ro | v1 | **Gia cố v2** |
|---|---|---|
| **Sót đường truy vấn không scope (IDOR)** — HIGH | Checklist + test | **1) Signature bắt buộc `userID` (compiler chặn mọi call site cũ).** 2) `Query` luôn chèn `user_id=?` ở helper chung, không nhánh nào bỏ qua. 3) Test IDOR tự động nhiều hướng (đọc/ghi/xóa/SSE/reply_to). 4) `detect_changes` review trước commit. |
| **Brute-force admin/123456** — HIGH | rate-limit mơ hồ | **Limiter cụ thể (§6.5): 5 fail/60s → 429 + backoff.** Ép `must_change_password` lần đầu. Cảnh báo log `123456`. |
| **Username enumeration / timing** — MED | không có | **Dummy bcrypt hash khi user không tồn tại** + thông điệp lỗi đồng nhất. |
| **Xóa user để lại room mồ côi** — MED | "quyết định cascade" chưa rõ cơ chế | **Transaction cascade tường minh (§4.4)** + guard chống xóa admin cuối/chính mình. |
| **Quên bật lại auth khi debug** — MED | env đơn thuần | **Log WARNING mỗi lần khởi động khi `AUTH_REQUIRED=0`.** Mặc định =1. |
| **Session hijack** — MED | cookie cơ bản | HttpOnly + SameSite=Lax + Secure(TLS) + logout-all khi đổi pass. |
| **bcrypt chậm gây DoS login** (cost cao) | chưa xét | cost 12 + **rate-limit chặn trước khi hash** (limiter ở middleware, trước bcrypt). |
| **Regression call-graph** — LOW | smoke | impact analysis LOW + 14 case cũ chạy lại qua auth. |
| **Thêm x/crypto phá CGO=0** — LOW | verify | x/crypto thuần Go; test `CGO_ENABLED=0 go build`. |
| **Hub match sai tenant** — MED | suy luận | `send` match **cả** userID + room; test tenant SSE. |

---

## 11. Thứ tự thực hiện

1. `store`: DSN `foreign_keys`, schema + `migrate.go` + `auth.go` + seed admin + `admin.go` cascade + `stats.go`.
2. `hub`: userID scoping.
3. `api/middleware.go` + `ratelimit.go` + auth handlers.
4. Scope `handlers.go` (messages/rooms/stream) theo identity (**đổi signature trước để compiler dẫn đường**).
5. Keys + user stats/agents handlers.
6. Admin handlers + stats.
7. `main.go` wiring + env + startup warning.
8. `smoke.sh` mở rộng + regression.
9. UI: `assets/` (css/js + `index.md`) → `login.html` → `app.html` (kèm panel dashboard admin).
10. Docs + re-index GitNexus + `detect_changes`.
11. Commit & push (khi user yêu cầu).
