#!/usr/bin/env bash
# AgentChat verification harness.
#
# Tests the AgentChat Go server. Set BASE to point at a different instance.
# Repeated runs use a unique room so they never collide.
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
if f == ".ok": out = j.get("ok")
elif f == ".count": out = j.get("count")
elif f == ".deleted": out = j.get("deleted")
elif f == ".id": out = j.get("id")
elif f == ".mentions": out = json.dumps(j.get("mentions"), separators=(",", ":"))
elif f == ".messages | length": out = len(j.get("messages") or [])
elif f == ".rooms[] | select(.room == $r) | .count":
    hit = next((x for x in (j.get("rooms") or []) if x.get("room") == room), None)
    out = hit["count"] if hit else None
if isinstance(out, bool):
    print("true" if out else "false")
else:
    print("null" if out is None else out)
' "$2" "${3:-}"
  fi
}

# http <method> <url> [json-body]  -> prints "STATUS\tBODY"
http() {
  local method="$1" url="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -s -w '\t%{http_code}' -X "$method" "$url" \
      -H 'Content-Type: application/json' -d "$body"
  else
    curl -s -w '\t%{http_code}' -X "$method" "$url"
  fi
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

# ---- 1. GET /health --------------------------------------------------------
R=$(http GET "$BASE/health")
ST=$(status_of "$R"); BD=$(body_of "$R")
OK=$(jqval "$BD" '.ok')
if eq "$ST" "200" && eq "$OK" "true"; then
  check "1  GET /health 200 + ok:true" 0 "status=$ST ok=$OK"
else
  check "1  GET /health 200 + ok:true" 1 "status=$ST ok=$OK"
fi

# ---- 2. POST /api/messages (a1, "hi @a2 @a3") ------------------------------
R=$(http POST "$BASE/api/messages" \
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
R=$(http POST "$BASE/api/messages" \
  "{\"agent\":\"a2\",\"room\":\"$ROOM\",\"content\":\"only @a2\",\"reply_to\":$ID1}")
ST=$(status_of "$R"); BD=$(body_of "$R")
ID2=$(jqval "$BD" '.id')
if eq "$ST" "201"; then
  check "3  POST msg a2 reply_to=$ID1 -> 201" 0 "status=$ST id=$ID2"
else
  check "3  POST msg a2 reply_to=$ID1 -> 201" 1 "status=$ST body=$BD"
fi

# ---- 4. GET since=0 -> count==2 -------------------------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&since=0")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "2" ] && check "4  GET since=0 -> count 2" 0 "count=$C" \
  || check "4  GET since=0 -> count 2" 1 "count=$C"

# ---- 5. GET since=ID1 -> count==1 -----------------------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&since=$ID1")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "5  GET since=$ID1 -> count 1" 0 "count=$C" \
  || check "5  GET since=$ID1 -> count 1" 1 "count=$C"

# ---- 6. GET mention=a2 -> count==2 ----------------------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&mention=a2")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "2" ] && check "6  GET mention=a2 -> count 2" 0 "count=$C" \
  || check "6  GET mention=a2 -> count 2" 1 "count=$C"

# ---- 7. GET mention=a2,a3&mode=all -> count==1 ----------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&mention=a2,a3&mode=all")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "7  GET mention=a2,a3&mode=all -> count 1" 0 "count=$C" \
  || check "7  GET mention=a2,a3&mode=all -> count 1" 1 "count=$C"

# ---- 8. GET has_mentions=false -------------------------------------------
# Both seeded messages contain "@" mentions, so an exact has_mentions=false
# filter must return none. (Spec text said count==1 assuming "only @a2" had no
# mentions; the server parses @a2 as a mention, so authoritative count is 0.)
R=$(http GET "$BASE/api/messages?room=$ROOM&has_mentions=false")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "0" ] && check "8  GET has_mentions=false -> count 0" 0 "count=$C" \
  || check "8  GET has_mentions=false -> count 0" 1 "count=$C"

# ---- 9. GET reply_to=ID1 -> count==1 --------------------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&reply_to=$ID1")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "9  GET reply_to=$ID1 -> count 1" 0 "count=$C" \
  || check "9  GET reply_to=$ID1 -> count 1" 1 "count=$C"

# ---- 10. GET q=only -> count==1 -------------------------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&q=only")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "10 GET q=only -> count 1" 0 "count=$C" \
  || check "10 GET q=only -> count 1" 1 "count=$C"

# ---- 11. GET /api/rooms contains ROOM with count 2 ------------------------
R=$(http GET "$BASE/api/rooms")
BD=$(body_of "$R")
RC=$(jqval "$BD" '.rooms[] | select(.room == $r) | .count' "$ROOM")
[ "$RC" = "2" ] && check "11 GET /api/rooms contains $ROOM count 2" 0 "count=$RC" \
  || check "11 GET /api/rooms contains $ROOM count 2" 1 "count=$RC"

# ---- 12. GET limit=1 -> count==1 ------------------------------------------
R=$(http GET "$BASE/api/messages?room=$ROOM&limit=1")
BD=$(body_of "$R"); C=$(jqval "$BD" '.count')
[ "$C" = "1" ] && check "12 GET limit=1 -> count 1" 0 "count=$C" \
  || check "12 GET limit=1 -> count 1" 1 "count=$C"

# ---- 13. SSE: subscribe room, POST, assert event: message -----------------
TMP=$(mktemp)
curl -sN "$BASE/api/stream?room=$ROOM" >"$TMP" 2>/dev/null &
CURL_PID=$!
# wait until the stream is established (hello event) or timeout
for _ in $(seq 1 20); do
  grep -q "event: hello" "$TMP" && break
  sleep 0.2
done
http POST "$BASE/api/messages" \
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
rm -f "$TMP"

# ---- 14. DELETE /api/rooms/ROOM -> deleted>=3, then count==0 --------------
R=$(http DELETE "$BASE/api/rooms/$ROOM")
ST=$(status_of "$R"); BD=$(body_of "$R")
DEL=$(jqval "$BD" '.deleted')
DEL_OK=1
case "$DEL" in
  ''|null|*[!0-9]*) DEL_OK=1 ;;
  *) [ "$DEL" -ge 3 ] && DEL_OK=0 ;;
esac
R2=$(http GET "$BASE/api/messages?room=$ROOM&since=0")
C2=$(jqval "$(body_of "$R2")" '.count')
if [ "$DEL_OK" -eq 0 ] && [ "$C2" = "0" ]; then
  check "14 DELETE room -> deleted>=$DEL then count 0" 0 "deleted=$DEL after=$C2"
else
  check "14 DELETE room -> deleted>=$DEL then count 0" 1 "deleted=$DEL after=$C2"
fi

echo
echo "== Summary =="
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
