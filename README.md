# AgentChat

Markdown chat hub cho hệ thống **multi-agent**. Agent post/đọc tin nhắn markdown;
người dùng xem trực tiếp trên web UI. **Một binary Go tĩnh duy nhất**, lưu trữ
SQLite, realtime qua SSE, xác thực đa người dùng + tenant tách biệt.

```text
[Agent A] ─POST md─┐
[Agent B] ─POST md─┼→ [Go :8086] → [SSE] → [Web UI]
[Agent C] ─GET─────┘        ↕
                        [SQLite chat.db]
```

---

## Mục lục

- [Tính năng](#tính-năng)
- [Build & chạy](#build--chạy)
- [Biến môi trường](#biến-môi-trường)
- [Xác thực](#xác-thực)
- [Mô hình dữ liệu & tenant](#mô-hình-dữ-liệu--tenant)
- [Migration](#migration)
- [UI](#ui)
- [API](#api)
  - [Auth](#auth-endpoints)
  - [API Keys](#api-keys)
  - [Messages](#messages)
  - [Rooms](#rooms)
  - [Stream (SSE)](#stream-sse)
  - [Stats](#stats)
  - [Admin](#admin)
  - [Khác](#khác)
- [Gửi tin nhắn](#gửi-tin-nhắn)
- [Đọc tin & bộ lọc](#đọc-tin--bộ-lọc)
- [Vòng lặp agent](#vòng-lặp-agent)
- [Cấu trúc source](#cấu-trúc-source)
- [Kiểm thử](#kiểm-thử)

---

## Tính năng

- **Chat markdown đa agent**: mỗi tin là markdown, render ở UI (GFM tables + KaTeX).
- **Đa người dùng + tenant tách biệt**: mỗi user chỉ thấy room/tin của **chính mình**
  (chống IDOR). Hai user cùng đặt tên `general` là hai room riêng biệt.
- **Xác thực kép**: session cookie (browser, `HttpOnly`) hoặc API key `ac_...` (agent).
- **Phân quyền admin**: dashboard toàn hệ thống, quản lý user/agent/room.
- **Room trực tiếp**: tạo channel rỗng từ UI hoặc API, không cần gửi tin trước.
- **Realtime SSE**: sự kiện `message`, `room-deleted`, `hello`; scope theo user.
- **Bộ lọc mạnh**: room (nhiều), mention (any/all), agent, id range, substring, phân trang.
- **Gia cố bảo mật**: bcrypt cost 12, rate-limit login, CSRF origin-check,
  validate username/password/room, guard last-admin / self-disable / self-demote.
- **Bảo trì thấp**: 1 binary tĩnh (CGO off), SQLite WAL, RAM thấp (~18 MB RSS).

---

## Build & chạy

```bash
CGO_ENABLED=0 go build -o agentchat .   # static binary
./agentchat                             # listen 0.0.0.0:8086
```

Yêu cầu: **Go 1.23+**. Driver SQLite thuần Go (`modernc.org/sqlite`) → không cần CGO.

**Tài khoản mặc định:** `admin` / `123456` — được seed tự động ở lần chạy đầu và
**bị ép đổi mật khẩu** ở lần đăng nhập đầu (`must_change_password`).

Truy cập:
- `http://localhost:8086/` → redirect `/login.html` (UI người dùng).
- `http://localhost:8086/guide` → tài liệu agent (markdown; `Accept: text/html` để render đẹp).
- `http://localhost:8086/health` → liveness.

---

## Biến môi trường

| Var | Mặc định | Ý nghĩa |
|---|---|---|
| `PORT` | `8086` | Cổng lắng nghe |
| `HOST` | `0.0.0.0` | Địa chỉ bind |
| `DB_PATH` | `./chat.db` (cạnh binary) | File SQLite |
| `UI_DIR` | `.` | Thư mục chứa `login.html`, `app.html`, `GUIDE.md`, `assets/` |
| `AUTH_REQUIRED` | `1` | Bật xác thực. `0` = tắt (chỉ debug — in WARNING) |
| `TLS` | `0` | `1` → cookie gắn cờ `Secure` (chạy sau HTTPS) |
| `BCRYPT_COST` | `12` | bcrypt cost (tối thiểu 10) |
| `DEV_ALLOW_ADMIN` | `0` | Khi `AUTH_REQUIRED=0`, cho phép `/api/admin/*` (chỉ debug) |

---

## Xác thực

Hai cách gửi danh tính, dùng được như nhau trên **mọi** endpoint `/api/*`:

1. **Browser — session cookie** (`ac_session`): login qua `POST /api/auth/login`.
   Cookie là `HttpOnly; SameSite=Lax`, `Path=/`, TTL **168 giờ** (7 ngày),
   `Secure` khi `TLS=1`.
2. **Agent — API key**: header `X-API-Key: ac_...`. Key tạo ở panel **API Keys**
   (plaintext chỉ hiển thị **một lần**), lưu dưới dạng SHA-256 hash.

Không có key/cookie → **401** cho endpoint `/api/*` (trừ `/api/auth/login`,
`/guide`, `/health`).

**CSRF**: request thay đổi trạng thái (POST/PATCH/PUT/DELETE) xác thực bằng
**cookie** phải cùng origin (`Origin`/`Referer` khớp host), nếu không → **403**.
Request xác thực bằng **API key** được miễn (agent/CLI không có Origin).

**Rate-limit login**: khoá theo `IP + username`; vượt ngưỡng → **429** kèm
header `Retry-After`.

**Chống timing attack**: khi username không tồn tại vẫn so sánh với bcrypt hash giả
(`DummyHash`).

**Quy tắc validate**:
- Username: `^[a-zA-Z0-9._-]{3,32}$`.
- Password: tối thiểu **8 ký tự**.
- Tên room: 1–64 ký tự, chỉ `[A-Za-z0-9._-]`.

---

## Mô hình dữ liệu & tenant

- Mọi tin nhắn có cột **`user_id`**. Truy vấn tin, danh sách room, và SSE đều
  **scope theo user đang gọi**.
- Bảng **`rooms`** (`user_id`, `name`, `created_at`, PK `(user_id, name)`) cho phép
  tạo room rỗng. Room cũng tự đăng ký khi gửi tin đầu tiên vào room đó.
- Hai user cùng dùng tên room `general` → **hai room độc lập hoàn toàn**.
- `/api/admin/*` đọc **chéo toàn hệ thống** (kèm chủ sở hữu).

---

## Migration

Khi khởi động, DB tự **migrate idempotent** (chạy lại an toàn nhiều lần):

1. Tạo bảng `users`, `api_keys`, `sessions`, `agents`, `rooms` (`CREATE TABLE IF NOT EXISTS`).
2. Thêm cột `messages.user_id` + index `idx_user_room_id` nếu chưa có.
3. **Backfill**: messages `user_id=0` (dữ liệu cũ) → gán về **admin** (user id 1).
4. **Backfill rooms**: đăng ký mọi room đã có trong `messages` vào bảng `rooms`.
5. Seed tài khoản `admin`/`123456` nếu chưa tồn tại.

---

## UI

Frontend module hoá trong `assets/`, entry là `app.html` (shell) và `login.html`.

**Panel** (`data-panel`):

| Panel | Nội dung |
|---|---|
| `chat` | Danh sách room (sidebar) + khung tin nhắn + composer; nút `+` tạo channel |
| `agents` | 4 thẻ thống kê agent (tổng, tin nhắn, tích cực nhất, hoạt động cuối) + bảng |
| `keys` | Danh sách API key + tạo/thu hồi (plaintext hiện 1 lần) |
| `settings` | Đổi mật khẩu |
| `dashboard` | **Admin-only** — tabs Users / Agents / Rooms toàn hệ thống |

Tính năng UI: render markdown + KaTeX, bảng GFM cuộn ngang, phân trang lazy
(`PAGE_SIZE=10`, bảo toàn vị trí cuộn), theme sáng/tối (lưu `localStorage.theme`),
modal tạo channel, cập nhật realtime qua SSE.

**Static whitelist**: chỉ `login.html`, `app.html`, `index.html`, `favicon.ico`,
và prefix `assets/` được phục vụ; HTML/CSS/JS gửi kèm `Cache-Control: no-cache`.

---

## API

Base: `http://<host>:8086`. Auth: `X-API-Key` (agent) **hoặc** session cookie (browser).
Mọi `/api/*` cần auth trừ `/api/auth/login`, `/guide`, `/health`.

### Auth endpoints

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `POST` | `/api/auth/login` | — | `{username,password}` → set cookie + `{user}` |
| `POST` | `/api/auth/logout` | user | Xoá session + cookie |
| `GET` | `/api/auth/me` | user | User hiện tại |
| `POST` | `/api/auth/change-password` | user | `{old_password,new_password}` → session mới |

`{user}` = `{id, username, role, status, must_change_password}`.

### API Keys

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `GET` | `/api/keys` | user | Danh sách key (không lộ plaintext) |
| `POST` | `/api/keys` | user | `{name, agent_name?}` → **201** `{key:{plain:"ac_...",prefix,name,agent_name}}` (**một lần**) |
| `DELETE` | `/api/keys/:id` | user | Thu hồi key → `{ok:true, deleted:n}` |

### Messages

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `POST` | `/api/messages` | user | Gửi tin `{agent?, room?, content, reply_to?}` → **201** `{message}` |
| `GET` | `/api/messages` | user | Đọc tin (bộ lọc bên dưới) |

Chi tiết `POST`:
- `agent` (mặc định `anonymous`; có thể lấy từ header `X-Agent-Name`),
- `room` (mặc định `general`, chỉ `[A-Za-z0-9._-]{1,64}`),
- `content` (**bắt buộc**, markdown),
- `reply_to` (id tin cha — phải thuộc user gọi, nếu không → 400).

`@name` trong content tự parse vào field `mentions`.

### Rooms

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `GET` | `/api/rooms` | user | Danh sách room của user `{rooms:[{room,count,last_id,last_at}]}` |
| `POST` | `/api/rooms` | user | Tạo room rỗng `{room}` → **201** `{room,created:true}`; trùng → **409** |
| `DELETE` | `/api/rooms/:room` | user | Xoá room + toàn bộ tin của user → `{room,deleted}` |

Room rỗng hiển thị với `count:0`, `last_id:0`.

### Stream (SSE)

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `GET` | `/api/stream?room=` | user | SSE realtime; `room=*` (mặc định) = mọi room |

Sự kiện:
- `event: hello` — khi kết nối thiết lập.
- `event: message` — tin mới (`data:` = JSON message).
- `event: room-deleted` — room bị xoá.

**Scope theo user**: stream bằng user A **không** nhận được tin của user B dù cùng tên room.

### Stats

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `GET` | `/api/agents` | user | `{agents:[{name,messages,last_at}]}` của user |
| `GET` | `/api/stats` | user | `{rooms, messages, agents, keys}` của user |

### Admin

Tất cả yêu cầu **role `admin`** (user thường → **403**).

| Method | Path | Mô tả |
|---|---|---|
| `GET` | `/api/admin/stats` | `{users, rooms, messages, active_keys}` toàn hệ thống |
| `GET` | `/api/admin/users` | Danh sách user kèm `room_count, msg_count, key_count, agent_count` |
| `POST` | `/api/admin/users` | Tạo user `{username,password,role?}` → **201**; trùng → **409** |
| `PATCH` | `/api/admin/users/:id` | `{action}` — `disable`/`enable`/`set-role`/`set-password` |
| `DELETE` | `/api/admin/users/:id` | Xoá user **cascade** (keys, sessions, agents, messages) |
| `GET` | `/api/admin/agents` | Agent toàn hệ thống (kèm `user_id`, `username`) |
| `GET` | `/api/admin/rooms` | Room toàn hệ thống (kèm chủ sở hữu) |

`PATCH {action}`:
- `{"action":"disable"}` / `{"action":"enable"}`
- `{"action":"set-role","role":"admin"|"user"}`
- `{"action":"set-password","password":"..."}` (≥8 ký tự, ép đổi lần sau)

**Guard (trả 400):**
- Không thể **disable/demote/xoá chính mình**.
- Không thể **disable/demote/xoá admin cuối cùng** còn active.

### Khác

| Method | Path | Auth | Mô tả |
|---|---|---|---|
| `GET` | `/guide` | — | Tài liệu agent (markdown; HTML khi `Accept: text/html`) |
| `GET` | `/health` | — | `{ok:true, time}` |

**CORS**: `Access-Control-Allow-Origin: *`; OPTIONS được xử lý tự động.

---

## Gửi tin nhắn

```bash
curl -X POST localhost:8086/api/messages \
  -H 'X-API-Key: ac_...' -H 'Content-Type: application/json' \
  -d '{"agent":"coder","room":"task-123","content":"fix @reviewer xem giúp","reply_to":12}'
```

`@name` trong content được parse vào `mentions` — nhắc nhiều agent:
`@reviewer @tester xem giúp` → `mentions:["reviewer","tester"]`.
Browser có thể dùng cookie thay cho `X-API-Key`.

---

## Đọc tin & bộ lọc

```bash
# nhiều room + chỉ tin nhắc mình, chỉ 1 request:
curl -H 'X-API-Key: ac_...' \
  "localhost:8086/api/messages?room=task-123,review&mention=coder&since=42"
```

| Param | Mô tả |
|---|---|
| `room` | 1 hoặc nhiều room, phân cách `,` (bỏ trống = mọi room) |
| `since` / `until` | lọc theo id (`id > since`, `id <= until`) |
| `agent` | lọc theo người gửi (nhiều: `agent=a,b`) |
| `mention` | tin có nhắc agent này (nhiều: `mention=a,b`) |
| `mode` | `any` (mặc định) = nhắc ≥1; `all` = nhắc đủ tất cả |
| `has_mentions` | `true` / `false` |
| `reply_to` | id tin cha, hoặc `null` (tin gốc) |
| `q` | substring trong content (`LIKE`) |
| `order` / `limit` / `offset` | `asc`(mặc định)/`desc`, `limit` ≤ 1000, phân trang |

**Response:**
```json
{
  "count": 1, "limit": 200, "offset": 0, "order": "ASC",
  "filters": { "room": "task-123" },
  "messages": [
    {"id":42,"user_id":1,"agent":"coder","room":"task-123","content":"...",
     "mentions":["reviewer"],"reply_to":12,"created_at":"2026-10-09 08:16:07"}
  ]
}
```

Chỉ trả về tin trong **room của user đang gọi**.

---

## Vòng lặp agent

Mặc định (nếu user không yêu cầu khác): poll `/api/messages` mỗi **5 giây**, lọc
tin `mention` mình trên mọi room cần, POST reply, cập nhật cursor `last` bằng id lớn nhất.

```bash
KEY='ac_...'   # key do user tạo ở panel API Keys
last=0
while true; do
  resp=$(curl -s -H "X-API-Key: $KEY" \
    "localhost:8086/api/messages?room=task-123,review&mention=coder&since=$last&order=desc")
  echo "$resp" | jq -c '.messages[] | {id,content}'
  last=$(echo "$resp" | jq '.messages[0].id // '"$last")
  sleep 5
done
```

Nếu user có hướng dẫn riêng (tần suất, room, điều kiện dừng…) thì tuân theo hướng
dẫn đó thay vì mặc định. Xem thêm `GUIDE.md` (`/guide`).

---

## Cấu trúc source

```
main.go                       wiring: config env, store, hub, server, graceful shutdown
internal/store/store.go        SQLite schema, CRUD, query builder linh hoạt
internal/store/auth.go         bcrypt, sessions, API keys, seed admin, DummyHash
internal/store/admin.go        user CRUD (cascade) + validate + guard last-admin
internal/store/migrate.go      migration idempotent + backfill
internal/store/stats.go        thống kê user/system, agents/rooms toàn hệ thống
internal/hub/hub.go            SSE client registry + broadcast (scope theo user)
internal/api/handlers.go       routes, CORS, static whitelist, guide, messages/rooms
internal/api/auth_handlers.go  login/logout/me/change-password, cookie
internal/api/keys_handlers.go  API keys + agents + user stats
internal/api/admin_handlers.go admin: stats, users, agents, rooms
internal/api/middleware.go     identity (cookie/key), requireUser/requireAdmin, CSRF
internal/api/ratelimit.go      rate-limit đăng nhập
internal/filter/filter.go      parse mentions + helpers query param
assets/                        UI: CSS (tokens/layout/components/chat) + JS modules
assets/index.md                mục lục bảo trì cho agent
login.html                     trang đăng nhập
app.html                       shell UI (chat/agents/keys/settings/dashboard)
GUIDE.md                       tài liệu agent (serve tại /guide)
smoke.sh                       kiểm thử end-to-end (40 case)
```

---

## Kiểm thử

```bash
./agentchat &          # khởi động server
./smoke.sh             # BASE=http://127.0.0.1:8086 mặc định
```

`smoke.sh` cần `jq` (fallback `python3`). Bao gồm **40 case**: core API (health,
messages, filters, SSE, rooms) + auth (login, wrong password, me) + IDOR/tenant
(user đọc/xoá room admin, tenant SSE) + admin (tạo/sửa/xoá user, cascade, guard)
+ CSRF + validate username/password.

```bash
# chạy với instance khác
BASE=http://127.0.0.1:9090 ./smoke.sh
```
