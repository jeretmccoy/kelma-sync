#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8081}"
USER_NAME="${USER_NAME:-demo}"
PASSWORD="${PASSWORD:-demo}"
CLIENT_LABEL="${CLIENT_LABEL:-Seeder}"

curl_json() {
  curl -sS "$@" | python3 -m json.tool
}

# register may already exist; ignore failures
curl -sS -X POST "$BASE_URL/v2/auth/register" \
  -H 'content-type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$PASSWORD\"}" >/dev/null || true

TOKEN=$(curl -sS -X POST "$BASE_URL/v2/auth/login" \
  -H 'content-type: application/json' \
  -d "{\"username\":\"$USER_NAME\",\"password\":\"$PASSWORD\",\"client_label\":\"$CLIENT_LABEL\"}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

echo "token acquired"

curl_json -X PUT "$BASE_URL/v2/notetypes/1" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"name":"Basic","definition":{"fields":["Front","Back"],"templates":[{"name":"Card 1"}]},"client_modified_at":"2026-07-10T00:00:00Z","base_checksum":""}'

echo
curl_json -X PUT "$BASE_URL/v2/decks/Default" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"config":{"new_per_day":20,"reviews_per_day":200},"client_modified_at":"2026-07-10T00:00:00Z","base_checksum":""}'

echo
curl_json -X PUT "$BASE_URL/v2/notes/guid-seed-1" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"notetype_id":1,"fields":["hello","world"],"tags":["seed"],"client_modified_at":"2026-07-10T00:00:00Z","base_checksum":""}'

echo
curl_json -X PUT "$BASE_URL/v2/cards/1001" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' \
  -d '{"note_guid":"guid-seed-1","deck_name":"Default","ord":0,"scheduling":{"due":1,"interval":0,"reps":0},"client_modified_at":"2026-07-10T00:00:00Z"}'

echo
curl_json "$BASE_URL/v2/sync/manifest" -H "Authorization: Bearer $TOKEN"
