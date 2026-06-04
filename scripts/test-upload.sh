#!/usr/bin/env bash
set -euo pipefail

API="${API:-http://localhost:8080}"
BUCKET="${BUCKET:-uploads}"
FILE="${1:-/tmp/test-image.png}"
W="${W:-320}"
H="${H:-320}"
C="${C:-fill}"

if [[ ! -f "$FILE" ]]; then
  echo "create a test png first: $FILE"
  exit 1
fi

CT=$(file -b --mime-type "$FILE")
NAME=$(basename "$FILE")

echo "== presign =="
RESP=$(curl -sf -X POST "$API/storage/v1/buckets/$BUCKET/uploads/presign" \
  -H 'Content-Type: application/json' \
  -d "{\"object_name\":\"$NAME\",\"content_type\":\"$CT\"}")
echo "$RESP" | jq .

TOKEN=$(echo "$RESP" | jq -r .complete_token)
URL=$(echo "$RESP" | jq -r .presigned_url)

echo "== put to s3 =="
curl -sf -X PUT "$URL" -H "Content-Type: $CT" --data-binary @"$FILE"

echo "== complete =="
OBJ=$(curl -sf -X POST "$API/storage/v1/buckets/$BUCKET/uploads/complete" \
  -H 'Content-Type: application/json' \
  -d "{\"complete_token\":\"$TOKEN\"}")
echo "$OBJ" | jq .

OID=$(echo "$OBJ" | jq -r .id)
OUT="/tmp/storage-transform-${OID}.jpg"
echo "== on-demand transform w=$W h=$H c=$C =="
curl -sf "$API/storage/v1/objects/$OID/image?w=$W&h=$H&c=$C&q=85" -o "$OUT"
file "$OUT"
echo "saved: $OUT"
