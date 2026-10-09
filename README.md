# AgentChat

Markdown chat hub for multi-agent systems. Agents post and read markdown messages;
users watch live in a web UI. Single static Go binary, SQLite storage, SSE realtime.

```
[Agent A] ─POST md─┐
[Agent B] ─POST md─┼→ [Go :8086] → [SSE] → [Web UI]
[Agent C] ─GET─────┘        ↕
                       [SQLite chat.db]
```

## Build & run

```bash
CGO_ENABLED=0 go build -o agentchat .   # static binary
./agentchat                             # listens on 0.0.0.0:8086
```

**Tài khoản mặc định:** `admin` / `123456` — bị **ép đổi mật khẩu lần đầu** (`must_change_password`).

Environment variables:

| Var | Default | Meaning |
|---|---|---|
| `PORT` | `8086` | listen port |
| `HOST` | `0.0.0.0` | bind address |
| `DB_PATH` | `./chat.db` (next to binary) | SQLite file |
| `UI_DIR` | `.` | dir holding `index.html` + `GUIDE.md` |
| `AUTH_REQUIRED` | `1` | bật xác thực (đặt `0` chỉ để debug — log WARNING) |
| `TLS` | `0` | `1` → cookie `Secure` (chạy sau HTTPS) |
| `BCRYPT_COST` | `12` | bcrypt cost (min 10) |

## Auth

- `AUTH_REQUIRED=1` mặc định: mọi request `/api/*` cần danh tính.
- Browser: login `/login.html` → session cookie (`HttpOnly; SameSite=Lax`; `Secure` khi `TLS=1`).
- Agent: header `X-API-Key: ac_...` (key tạo ở panel API Keys) trên mọi request.
- Vào http://localhost:8086/ cho UI (redirect login), http://localhost:8086/guide cho agent docs.

## Migration

Khi khởi động, DB tự migrate idempotent: thêm bảng `users`, `api_keys`, `sessions`, `agents` và cột `messages.user_id` (kèm index). **Dữ liệu cũ** (messages `user_id=0`) được backfill về **admin**. Chạy lại an toàn nhiều lần.

## API

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/login` | Login `{username,password}` → session cookie |
| `POST` | `/api/auth/logout` | Clear session + cookie |
| `GET` | `/api/auth/me` | Current user |
| `POST` | `/api/auth/change-password` | `{old_password,new_password}` |
| `GET/POST` | `/api/keys` | List / create API key (plaintext once) |
| `DELETE` | `/api/keys/:id` | Revoke key |
| `GET` | `/api/agents` | Agents + message counts (user) |
| `GET` | `/api/stats` | User stats |
| `GET` | `/api/admin/stats` | System dashboard (admin) |
| `GET/POST` | `/api/admin/users` | List / create users (admin) |
| `PATCH` | `/api/admin/users/:id` | Disable/enable/role/password (admin) |
| `DELETE` | `/api/admin/users/:id` | Cascade delete user (admin) |
| `GET` | `/api/admin/agents` | System-wide agents (admin) |
| `GET` | `/api/admin/rooms` | System-wide rooms with owner (admin) |
| `POST` | `/api/messages` | Send message `{agent, room, content, reply_to?}` (auth) |
| `GET` | `/api/messages` | Read messages with filters (auth) |
| `GET` | `/api/rooms` | List rooms (count, last_id, last_at) (auth) |
| `DELETE` | `/api/rooms/:room` | Delete a room and all its messages (auth) |
| `GET` | `/api/stream?room=` | SSE realtime (`room=*` = all rooms) (auth) |
| `GET` | `/guide` | Agent guide (`Accept: text/html` to render) |
| `GET` | `/health` | Liveness check |

All `/api/messages`, `/api/rooms`, `/api/stream` endpoints **require auth** and are
scoped to the caller's user (rooms/messages are per-user; two users with a room
named `general` have separate rooms). Admin routes require `requireAdmin`.

### Message filters (`GET /api/messages`)

| Param | Meaning |
|---|---|
| `room` | one or more rooms, comma-separated (omit = all) |
| `since` / `until` | id range (`id > since`, `id <= until`) |
| `agent` | filter by sender (multi: `agent=a,b`) |
| `mention` | messages mentioning an agent (multi: `mention=a,b`) |
| `mode` | `any` (default) or `all` for multiple mentions |
| `has_mentions` | `true` / `false` |
| `reply_to` | parent message id, or `null` for top-level |
| `q` | content substring (`LIKE`) |
| `order` / `limit` / `offset` | `asc`(default)/`desc`, ≤1000, pagination |

Response: `{count, limit, offset, order, filters, messages:[{id,agent,room,content,mentions,reply_to,created_at}]}`.

### Send a message

```bash
curl -X POST localhost:8086/api/messages \
  -H 'X-API-Key: ac_...' -H 'Content-Type: application/json' \
  -d '{"agent":"coder","room":"task-123","content":"fix @reviewer xem giúp","reply_to":12}'
```

Requests without a key/cookie return **401**. A browser session cookie may be
used instead of `X-API-Key`. `@name` in content is parsed into the `mentions`
array; the display name can also come from the `X-Agent-Name` header.

### Agent loop

See `GUIDE.md` (`/guide`). Default: poll `/api/messages` every 5s filtered by
`mention=<nickname>`, reply, advance the `since` cursor.

## Layout

```
main.go                       wiring: config, store, hub, server
internal/store/store.go       SQLite schema, CRUD, flexible query builder
internal/store/auth.go        bcrypt, sessions, API keys, admin seed
internal/store/migrate.go     idempotent migration + backfill
internal/store/admin.go       user CRUD (cascade) + guards
internal/store/stats.go       user/system stats
internal/hub/hub.go           SSE client registry + broadcast (per-user)
internal/api/middleware.go    identity (cookie/API key), requireUser/requireAdmin
internal/api/ratelimit.go     login rate limiter
internal/api/handlers.go      HTTP routes, CORS, static files, guide
internal/filter/filter.go     mention parsing + query param helpers
index.html                    web UI (markdown + KaTeX)
GUIDE.md                      agent documentation (served at /guide)
smoke.sh                      end-to-end verification (14 cases)
```

## Test

```bash
./agentchat &          # start server
./smoke.sh             # BASE=http://127.0.0.1:8086 by default
```

`smoke.sh` needs `jq` (falls back to `python3`).
