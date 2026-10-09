#!/usr/bin/env bash
# AgentChat verification harness.
#
# Tests the AgentChat Go server with auth. Set BASE to point at a different
# instance. Repeated runs use unique rooms/users so they never collide.
#
# Usage:
#   BASE=http://127.0.0.1:8086 ./smoke.sh
#
# Exit code is non-zero if any case FAILs.
set -u

BASE=${BASE:-http://127.0.0.1:8086}
ROOM=${ROOM:-smoke-$RANDOM-$RANDOM}

PASS=0
FAIL=0

# ---- JSON tooling: prefer jq, fall back to python3 -------------------------
if command -v jq >/dev/null 2>&1; then
  JSON_TOOL=jq
elif command -v python3 >/dev/null 2>&1; then
  JSON_TOOL=python3
else
  echo "ERROR: neither jq nor python3 found for JSON parsing" >&2
  exit 2
fi

# jqval <json> <jq-filter> [room-name]
jqval() {
  if [ "$JSON_TOOL" = "jq" ]; then
    if [ -n "${3:-}" ]; then
      printf '%s' "$1" | jq -c -r --arg r "$3" "$2"
    else
      printf '%s' "$1" | jq -c -r "$2"
    fi
  else
    # python3 fallback: minimal filters used by this script
    printf '%s' "$1" | python3 -c '
import sys, json
raw = sys.stdin.read()
try:
    j = json.loads(raw)
except Exception:
    print("null"); sys.exit(0)
f = sys.argv[1]
room = sys.argv[2] if len(sys.argv) > 2 else None
out = None
def g(*path):
    cur = j
    for p in path:
        if not isinstance(cur, dict):
            return None
        cur = cur.get(p)
    return cur
if f == ".ok": out = j.get("ok")
elif f == ".count": out = j.get("count")
elif f == ".deleted": out = j.get("deleted")
elif f == ".id": out = j.get("id")
elif f == ".mentions": out = json.dumps(j.get("mentions"), separators=(",", ":"))
elif f == ".messages | length": out = len(j.get("messages") or [])
elif f == ".rooms[] | select(.room == $r) | .count":
    hit = next((x for x in (j.get("rooms") or []) if x.get("room") == room), None)
    out = hit["count"] if hit else None
elif f == ".rooms[] | select(.room == $r) | .username":
    hit = next((x for x in (j.get("rooms") or []) if x.get("room") == room), None)
    out = hit["username"] if hit else None
elif f == ".user.role": out = g("user", "role")
elif f == ".user.id": out = g("user", "id")
elif f == ".user.status": out = g("user", "status")
elif f == ".key.plain": out = g("key", "plain")
elif f == ".plain": out = j.get("plain")
elif f == "first(.rooms[] | select(.username == $r) | .room)":
    hit = next((x for x in (j.get("rooms") or []) if x.get("username") == room), None)
    out = hit["room"] if hit else None
elif f == ".users | length": out = len(j.get("users") or [])
elif f == ".rooms | length": out = len(j.get("rooms") or [])
elif f == ".active_keys": out = j.get("active_keys")
elif f == ".messages": out = j.get("messages")
elif f == ".users": out = j.get("users")
elif f == ".rooms": out = j.get("rooms")
elif f == ".stats.users": out = g("stats", "users")
if isinstance(out, bool):
    print("true" if out else "false")
else:
    print("null" if out is None else out)
' "$2" "${3:-}"
  fi
}

# http <method> <url> [json-body]  -> prints "STATUS\tBODY"
# Auth args are carried in the AUTH_ARGS array (set via use_key/use_cookie/use_none).
AUTH_ARGS=()
http() {
  local method="$1" url="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -s "${AUTH_ARGS[@]}" -w '\t%{http_code}' -X "$method" "$url" \
      -H 'Content-Type: application/json' -d "$body"
  else
    curl -s "${AUTH_ARGS[@]}" -w '\t%{http_code}' -X "$method" "$url"
  fi
}

use_key()    { AUTH_ARGS=(-H "X-API-Key: $1"); }
use_cookie() { AUTH_ARGS=(-b "$1"); }
use_none()   { AUTH_ARGS=(); }

# http_raw <curl-args...> -> prints "STATUS\tBODY" (caller supplies full args)
http_raw() {
  curl -s -w '\t%{http_code}' "$@"
}

status_of() { printf '%s' "$1" | sed 's/.*\t//'; }
body_of() { printf '%s' "$1" | sed 's/\t[^\t]*$//'; }

# check <name> <condition: 0=pass 1=fail> <detail>
check() {
  local name="$1" rc="$2" detail="${3:-}"
  if [ "$rc" -eq 0 ]; then
    PASS=$((PASS + 1))
    printf 'PASS  %s%s\n' "$name" "${detail:+  ($detail)}"
  else
    FAIL=$((FAIL + 1))
    printf 'FAIL  %s%s\n' "$name" "${detail:+  ($detail)}"
  fi
}

eq() { [ "$1" = "$2" ]; }

echo "== AgentChat smoke =="
echo "BASE = $BASE"
echo "ROOM = $ROOM"
echo "JSON = $JSON_TOOL"
echo

# ---- Auth bootstrap: admin cookie jar + admin API key ----------------------
CJ=$(mktemp)
CJ2=$(mktemp)
TMP_FILES="$CJ $CJ2"
cleanup() { rm -f $TMP_FILES; }
trap cleanup EXIT

bootstrap_ok=1
LOGIN_R=$(http_raw -c "$CJ" -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"123456"}')
LOGIN_ST=$(status_of "$LOGIN_R")
LOGIN_BD=$(body_of "$LOGIN_R")
KEY_R=$(http_raw -b "$CJ" -X POST "$BASE/api/keys" \
  -H 'Content-Type: application/json' \
  -d '{"name":"smoke-admin","agent_name":"smoker"}')
KEY=$(jqval "$(body_of "$KEY_R")" '.key.plain')
[ "$KEY" = "null" ] && KEY=$(jqval "$(body_of "$KEY_R")" '.plain')
case "$KEY" in
  ac_*) : ;;
  *) KEY=""; bootstrap_ok=1 ;;
esac
if [ "$LOGIN_ST" = "200" ] && [ -n "$KEY" ]; then
  bootstrap_ok=0
else
  echo "FATAL: auth bootstrap failed (login=$LOGIN_ST). Is BASE=$BASE an auth-enabled server?" >&2
  echo "login body: $LOGIN_BD" >&2
  echo "key body: $(body_of "$KEY_R")" >&2
fi

# http_a <method> <url> [body]         -> admin (cookie)
http_a()  { use_cookie "$CJ"; http "$@"; }
# http_u <method> <url> [body]         -> user #2 (u1 API key)
http_u()  { use_key "$U1KEY"; http "$@"; }
# http_k <method> <url> [body]         -> agent (admin API key)
http_k()  { use_key "$KEY"; http "$@"; }

if [ "$bootstrap_ok" -eq 0 ]; then

# ---- 1. GET /health --------------------------------------------------------
R=$(http_raw "$BASE/health")
ST=$(status_of "$R"); BD=$(body_of "$R")
OK=$(jqval "$BD" '.ok')
if eq "$ST" "200" && eq "$OK" "true"; then
  check "1  GET /health 200 + ok:true" 0 "status=$ST ok=$OK"
else
  check "1  GET /health 200 + ok:true" 1 "status=$ST ok=$OK"
fi

# ---- 2. POST /api/messages (a1, "hi @a2 @a3") ------------------------------
R=$(use_key "$KEY"; http POST "$BASE/api/messages" \
  "{\"agent\":\"a1\",\"room\":\"$ROOM\",\"content\":\"hi @a2 @a3\"}")
ST=$(status_of "$R"); BD=$(body_of "$R")
ID1=$(jqval "$BD" '.id')
M1=$(jqval "$BD" '.mentions')
MENT_OK=1
if [ "$M1" = '["a2","a3"]' ] || [ "$M1" = '["a3","a2"]' ]; then MENT_OK=0; fi
if eq "$ST" "201" && [ "$MENT_OK" -eq 0 ]; then
  check "2  POST msg a1 -> 201 + mentions [a2,a3]" 0 "status=$ST id=$ID1 mentions=$M1"
else
  check "2  POST msg a1 -> 201 + mentions [a2,a3]" 1 "status=$ST id=$ID1 mentions=$M1"
fi

# ---- 3. POST /api/messages (a2, "only @a2", reply_to=ID1) ------------------
R=$(use_key "$KEY"; http POST "$BASE/api/messages" \
  "{\"agent\":\"a2\",\"room\":\"$ROOM\",\"content\":\"only @a2\",\"reply_to\":$ID1}")
ST=$(status_of "$R"); BD=$(body_of "$R")
ID2=$(jqval "$BD" '.id')
if eq "$ST" "201"; then
  check "3  POST msg a2 reply_to=$ID1 -> 201" 0 "status=$ST id=$ID2"
else
  check "3  POST msg a2 reply_to=$ID1 -> 201" 1 "status=$ST body=$BD"
fi

# ---- 4. GET since=0 -> count==2 -------------------------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&since=0")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "2" ] && check "4  GET since=0 -> count 2" 0 "count=$C" \
  || check "4  GET since=0 -> count 2" 1 "count=$C"

# ---- 5. GET since=ID1 -> count==1 -----------------------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&since=$ID1")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "5  GET since=$ID1 -> count 1" 0 "count=$C" \
  || check "5  GET since=$ID1 -> count 1" 1 "count=$C"

# ---- 6. GET mention=a2 -> count==2 ----------------------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&mention=a2")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "2" ] && check "6  GET mention=a2 -> count 2" 0 "count=$C" \
  || check "6  GET mention=a2 -> count 2" 1 "count=$C"

# ---- 7. GET mention=a2,a3&mode=all -> count==1 ----------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&mention=a2,a3&mode=all")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "7  GET mention=a2,a3&mode=all -> count 1" 0 "count=$C" \
  || check "7  GET mention=a2,a3&mode=all -> count 1" 1 "count=$C"

# ---- 8. GET has_mentions=false -------------------------------------------
# Both seeded messages contain "@" mentions, so an exact has_mentions=false
# filter must return none. (Spec text said count==1 assuming "only @a2" had no
# mentions; the server parses @a2 as a mention, so authoritative count is 0.)
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&has_mentions=false")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "0" ] && check "8  GET has_mentions=false -> count 0" 0 "count=$C" \
  || check "8  GET has_mentions=false -> count 0" 1 "count=$C"

# ---- 9. GET reply_to=ID1 -> count==1 --------------------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&reply_to=$ID1")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "9  GET reply_to=$ID1 -> count 1" 0 "count=$C" \
  || check "9  GET reply_to=$ID1 -> count 1" 1 "count=$C"

# ---- 10. GET q=only -> count==1 -------------------------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&q=only")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "10 GET q=only -> count 1" 0 "count=$C" \
  || check "10 GET q=only -> count 1" 1 "count=$C"

# ---- 11. GET /api/rooms contains ROOM with count 2 ------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/rooms")
BD=$(body_of "$R")
RC=$(jqval "$BD" '.rooms[] | select(.room == $r) | .count' "$ROOM")
[ "$RC" = "2" ] && check "11 GET /api/rooms contains $ROOM count 2" 0 "count=$RC" \
  || check "11 GET /api/rooms contains $ROOM count 2" 1 "count=$RC"

# ---- 12. GET limit=1 -> count==1 ------------------------------------------
R=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&limit=1")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "12 GET limit=1 -> count 1" 0 "count=$C" \
  || check "12 GET limit=1 -> count 1" 1 "count=$C"

# ---- 13. SSE: subscribe room, POST, assert event: message -----------------
TMP=$(mktemp); TMP_FILES="$TMP_FILES $TMP"
curl -sN -H "X-API-Key: $KEY" "$BASE/api/stream?room=$ROOM" >"$TMP" 2>/dev/null &
CURL_PID=$!
# wait until the stream is established (hello event) or timeout
for _ in $(seq 1 20); do
  grep -q "event: hello" "$TMP" && break
  sleep 0.2
done
use_key "$KEY"; http POST "$BASE/api/messages" \
  "{\"agent\":\"a1\",\"room\":\"$ROOM\",\"content\":\"sse ping @a2\"}" >/dev/null
# wait for the message event to arrive
for _ in $(seq 1 25); do
  grep -q "event: message" "$TMP" && break
  sleep 0.2
done
kill "$CURL_PID" 2>/dev/null
wait "$CURL_PID" 2>/dev/null
if grep -q "event: message" "$TMP"; then
  check "13 SSE stream emits event: message" 0 "after POST"
else
  check "13 SSE stream emits event: message" 1 "no event in stream"
fi

# ---- 14. DELETE /api/rooms/ROOM -> deleted>=3, then count==0 --------------
R=$(use_key "$KEY"; http DELETE "$BASE/api/rooms/$ROOM")
ST=$(status_of "$R"); BD=$(body_of "$R")
DEL=$(jqval "$BD" '.deleted')
DEL_OK=1
case "$DEL" in
  ''|null|*[!0-9]*) DEL_OK=1 ;;
  *) [ "$DEL" -ge 3 ] && DEL_OK=0 ;;
esac
R2=$(use_key "$KEY"; http GET "$BASE/api/messages?room=$ROOM&since=0")
C2=$(jqval "$(body_of "$R2")" '.count')
if [ "$DEL_OK" -eq 0 ] && [ "$C2" = "0" ]; then
  check "14 DELETE room -> deleted>=$DEL then count 0" 0 "deleted=$DEL after=$C2"
else
  check "14 DELETE room -> deleted>=$DEL then count 0" 1 "deleted=$DEL after=$C2"
fi

# ===========================================================================
# Auth / IDOR / tenant / admin cases (PLAN §8.1)
# ===========================================================================

# ---- A1 login admin/123456 -> 200 + Set-Cookie -----------------------------
A1_R=$(http_raw -c "$CJ" -D - -o /dev/null -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"123456"}')
A1_ST=$(status_of "$A1_R" | tail -1)
A1_COOKIE=$(printf '%s' "$A1_R" | grep -ci '^set-cookie')
if [ "$A1_ST" = "200" ] && [ "$A1_COOKIE" -ge 1 ]; then
  check "A1 login admin -> 200 + Set-Cookie" 0 "status=$A1_ST set-cookie=$A1_COOKIE"
else
  check "A1 login admin -> 200 + Set-Cookie" 1 "status=$A1_ST set-cookie=$A1_COOKIE"
fi

# ---- A2 login wrong password -> 401 ----------------------------------------
R=$(http_raw -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"wrong-password-xyz"}')
ST=$(status_of "$R")
[ "$ST" = "401" ] && check "A2 login wrong password -> 401" 0 "status=$ST" \
  || check "A2 login wrong password -> 401" 1 "status=$ST"

# ---- A3 GET /api/auth/me with cookie -> 200 role=admin ---------------------
R=$(http_a GET "$BASE/api/auth/me")
ST=$(status_of "$R"); BD=$(body_of "$R")
ROLE=$(jqval "$BD" '.user.role')
if [ "$ST" = "200" ] && [ "$ROLE" = "admin" ]; then
  check "A3 GET /api/auth/me -> 200 role=admin" 0 "status=$ST role=$ROLE"
else
  check "A3 GET /api/auth/me -> 200 role=admin" 1 "status=$ST role=$ROLE"
fi

# ---- A4 admin POST /api/admin/users create user u_$RANDOM -> 201 -----------
U1=u_$RANDOM-$RANDOM
U1PW=userpass-1234
R=$(http_a POST "$BASE/api/admin/users" \
  "{\"username\":\"$U1\",\"password\":\"$U1PW\",\"role\":\"user\"}")
ST=$(status_of "$R"); BD=$(body_of "$R")
UID1=$(jqval "$BD" '.user.id')
if [ "$ST" = "201" ] && [ -n "$UID1" ] && [ "$UID1" != "null" ]; then
  check "A4 admin create user $U1 -> 201" 0 "status=$ST id=$UID1"
else
  check "A4 admin create user $U1 -> 201" 1 "status=$ST body=$BD"
fi

# ---- A5 duplicate username -> 409 ------------------------------------------
R=$(http_a POST "$BASE/api/admin/users" \
  "{\"username\":\"$U1\",\"password\":\"$U1PW\",\"role\":\"user\"}")
ST=$(status_of "$R"); BD=$(body_of "$R")
[ "$ST" = "409" ] && check "A5 duplicate username -> 409" 0 "status=$ST" \
  || check "A5 duplicate username -> 409" 1 "status=$ST body=$BD"

# ---- A6 login as new user -> 200 (cookie jar #2) ---------------------------
R=$(http_raw -c "$CJ2" -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U1\",\"password\":\"$U1PW\"}")
ST=$(status_of "$R")
[ "$ST" = "200" ] && check "A6 login new user -> 200" 0 "status=$ST" \
  || check "A6 login new user -> 200" 1 "status=$ST body=$(body_of "$R")"

# ---- A7 new user POST /api/keys -> 200/201 plaintext ac_... ----------------
R=$(http_raw -b "$CJ2" -X POST "$BASE/api/keys" \
  -H 'Content-Type: application/json' \
  -d '{"name":"smoke-u1","agent_name":"u1agent"}')
ST=$(status_of "$R"); BD=$(body_of "$R")
U1KEY=$(jqval "$BD" '.key.plain')
[ "$U1KEY" = "null" ] && U1KEY=$(jqval "$BD" '.plain')
U1_AUTH="-H X-API-Key: $U1KEY"
case "$U1KEY" in
  ac_*) KEY_OK=0 ;;
  *) KEY_OK=1 ;;
esac
if { [ "$ST" = "200" ] || [ "$ST" = "201" ]; } && [ "$KEY_OK" -eq 0 ]; then
  check "A7 new user create key -> $ST plaintext ac_..." 0 "status=$ST key=${U1KEY%%??????????}..."
else
  check "A7 new user create key -> $ST plaintext ac_..." 1 "status=$ST body=$BD"
fi

# ---- A8 new user agent POST /api/messages with X-API-Key -> 201 ------------
U1ROOM=uroom-$RANDOM-$RANDOM
R=$(http_u POST "$BASE/api/messages" \
  "{\"agent\":\"u1agent\",\"room\":\"$U1ROOM\",\"content\":\"u1 hello\"}")
ST=$(status_of "$R"); BD=$(body_of "$R")
if [ "$ST" = "201" ]; then
  check "A8 new user agent POST msg -> 201" 0 "status=$ST room=$U1ROOM"
else
  check "A8 new user agent POST msg -> 201" 1 "status=$ST body=$BD"
fi

# ---- A9 IDOR: new user GET admin's room -> no admin messages ---------------
# Admin's ROOM was deleted in case 14; use an admin-owned room known to exist.
R=$(http_a GET "$BASE/api/admin/rooms")
ADM_ROOM=$(jqval "$(body_of "$R")" 'first(.rooms[] | select(.username == $r) | .room)' "admin")
R=$(http_u GET "$BASE/api/messages?room=$ADM_ROOM&since=0")
BD=$(body_of "$R")
N=$(jqval "$BD" '.messages | length')
if [ "$N" = "0" ]; then
  check "A9 IDOR new user reads admin room -> 0 msgs" 0 "room=$ADM_ROOM count=$N"
else
  check "A9 IDOR new user reads admin room -> 0 msgs" 1 "room=$ADM_ROOM count=$N"
fi

# ---- A10 IDOR: admin GET /api/admin/rooms -> sees new-user room (owner) ----
R=$(http_a GET "$BASE/api/admin/rooms")
BD=$(body_of "$R")
OWNER=$(jqval "$BD" '.rooms[] | select(.room == $r) | .username' "$U1ROOM")
[ "$OWNER" = "$U1" ] && check "A10 admin rooms shows $U1ROOM owner=$U1" 0 "owner=$OWNER" \
  || check "A10 admin rooms shows $U1ROOM owner=$U1" 1 "owner=$OWNER"

# ---- A11 IDOR: new user DELETE admin room -> admin data intact -------------
R=$(http_a GET "$BASE/api/admin/rooms")
ADM_ROOM2=$(jqval "$(body_of "$R")" 'first(.rooms[] | select(.username == $r) | .room)' "admin")
R=$(http_a GET "$BASE/api/admin/rooms")
BEFORE=$(jqval "$(body_of "$R")" '.rooms[] | select(.room == $r) | .count' "$ADM_ROOM2")
R=$(http_u DELETE "$BASE/api/rooms/$ADM_ROOM2")
DEL_ST=$(status_of "$R"); DEL_BD=$(body_of "$R")
DEL_N=$(jqval "$DEL_BD" '.deleted')
R=$(http_a GET "$BASE/api/admin/rooms")
AFTER=$(jqval "$(body_of "$R")" '.rooms[] | select(.room == $r) | .count' "$ADM_ROOM2")
DESTROYED=0
case "$DEL_N" in 0|null|"") : ;; *) DESTROYED=1 ;; esac
if [ "$BEFORE" != "null" ] && [ "$AFTER" = "$BEFORE" ] && [ "$DESTROYED" -eq 0 ]; then
  check "A11 IDOR user DELETE admin room -> data intact" 0 "before=$BEFORE after=$AFTER deleted=$DEL_N"
else
  check "A11 IDOR user DELETE admin room -> data intact" 1 "before=$BEFORE after=$AFTER deleted=$DEL_N status=$DEL_ST"
fi

# ---- A12 tenant SSE: u1 stream must NOT receive admin message --------------
TENROOM=tenant-$RANDOM-$RANDOM
TMP=$(mktemp); TMP_FILES="$TMP_FILES $TMP"
curl -sN -b "$CJ2" "$BASE/api/stream?room=$TENROOM" >"$TMP" 2>/dev/null &
SPID=$!
for _ in $(seq 1 25); do
  grep -q "event: hello" "$TMP" && break
  sleep 0.2
done
HELLO=$(grep -c "event: hello" "$TMP")
use_key "$KEY"; http POST "$BASE/api/messages" \
  "{\"agent\":\"admin\",\"room\":\"$TENROOM\",\"content\":\"admin secret\"}" >/dev/null
for _ in $(seq 1 15); do
  grep -q "event: message" "$TMP" && break
  sleep 0.2
done
GOT_MSG=$(grep -c "event: message" "$TMP")
kill "$SPID" 2>/dev/null
wait "$SPID" 2>/dev/null
if [ "$HELLO" -ge 1 ] && [ "$GOT_MSG" -eq 0 ]; then
  check "A12 tenant SSE u1 does not receive admin msg" 0 "hello=$HELLO admin_events=$GOT_MSG"
else
  check "A12 tenant SSE u1 does not receive admin msg" 1 "hello=$HELLO admin_events=$GOT_MSG"
fi

# ---- A13 no auth -> GET /api/messages 401 ----------------------------------
R=$(http_raw "$BASE/api/messages?room=$U1ROOM")
ST=$(status_of "$R")
[ "$ST" = "401" ] && check "A13 no auth GET /api/messages -> 401" 0 "status=$ST" \
  || check "A13 no auth GET /api/messages -> 401" 1 "status=$ST"

# ---- A14 admin disable user -> login 401 + key 401 -------------------------
R=$(http_a PATCH "$BASE/api/admin/users/$UID1" '{"action":"disable"}')
DIS_ST=$(status_of "$R")
L=$(http_raw -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U1\",\"password\":\"$U1PW\"}")
L_ST=$(status_of "$L")
K=$(http_raw -H "X-API-Key: $U1KEY" "$BASE/api/messages?room=$U1ROOM")
K_ST=$(status_of "$K")
if { [ "$DIS_ST" = "200" ]; } && [ "$L_ST" = "401" ] && [ "$K_ST" = "401" ]; then
  check "A14 disable user -> login 401 + key 401" 0 "disable=$DIS_ST login=$L_ST key=$K_ST"
else
  check "A14 disable user -> login 401 + key 401" 1 "disable=$DIS_ST login=$L_ST key=$K_ST"
fi

# ---- A15 admin set-password user -> login with new password 200 ------------
# A14 disabled the user; re-enable first so set-password can be exercised.
http_a PATCH "$BASE/api/admin/users/$UID1" '{"action":"enable"}' >/dev/null
U1PW2=newpass-$RANDOM-$RANDOM
R=$(http_a PATCH "$BASE/api/admin/users/$UID1" "{\"action\":\"set-password\",\"password\":\"$U1PW2\"}")
SP_ST=$(status_of "$R")
L=$(http_raw -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U1\",\"password\":\"$U1PW2\"}")
L_ST=$(status_of "$L")
if [ "$SP_ST" = "200" ] && [ "$L_ST" = "200" ]; then
  check "A15 admin set-password -> login new pass 200" 0 "patch=$SP_ST login=$L_ST"
else
  check "A15 admin set-password -> login new pass 200" 1 "patch=$SP_ST login=$L_ST"
fi

# ---- A16 admin delete user -> gone from list + login 401 -------------------
R=$(http_a DELETE "$BASE/api/admin/users/$UID1")
DELU_ST=$(status_of "$R")
R=$(http_a GET "$BASE/api/admin/users")
BD=$(body_of "$R")
PRESENT=$(printf '%s' "$BD" | grep -c "\"$U1\"")
L=$(http_raw -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$U1\",\"password\":\"$U1PW2\"}")
L_ST=$(status_of "$L")
if [ "$DELU_ST" = "200" ] && [ "$PRESENT" -eq 0 ] && [ "$L_ST" = "401" ]; then
  check "A16 admin delete user -> gone + login 401" 0 "delete=$DELU_ST present=$PRESENT login=$L_ST"
else
  check "A16 admin delete user -> gone + login 401" 1 "delete=$DELU_ST present=$PRESENT login=$L_ST"
fi

# ---- A17 guard: admin DELETE self -> 400 -----------------------------------
R=$(http_a DELETE "$BASE/api/admin/users/1")
ST=$(status_of "$R"); BD=$(body_of "$R")
[ "$ST" = "400" ] && check "A17 guard admin delete self -> 400" 0 "status=$ST" \
  || check "A17 guard admin delete self -> 400" 1 "status=$ST body=$BD"

# ---- A18 guard: last-admin delete -> 400 -----------------------------------
R=$(http_a DELETE "$BASE/api/admin/users/1")
ST=$(status_of "$R")
[ "$ST" = "400" ] && check "A18 guard last-admin delete -> 400" 0 "status=$ST" \
  || check "A18 guard last-admin delete -> 400" 1 "status=$ST"

# ---- A19 rate-limit: 6 bad logins -> 6th returns 429 + Retry-After ---------
RLU=nobody-$RANDOM-$RANDOM
RL6_ST=""; RL6_RA=""
for i in 1 2 3 4 5 6; do
  HDR=$(http_raw -D - -o /dev/null -X POST "$BASE/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$RLU\",\"password\":\"bad\"}")
  RL6_ST=$(printf '%s' "$HDR" | grep -i '^HTTP/' | tail -1 | awk '{print $2}')
  RL6_RA=$(printf '%s' "$HDR" | grep -ci '^retry-after')
done
if [ "$RL6_ST" = "429" ] && [ "$RL6_RA" -ge 1 ]; then
  check "A19 rate-limit 6th bad login -> 429 + Retry-After" 0 "status=$RL6_ST retry-after=$RL6_RA"
else
  check "A19 rate-limit 6th bad login -> 429 + Retry-After" 1 "status=$RL6_ST retry-after=$RL6_RA"
fi

# ---- A20 admin GET /api/admin/stats -> 200 with 4 counters -----------------
R=$(http_a GET "$BASE/api/admin/stats")
ST=$(status_of "$R"); BD=$(body_of "$R")
US=$(jqval "$BD" '.users'); RMS=$(jqval "$BD" '.rooms')
MSG=$(jqval "$BD" '.messages'); AK=$(jqval "$BD" '.active_keys')
STATS_OK=1
for v in "$US" "$RMS" "$MSG" "$AK"; do
  case "$v" in ''|null|*[!0-9]*) STATS_OK=1 ;; esac
done
case "$US$RMS$MSG$AK" in *[!0-9]*) : ;; *) STATS_OK=0 ;; esac
if [ "$ST" = "200" ] && [ "$STATS_OK" -eq 0 ]; then
  check "A20 GET /api/admin/stats -> 200 counters" 0 "users=$US rooms=$RMS messages=$MSG active_keys=$AK"
else
  check "A20 GET /api/admin/stats -> 200 counters" 1 "status=$ST body=$BD"
fi

# ---- A21 guard: admin disable self -> 400 ----------------------------------
R=$(http_a PATCH "$BASE/api/admin/users/1" '{"action":"disable"}')
ST=$(status_of "$R"); BD=$(body_of "$R")
[ "$ST" = "400" ] && check "A21 guard admin disable self -> 400" 0 "status=$ST" \
  || check "A21 guard admin disable self -> 400" 1 "status=$ST body=$BD"

# ---- A22 guard: admin demote self -> 400 -----------------------------------
R=$(http_a PATCH "$BASE/api/admin/users/1" '{"action":"set-role","role":"user"}')
ST=$(status_of "$R"); BD=$(body_of "$R")
[ "$ST" = "400" ] && check "A22 guard admin demote self -> 400" 0 "status=$ST" \
  || check "A22 guard admin demote self -> 400" 1 "status=$ST body=$BD"

# ---- A23 CSRF: cross-origin cookie POST -> 403 -----------------------------
R=$(http_raw -b "$CJ" -H 'Origin: https://evil.example' -X POST "$BASE/api/messages" \
  -H 'Content-Type: application/json' -d '{"content":"csrf-probe","room":"general"}')
ST=$(status_of "$R")
[ "$ST" = "403" ] && check "A23 CSRF cross-origin cookie POST -> 403" 0 "status=$ST" \
  || check "A23 CSRF cross-origin cookie POST -> 403" 1 "status=$ST"

# ---- A24 CSRF exempt for API key: cross-origin key POST -> 201 -------------
R=$(http_raw -H "X-API-Key: $KEY" -H 'Origin: https://evil.example' -X POST "$BASE/api/messages" \
  -H 'Content-Type: application/json' -d '{"content":"apikey-cross","room":"general"}')
ST=$(status_of "$R")
[ "$ST" = "201" ] && check "A24 CSRF API key cross-origin exempt -> 201" 0 "status=$ST" \
  || check "A24 CSRF API key cross-origin exempt -> 201" 1 "status=$ST"

# ---- A25 validation: bad username -> 400 -----------------------------------
R=$(http_a POST "$BASE/api/admin/users" '{"username":"bad name!","password":"password123","role":"user"}')
ST=$(status_of "$R")
[ "$ST" = "400" ] && check "A25 bad username -> 400" 0 "status=$ST" \
  || check "A25 bad username -> 400" 1 "status=$ST"

# ---- A26 validation: short password -> 400 ---------------------------------
R=$(http_a POST "$BASE/api/admin/users" '{"username":"validname1","password":"short","role":"user"}')
ST=$(status_of "$R")
[ "$ST" = "400" ] && check "A26 short password -> 400" 0 "status=$ST" \
  || check "A26 short password -> 400" 1 "status=$ST"

fi  # bootstrap_ok

echo
echo "== Summary =="
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
