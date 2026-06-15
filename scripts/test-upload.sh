#!/usr/bin/env bash
set -euo pipefail

API="${API:-http://localhost:8080}"
BUCKET="${BUCKET:-uploads}"
FILE="${1:-/tmp/test-image.png}"
W="${W:-320}"
H="${H:-320}"

if [[ ! -f "$FILE" ]]; then
  echo "create a test png first: $FILE"
  exit 1
fi

CT=$(file -b --mime-type "$FILE")
NAME=$(basename "$FILE")

echo "== ensure bucket =="
curl -sf -X POST "$API/storage/v1/bucket" \
  -H 'Content-Type: application/json' \
  -d "{\"id\":\"$BUCKET\",\"name\":\"$BUCKET\",\"public\":true}" || true

echo "== upload (Supabase Storage API) =="
RESP=$(curl -sf -X POST "$API/storage/v1/object/$BUCKET/$NAME" \
  -H "Content-Type: $CT" \
  -H "x-upsert: true" \
  --data-binary @"$FILE")
echo "$RESP" | jq .

echo "== list =="
curl -sf -X POST "$API/storage/v1/object/list/$BUCKET" \
  -H 'Content-Type: application/json' \
  -d '{"prefix":"","limit":10}' | jq .

OUT="/tmp/storage-transform-${NAME}.jpg"
echo "== on-demand transform width=$W height=$H =="
curl -sf "$API/storage/v1/render/image/public/$BUCKET/$NAME?width=$W&height=$H&resize=cover&quality=85" -o "$OUT"
file "$OUT"
echo "saved: $OUT"
