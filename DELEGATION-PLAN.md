# Delegation Plan — AgentChat Multi-user/Auth/Admin/UI

> Dùng file này để **điều phối sub-agent**. Đọc kèm `PLAN-MULTIUSER.md` (spec gốc).
> Nguyên tắc: mỗi Work Package (WP) **nhỏ**, **độc lập**, có **1 mục tiêu**, **input rõ**, **output cụ thể**, **lệnh verify**, **definition of done (DoD)**.

---

## A. Protocol cho sub-agent (BẮT BUỘC đọc trước khi làm)

Bất kỳ sub-agent nào nhận 1 WP phải tuân thủ:

1. **Chỉ làm đúng 1 WP.** Không sửa file ngoài danh sách `FILES` của WP.
2. **Đọc trước, sửa sau.** Bắt buộc `Read` mọi file trong `FILES` trước khi `Edit`/`Write`.
3. **Không thêm comment** vào code (trừ doc trong `assets/index.md`).
4. **Không đổi chữ ký hàm public chưa được giao.** Nếu cần, dừng lại và ghi `BLOCKED`.
5. **Chạy đúng `VERIFY`** và dán output thật vào báo cáo. Không tự suy diễn "pass".
6. **Không commit / không push.** Chỉ để working tree.
7. **Báo cáo theo format** (mục D). Thiếu mục nào → coi như chưa xong.
8. **Nếu kẹt** (thiếu dep, spec mâu thuẫn, test fail không hiểu): dừng, ghi `BLOCKED + lý do + đề xuất`, KHÔNG tự bịa.
9. **Toolchain:** `export PATH=$PATH:/home/baongo/go/bin`. Build: `CGO_ENABLED=0 go build -o agentchat .`
10. **Không sửa** `chat.db`, `*.db-wal`, `agentchat`, `.gitnexus/`, `.claude/`, `AGENTS.md`, `CLAUDE.md`.

### Ranh giới "được phép chạm"
- Chỉ các file trong `FILES` của WP mình.
- WP frontend **không** chạm file `.go`. WP backend **không** chạm `.html`/`assets/`.

---

## B. Bản đồ phụ thuộc (dependency graph)

```
Wave 0 (sequencer — 1 agent, chạy 1 mình)
  WP-00  go.mod: thêm x/crypto + verify build

Wave 1 (backend nền — tuần tự, cùng 1 agent backend)
  WP-01  store: PRAGMA foreign_keys + Message.UserID + migration + schema auth
  WP-02  store/auth.go: bcrypt + session + api key + seed admin  (dep: WP-01)
  WP-03  store/admin.go + stats.go: CRUD user cascade + thống kê (dep: WP-02)

Wave 2 (hub — đọc lập sau WP-01)
  WP-04  hub: Client.UserID + scope mọi hàm            (dep: WP-01)

Wave 3 (api middleware + scope — tuần tự)
  WP-05  api/middleware.go: identity/requireUser/requireAdmin (dep: WP-02)
  WP-06  api/ratelimit.go: login limiter              (dep: WP-05)
  WP-07  api/auth_handlers.go: login/logout/me/change-password (dep: WP-05,06)
  WP-08  handlers.go: scope messages/rooms/stream theo userID (dep: WP-01,04,05)

Wave 4 (api handlers mới)
  WP-09  api/keys_handlers.go: keys/agents/stats (user) (dep: WP-03,05,08)
  WP-10  api/admin_handlers.go: dashboard + CRUD user  (dep: WP-03,05)

Wave 5 (wiring + docs backend)
  WP-11  main.go: env + migrate + seed + startup warning (dep: WP-02,03,07,09,10)
  WP-12  GUIDE.md + README.md: auth + api key docs     (dep: WP-08,09)

Wave 6 (test backend)
  WP-13  smoke.sh: case auth/IDOR/tenant/rate-limit    (dep: WP-11)
  WP-14  regression: chạy lại 14 case cũ qua auth      (dep: WP-13)

Wave 7 (frontend — dùng assets, tuần tự)
  WP-15  assets/css: tokens+base+layout+components+chat (độc lập)
  WP-16  assets/index.md: mục lục cho agent            (dep: WP-15)
  WP-17  login.html + js/auth.js + js/api.js           (dep: WP-15)
  WP-18  app.html + js/ui.js/router + js/chat.js       (dep: WP-15,17)
  WP-19  app panels: js/agents.js + js/keys.js + js/settings (dep: WP-18)
  WP-20  app.html dashboard panel + js/admin.js         (dep: WP-15,17)
```

### Song song hóa an toàn
- **Được chạy song song:** `{WP-04}` ∥ `{WP-02→WP-03}` (khác thư mục).
- **Được chạy song song:** toàn bộ Wave 7 frontend (`WP-15` xong trước; các WP khác chờ `WP-15`) ∥ Wave 1–6 backend, **vì khác loại file** — nhưng integration test cần cả hai.
- **KHÔNG song song** các WP cùng file: `WP-02`/`WP-03` (đều `store/`), `WP-05`/`WP-06`/`WP-07` (đều `api/`), `WP-09`/`WP-10` (đều `api/`).

---

## C. Work Packages (chi tiết)

> Format mỗi WP: **Mục tiêu · Dep · Files · Input · Việc làm · Output · Verify · DoD**

---

### WP-00 — Thêm dependency x/crypto
- **Mục tiêu:** `golang.org/x/crypto/bcrypt` khả dụng, build CGO=0 vẫn OK.
- **Dep:** none.
- **Files:** `go.mod`, `go.sum`.
- **Việc làm:**
  1. `export PATH=$PATH:/home/baongo/go/bin && cd /home/baongo/Projects/AgentChat`
  2. `go get golang.org/x/crypto@latest && go mod tidy`
  3. Tạo file tạm `zz_probe.go` import bcrypt, build, rồi **xóa file tạm**.
- **Verify:** `CGO_ENABLED=0 go build -o agentchat . && go vet ./...`
- **DoD:** build xanh, `go.mod` có `golang.org/x/crypto`, không còn file tạm.

---

### WP-01 — Store: PRAGMA FK + cột user_id + schema auth
- **Mục tiêu:** DB có bảng `users/api_keys/sessions/agents`; `messages.user_id`; migration idempotent.
- **Dep:** WP-00.
- **Files:** `internal/store/store.go`, `internal/store/migrate.go` (mới).
- **Input:** `PLAN-MULTIUSER.md` §3.
- **Việc làm:**
  1. DSN thêm `_pragma=foreign_keys(1)`; `init()` thêm `PRAGMA foreign_keys=ON`.
  2. `Message` thêm `UserID int64 \`json:"user_id"\``.
  3. `migrate.go`: `Migrate()` idempotent — tạo 4 bảng mới (đúng DDL §3.2), thêm cột `user_id` nếu thiếu (`PRAGMA table_info`), tạo index, backfill `UPDATE messages SET user_id=1 WHERE user_id=0`.
  4. `init()` gọi `Migrate()`.
- **Output:** bảng + cột tồn tại sau khi chạy; message cũ user_id=1.
- **Verify:**
  - `go build ./...`
  - Chạy server tạm, `sqlite3 chat.db ".tables"` thấy 4 bảng + users; `.schema messages` có `user_id`.
- **DoD:** build xanh; 4 bảng + cột đúng; chạy `Migrate()` 2 lần không lỗi.

---

### WP-02 — store/auth.go: bcrypt + session + api key + seed admin
- **Mục tiêu:** hàm auth nguyên thủy ở tầng store.
- **Dep:** WP-01.
- **Files:** `internal/store/auth.go` (mới).
- **Việc làm (export đúng tên):**
  - `HashPassword(pw string, cost int) (string, error)`, `CheckPassword(hash, pw string) bool`.
  - `SeedAdmin() error`: nếu chưa có username `admin` → tạo role=admin, `must_change_password=1`, hash `123456`.
  - `GetUserByUsername`, `GetUserByID`.
  - `CreateSession(userID int64, ttl time.Duration) (string, error)` (32 bytes hex), `GetSession(id)` (join user, check `expires_at`), `DeleteSession(id)`, `DeleteSessionsForUser(userID)`, `TouchLastLogin(userID)`.
  - `CreateAPIKey(userID int64, name, agentName string) (plain, prefix string, err error)` → plain `ac_`+hex, lưu sha256+prefix; `UserByAPIKey(plain)` (sha256 lookup, check status/user active, cập nhật `last_used_at`); `ListAPIKeys(userID)`, `RevokeAPIKey(userID, id)`.
  - **Dummy hash** hằng số cho login timing-safe.
- **Verify:** `go build ./...`; unit thủ công bằng `sqlite3` sau khi gọi qua test script tạm (tạo/xóa file `_auth_probe_test.go` rồi xóa).
- **DoD:** build xanh; seed tạo đúng 1 admin; `CheckPassword` đúng; session hết hạn bị từ chối.

---

### WP-03 — store/admin.go + stats.go
- **Mục tiêu:** CRUD user cascade (transaction) + guard admin + thống kê.
- **Dep:** WP-02.
- **Files:** `internal/store/admin.go` (mới), `internal/store/stats.go` (mới).
- **Việc làm:**
  - `CreateUser(username, pw, role)`, `ListUsersWithStats()`, `SetUserStatus(id, status)`, `SetUserRole(id, role)`, `AdminSetPassword(id, pw, mustChange bool)`.
  - `DeleteUserCascade(id)`: transaction đúng thứ tự §4.4; **guard**: không xóa self, không xóa admin cuối (`COUNT role=admin > 1`).
  - `CountAdmins()`.
  - `stats.go`: `UserStats(userID)` (rooms, messages, agents, keys) và `SystemStats()` (users, rooms, messages, active_keys).
  - `ListAgents(userID)` + `AgentCounts(userID)` (số tin/agent) cho panel.
- **Verify:** `go build ./...`; test cascade: tạo user+key+msg → `DeleteUserCascade` → không còn row nào của user; guard trả error đúng.
- **DoD:** build xanh; cascade đúng; guard hoạt động; stats khớp COUNT.

---

### WP-04 — Hub: scope theo userID
- **Mục tiêu:** SSE cô lập tenant.
- **Dep:** WP-01.
- **Files:** `internal/hub/hub.go`.
- **Việc làm:**
  - `Client` thêm `UserID int64`.
  - `Add(userID int64, room string) *Client`.
  - `send(userID int64, room string, frame []byte)`: match `(userID==0 || c.UserID==userID) && (c.Room==room || c.Room=="*")`.
  - `Broadcast(userID, room, msg)`, `NotifyRoomDeleted(userID, room)`.
- **Verify:** `go build ./...`.
- **DoD:** build xanh; không còn chữ ký cũ.

---

### WP-05 — api/middleware.go
- **Mục tiêu:** xác thực + context identity.
- **Dep:** WP-02.
- **Files:** `internal/api/middleware.go` (mới).
- **Việc làm:**
  - `type Identity struct{ UserID int64; Username, Role string; Via string }`.
  - `resolveIdentity(r) (*Identity, error)`: cookie session **trước**, fallback `X-API-Key`.
  - `requireUser(next)`, `requireAdmin(next)` trả 401/403 JSON đúng.
  - `IdentityFrom(ctx)` helper.
  - Đọc env `AUTH_REQUIRED` (mặc định "1"); nếu "0" → bypass gắn identity admin giả (chỉ khi tắt) + vẫn cho phép cookie/key nếu có.
- **Verify:** `go build ./...`.
- **DoD:** build xanh; 401 khi không identity và auth bật.

---

### WP-06 — api/ratelimit.go
- **Mục tiêu:** chặn brute-force login.
- **Dep:** WP-05.
- **Files:** `internal/api/ratelimit.go` (mới).
- **Việc làm:** limiter in-memory theo khóa `username|ip`; 5 fail/60s → khóa 30s, backoff tăng (60/120… tối đa 15'); `Allow(key) (ok bool, retryAfter int)`; `Fail(key)`; `Reset(key)`; GC định kỳ; guard `sync.Mutex`.
- **Verify:** `go build ./...`; test nhanh bằng script gọi 6 lần liên tiếp (sau WP-07).
- **DoD:** build xanh; sau 5 fail → chặn có `Retry-After`.

---

### WP-07 — api/auth_handlers.go
- **Mục tiêu:** endpoint auth.
- **Dep:** WP-05, WP-06.
- **Files:** `internal/api/auth_handlers.go` (mới).
- **Việc làm:** `handleLogin` (limit → dummy bcrypt timing-safe → set cookie HttpOnly/SameSite=Lax/Secure nếu `TLS=1`), `handleLogout`, `handleMe`, `handleChangePassword` (verify old, update hash, xóa `must_change_password`, `DeleteSessionsForUser` trừ session hiện tại).
- **Verify:** `go build ./...` + curl thủ công (WP-11 chưa wire → có thể test bằng cách tạm wire trong main sau WP-11).
- **DoD:** build xanh; login admin/123456 → 200 + cookie; sai → 401; 6 fail → 429.

---

### WP-08 — handlers.go: scope messages/rooms/stream
- **Mục tiêu:** mọi đường đọc/ghi bị ràng theo userID (chống IDOR).
- **Dep:** WP-01, WP-04, WP-05.
- **Files:** `internal/api/handlers.go`.
- **Việc làm:**
  - `handleMessages`: GET `Store.Query(id.UserID, params)`; POST → `Store.Insert(id.UserID, ...)`; validate `reply_to` cùng user.
  - `handleRooms`: GET `ListRooms(id.UserID)`; DELETE `DeleteRoom(id.UserID, room)` + `Hub.NotifyRoomDeleted(id.UserID, room)`.
  - `handleStream`: `Hub.Add(id.UserID, room)`.
  - `handlePostMessage`: `Hub.Broadcast(id.UserID, room, msg)`.
  - `Register()`: bọc `requireUser` cho nhóm `/api/messages`, `/api/rooms`, `/api/stream`.
- **Verify:** `go build ./...`.
- **DoD:** build xanh; không còn call `Query/Insert/ListRooms/DeleteRoom/Broadcast` thiếu userID.

---

### WP-09 — api/keys_handlers.go
- **Mục tiêu:** API user self-service.
- **Dep:** WP-03, WP-05, WP-08.
- **Files:** `internal/api/keys_handlers.go` (mới), sửa `internal/api/handlers.go` (chỉ thêm route trong `Register`).
- **Việc làm:** `GET/POST /api/keys`, `DELETE /api/keys/:id`, `GET /api/agents`, `GET /api/stats`.
- **Verify:** `go build ./...`.
- **DoD:** build xanh; create key trả plaintext đúng 1 lần; list không lộ plaintext.

---

### WP-10 — api/admin_handlers.go
- **Mục tiêu:** admin dashboard + CRUD user.
- **Dep:** WP-03, WP-05.
- **Files:** `internal/api/admin_handlers.go` (mới), sửa `internal/api/handlers.go` (thêm route, bọc `requireAdmin`).
- **Việc làm:** `GET /api/admin/stats|users|agents|rooms`, `POST /api/admin/users`, `PATCH /api/admin/users/:id`, `DELETE /api/admin/users/:id`.
- **Verify:** `go build ./...`.
- **DoD:** build xanh; non-admin → 403; delete self/last admin → 400.

---

### WP-11 — main.go wiring
- **Mục tiêu:** boot đúng: env, migrate, seed, warning.
- **Dep:** WP-02, WP-03, WP-07, WP-09, WP-10.
- **Files:** `main.go`.
- **Việc làm:** env `AUTH_REQUIRED`(1)/`BCRYPT_COST`(12)/`TLS`(0); gọi `store.SeedAdmin()` sau migrate; nếu `AUTH_REQUIRED=0` → `log.Printf("WARNING: auth disabled...")`.
- **Verify:** chạy `./agentchat`; `curl /api/health` 200.
- **DoD:** boot sạch; log warning chỉ khi auth tắt; admin seed tồn tại.

---

### WP-12 — Docs
- **Mục tiêu:** tài liệu hoá auth + api key.
- **Dep:** WP-08, WP-09.
- **Files:** `GUIDE.md`, `README.md`.
- **Việc làm:** GUIDE: mọi ví dụ thêm `-H 'X-API-Key: ac_...'`; mục login; vòng lặp agent có key. README: tài khoản mặc định admin/123456 + ép đổi pass; note migration.
- **Verify:** đọc lại, đảm bảo ví dụ khớp endpoint thực.
- **DoD:** không còn ví dụ ẩn danh; hướng dẫn đủ để agent mới join.

---

### WP-13 — smoke.sh mở rộng
- **Mục tiêu:** phủ §8.1.
- **Dep:** WP-11.
- **Files:** `smoke.sh`.
- **Việc làm:** thêm 17 case §8.1 (login, me, tạo user, key, IDOR đọc/ghi/xóa, tenant SSE, 401, disable, set-password, cascade delete, guard admin, rate-limit, stats). Giữ fallback jq/python3.
- **Verify:** `BASE=http://127.0.0.1:8086 ./smoke.sh` → PASS tất cả.
- **DoD:** exit 0; báo cáo PASS/FAIL từng case.

---

### WP-14 — Regression
- **Mục tiêu:** 14 case cũ vẫn đúng qua auth.
- **Dep:** WP-13.
- **Files:** `smoke.sh` (chỉnh case cũ dùng API key), có thể thêm `smoke-legacy.sh`.
- **Việc làm:** xác nhận CRUD tin/room/SSE/filter vẫn pass khi có identity.
- **Verify:** `gofmt -l .` rỗng; `go vet ./...`; chạy cả bộ smoke.
- **DoD:** tất cả xanh, không hồi quy.

---

### WP-15 — assets/css
- **Mục tiêu:** design system theo reference.
- **Dep:** none (chỉ cần `PLAN-MULTIUSER.md` §9.3–9.6).
- **Files:** `assets/css/tokens.css`, `base.css`, `layout.css`, `components.css`, `chat.css`.
- **Việc làm:** tokens đúng §9.3; app-card/backdrop mesh; sidebar; composer-pill; stat-card; badge; table; modal; toast; skeleton; sparkline; responsive §9.7; light+`[data-theme=dark]`.
- **Verify:** tạo `assets/_preview.html` tạm render mọi component, mở bằng trình duyệt; **xóa sau khi xong**.
- **DoD:** đủ component; theme dark hoạt động; không lib CDN mới.

---

### WP-16 — assets/index.md
- **Mục tiêu:** mục lục cho agent maintain.
- **Dep:** WP-15.
- **Files:** `assets/index.md`.
- **Việc làm:** bảng file → responsibility; dependency graph JS; quy ước đặt tên; "cách thêm 1 panel"; "cách thêm 1 route admin"; danh sách endpoint UI gọi; checklist đổi token.
- **Verify:** mỗi file trong `assets/` đều có mục.
- **DoD:** agent khác đọc file này hiểu cấu trúc, không cần đọc hết code.

---

### WP-17 — login + auth.js + api.js
- **Mục tiêu:** trang đăng nhập + lớp fetch.
- **Dep:** WP-15.
- **Files:** `login.html`, `assets/js/api.js`, `assets/js/auth.js`.
- **Việc làm:** card login theo §9.5; api.js wrapper xử lý 401 → redirect; auth.js login/logout/me; xử lý `must_change_password` → chuyển form đổi pass.
- **Verify:** mở `login.html`, login admin/123456 (server WP-11 chạy).
- **DoD:** login OK → vào `/app.html`; sai → hiện lỗi `--danger`.

---

### WP-18 — app shell + chat
- **Mục tiêu:** app card + sidebar + chat view.
- **Dep:** WP-15, WP-17.
- **Files:** `app.html`, `assets/js/ui.js`, `assets/js/store.js`, `assets/js/chat.js`.
- **Việc làm:** layout §9.4; router panel; chat giữ markdown/KaTeX/mention/SSE; composer pill tái hiện reference; topbar trạng thái live.
- **Verify:** gửi/nhận tin realtime; reload giữ lịch sử.
- **DoD:** chat hoạt động qua auth (cookie/key); UI khớp reference.

---

### WP-19 — panels Agents/Keys/Settings
- **Mục tiêu:** user panel.
- **Dep:** WP-18.
- **Files:** `assets/js/agents.js`, `assets/js/keys.js`, `assets/js/settings.js` (+ đăng ký panel trong `ui.js`).
- **Việc làm:** Agents (bảng + số tin + last active), Keys (modal tạo + banner copy 1 lần + revoke), Settings (đổi pass).
- **Verify:** tạo key, dùng key gọi API OK; đổi pass → login lại.
- **DoD:** 3 panel hoạt động; dữ liệu scope đúng user.

---

### WP-20 — admin dashboard (panel trong `app.html`)
- **Mục tiêu:** admin UI.
- **Dep:** WP-15, WP-17.
- **Files:** `app.html` (panel `dashboard`), `assets/js/admin.js`.
- **Việc làm:** stat cards + SVG chart; bảng users (disable/enable/reset/delete có confirm gõ tên); tab agents/rooms.
- **Verify:** login admin → `app.html` → nav `Dashboard`; thao tác CRUD phản ánh đúng.
- **DoD:** dashboard đúng số liệu; user thường không thấy nav `Dashboard`.

---

## D. Format báo cáo sub-agent (dán vào cuối task)

```
WP: <id> — <tên>
STATUS: DONE | BLOCKED
FILES_CHANGED:
  - <path> (new|modified)
WHAT_DONE: <2-5 gạch đầu dòng>
VERIFY_CMD: <lệnh đã chạy>
VERIFY_OUTPUT: <dán output thật>
DOF_MET: yes|no
BLOCKERS: <none | mô tả + đề xuất>
NOTES_FOR_NEXT: <điểm agent kế cần biết>
```

---

## E. Thứ tự chạy đề xuất

1. **WP-00** (tuần tự).
2. Chạy **backend**: WP-01 → WP-02 → WP-03 → WP-04 → WP-05 → WP-06 → WP-07 → WP-08 → WP-09 → WP-10 → WP-11 → WP-12 → WP-13 → WP-14.
3. **Song song** (sau khi WP-15 xong): frontend WP-15 → WP-16/17 → WP-18 → WP-19/20.
4. **Integration cuối:** bật server, chạy `smoke.sh`, mở UI kiểm tra end-to-end.
5. Sau khi tất cả xong: `gitnexus analyze . --skip-git` + `gitnexus_detect_changes`, rồi (khi user yêu cầu) commit/push.

### Gate giữa các wave
- Hết Wave 1: `go build ./...` + `go vet ./...` xanh.
- Hết Wave 3: curl login + IDOR test thủ công.
- Hết Wave 6: `smoke.sh` exit 0.
- Hết Wave 7: UI end-to-end qua browser.

---

## F. Checklist điều phối (orchestrator)

- [ ] Mỗi WP giao cho 1 agent, kèm **nguyên văn mục A + WP tương ứng + mục D**.
- [ ] Không giao 2 WP chạm cùng file cho 2 agent cùng lúc.
- [ ] Sau mỗi WP: kiểm `STATUS=DONE` + `DOF_MET=yes` + dán `VERIFY_OUTPUT` thật.
- [ ] Nếu `BLOCKED`: đọc `BLOCKERS`, điều chỉnh scope/instruct rồi giao lại (không tự sửa hộ trừ khi quyết định).
- [ ] Chỉ chạy WP phụ thuộc khi mọi dep đã DONE.
- [ ] Cuối cùng: chạy `detect_changes` để chắc blast radius đúng kỳ vọng.
