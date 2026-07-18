#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://localhost:9090}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:?Set ADMIN_PASSWORD to the seeded administrator password}"
CLIENT_ID="${CLIENT_ID:?Set CLIENT_ID from an app credential}"
CLIENT_SECRET="${CLIENT_SECRET:?Set CLIENT_SECRET from an app credential}"
NONCE="${NONCE:-$(date +%s)}"
command -v curl >/dev/null && command -v jq >/dev/null || { echo 'curl and jq are required' >&2; exit 2; }
api() { local m=$1 p=$2 t=${3:-} b=${4:-}; curl -fsS -X "$m" "$BASE_URL$p" ${t:+-H "Authorization: Bearer $t"} ${b:+-H 'Content-Type: application/json' -d "$b"}; }
token() { curl -fsS -X POST "$BASE_URL$1" -H 'Content-Type: application/x-www-form-urlencoded' "${@:2}"; }
section() { printf '\n==> %s\n' "$*"; }

section health
api GET /healthz | jq -e '.ok'
section admin-auth
ADMIN=$(token /api/admin/token --data-urlencode grant_type=password --data-urlencode "username=$ADMIN_EMAIL" --data-urlencode "password=$ADMIN_PASSWORD")
AT=$(jq -r .access_token <<<"$ADMIN"); AR=$(jq -r '.refresh_token // empty' <<<"$ADMIN")
[[ -n $AT && $AT != null ]]
[[ -z $AR ]] || token /api/admin/token/refresh --data-urlencode grant_type=refresh_token --data-urlencode "refresh_token=$AR" | jq -e .access_token >/dev/null
for path in me system/info system/health workers runtime/providers health/providers balances/providers; do api GET "/api/admin/$path" "$AT" | jq -e . >/dev/null; done

section app-auth
APP=$(token /api/v1/token --data-urlencode grant_type=client_credentials --data-urlencode "client_id=$CLIENT_ID" --data-urlencode "client_secret=$CLIENT_SECRET")
PT=$(jq -r .access_token <<<"$APP"); [[ -n $PT && $PT != null ]]
route_count=$(api GET /api/admin/routes "$AT" | jq '[.data.items[] | select(.active and .payment_method=="momo")] | length')
if (( route_count )); then
  body=$(jq -nc --arg ref "SMOKE-COLL-$NONCE" '{payment_method:"momo",amount:5000,currency:"UGX",country:"UG",reference:$ref,customer:{name:"Smoke",phone:"256771111111"},momo:{phone:"256771111111",network:"airtel"}}')
  create() { curl -fsS -X POST "$BASE_URL/api/v1/collections" -H "Authorization: Bearer $PT" -H 'Content-Type: application/json' -H "Idempotency-Key: smoke-$NONCE" -d "$body"; }
  first=$(create); id=$(jq -r .data.transaction_id <<<"$first"); [[ $id == "$(create | jq -r .data.transaction_id)" ]]
  api GET "/api/v1/transactions/$id" "$PT" | jq -e . >/dev/null
  api GET "/api/v1/transactions/by-reference/SMOKE-COLL-$NONCE" "$PT" | jq -e . >/dev/null
else
  echo 'no active momo route; payment call skipped'
fi

section credential-lifecycle
app=$(api POST /api/admin/apps "$AT" "{\"name\":\"Smoke $NONCE\",\"environment\":\"sandbox\"}")
aid=$(jq -r .data.id <<<"$app")
cred=$(api POST "/api/admin/apps/$aid/credentials" "$AT" '{"name":"Smoke","scopes":"collections:create transactions:read"}')
cid=$(jq -r .data.credential.id <<<"$cred"); ckey=$(jq -r .data.credential.client_id <<<"$cred"); secret=$(jq -r .data.client_secret <<<"$cred")
token /api/v1/token --data-urlencode grant_type=client_credentials --data-urlencode "client_id=$ckey" --data-urlencode "client_secret=$secret" | jq -e .access_token >/dev/null
api PATCH "/api/admin/apps/$aid/credentials/$cid/revoke" "$AT" | jq -e . >/dev/null
code=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/api/v1/token" -d "grant_type=client_credentials&client_id=$ckey&client_secret=$secret")
[[ $code == 400 || $code == 401 ]]
echo 'backend API smoke suite passed'
