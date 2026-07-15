#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8081}"
USER_NAME="${USER_NAME:-smoke}"
PASSWORD="${PASSWORD:-smoke}"

json() { python3 -m json.tool; }

echo '== health =='
curl -sS "$BASE_URL/health" | json

echo '== register =='
curl -sS -X POST "$BASE_URL/v2/auth/register" \
  -H 'content-type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$PASSWORD\"}" | json || true

echo '== login (mac) =='
MAC=$(curl -sS -X POST "$BASE_URL/v2/auth/login" \
  -H 'content-type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$PASSWORD\",\"client_label\":\"MacBook\"}" )
echo "$MAC" | json
MAC_TOKEN=$(echo "$MAC" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

echo '== login (phone) =='
PHONE=$(curl -sS -X POST "$BASE_URL/v2/auth/login" \
  -H 'content-type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$PASSWORD\",\"client_label\":\"iPhone\"}" )
echo "$PHONE" | json
PHONE_TOKEN=$(echo "$PHONE" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

echo '== create note =='
CREATE=$(curl -sS -X PUT "$BASE_URL/v2/notes/guid-smoke-1" \
  -H "Authorization: Bearer $MAC_TOKEN" \
  -H 'content-type: application/json' \
  -d '{"notetype_id":1,"fields":["front","back"],"tags":["x"],"client_modified_at":"2026-07-10T00:00:00Z","base_checksum":""}')
echo "$CREATE" | json
BASE=$(echo "$CREATE" | python3 -c 'import json,sys; print(json.load(sys.stdin)["checksum"])')

echo '== phone edits from current base =='
curl -sS -X PUT "$BASE_URL/v2/notes/guid-smoke-1" \
  -H "Authorization: Bearer $PHONE_TOKEN" \
  -H 'content-type: application/json' \
  -d "{\"notetype_id\":1,\"fields\":[\"front phone\",\"back\"],\"tags\":[\"x\"],\"client_modified_at\":\"2026-07-10T01:00:00Z\",\"base_checksum\":\"$BASE\"}" | json

echo '== mac stale push -> expect 409 =='
HTTP=$(curl -sS -o /tmp/ks2-conflict.json -w '%{http_code}' -X PUT "$BASE_URL/v2/notes/guid-smoke-1" \
  -H "Authorization: Bearer $MAC_TOKEN" \
  -H 'content-type: application/json' \
  -d "{\"notetype_id\":1,\"fields\":[\"front mac\",\"back\"],\"tags\":[\"x\"],\"client_modified_at\":\"2026-07-10T02:00:00Z\",\"base_checksum\":\"$BASE\"}")
echo "status: $HTTP"
cat /tmp/ks2-conflict.json | json

echo '== mac force override =='
curl -sS -X PUT "$BASE_URL/v2/notes/guid-smoke-1" \
  -H "Authorization: Bearer $MAC_TOKEN" \
  -H 'content-type: application/json' \
  -H 'Force-Override: true' \
  -d "{\"notetype_id\":1,\"fields\":[\"front mac\",\"back\"],\"tags\":[\"x\"],\"client_modified_at\":\"2026-07-10T02:00:00Z\",\"base_checksum\":\"$BASE\"}" | json

echo '== manifest =='
curl -sS "$BASE_URL/v2/sync/manifest" -H "Authorization: Bearer $MAC_TOKEN" | json
