#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:3399}"
# Bypass any local proxy (e.g. Surge)
export no_proxy="*"
PASS=0
FAIL=0
TOTAL=0

red()   { printf "\033[31m%s\033[0m" "$1"; }
green() { printf "\033[32m%s\033[0m" "$1"; }
bold()  { printf "\033[1m%s\033[0m" "$1"; }

assert_status() {
  local label="$1" expected="$2" actual="$3"
  TOTAL=$((TOTAL + 1))
  if [ "$actual" -eq "$expected" ]; then
    PASS=$((PASS + 1))
    printf "  %-50s %s\n" "$label" "$(green "✓ $actual")"
  else
    FAIL=$((FAIL + 1))
    printf "  %-50s %s\n" "$label" "$(red "✗ got $actual, expected $expected")"
  fi
}

assert_json_field() {
  local label="$1" json="$2" field="$3" expected="$4"
  TOTAL=$((TOTAL + 1))
  local actual
  actual=$(echo "$json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d$field)" 2>/dev/null || echo "__MISSING__")
  if [ "$actual" = "$expected" ]; then
    PASS=$((PASS + 1))
    printf "  %-50s %s\n" "$label" "$(green "✓ $field=$actual")"
  else
    FAIL=$((FAIL + 1))
    printf "  %-50s %s\n" "$label" "$(red "✗ $field=$actual, expected $expected")"
  fi
}

assert_json_exists() {
  local label="$1" json="$2" field="$3"
  TOTAL=$((TOTAL + 1))
  local actual
  actual=$(echo "$json" | python3 -c "import sys,json; d=json.load(sys.stdin); v=d$field; print('EXISTS' if v else 'EMPTY')" 2>/dev/null || echo "__MISSING__")
  if [ "$actual" = "EXISTS" ]; then
    PASS=$((PASS + 1))
    printf "  %-50s %s\n" "$label" "$(green "✓ $field exists")"
  else
    FAIL=$((FAIL + 1))
    printf "  %-50s %s\n" "$label" "$(red "✗ $field missing or empty")"
  fi
}

assert_contains() {
  local label="$1" body="$2" expected="$3"
  TOTAL=$((TOTAL + 1))
  if echo "$body" | grep -q "$expected"; then
    PASS=$((PASS + 1))
    printf "  %-50s %s\n" "$label" "$(green "✓ contains '$expected'")"
  else
    FAIL=$((FAIL + 1))
    printf "  %-50s %s\n" "$label" "$(red "✗ missing '$expected'")"
  fi
}

# ─── helper: HTTP request returning "STATUS\nBODY" ───
http() {
  local method="$1" url="$2"
  shift 2
  curl -s -o /tmp/smoke_body -w "%{http_code}" -X "$method" "$url" "$@"
  local code=$?
  local status
  status=$(cat /tmp/smoke_body 2>/dev/null; true)
  # curl -w writes status code to stdout, body to file
  # re-do properly
  true
}

echo ""
bold "═══════════════════════════════════════════════════════════════"
echo ""
bold "  Git-AI Go Server — Smoke Test"
echo ""
bold "  Target: $BASE_URL"
echo ""
bold "═══════════════════════════════════════════════════════════════"
echo ""

# ═══════════════════════════════════════════
bold "① Health Check"
echo ""
# ═══════════════════════════════════════════

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/health")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /health" 200 "$STATUS"
assert_json_field "  status=ok" "$BODY" "['status']" "ok"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/health")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/health" 200 "$STATUS"
assert_json_field "  status=ok" "$BODY" "['status']" "ok"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/health/database")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/health/database" 200 "$STATUS"
assert_json_field "  database=connected" "$BODY" "['database']" "connected"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/status")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/status" 200 "$STATUS"
assert_json_field "  status=ok" "$BODY" "['status']" "ok"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/version")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/version" 200 "$STATUS"
assert_json_field "  version=1.0.0" "$BODY" "['version']" "1.0.0"

echo ""

# ═══════════════════════════════════════════
bold "② OAuth Device Flow"
echo ""
# ═══════════════════════════════════════════

# Start device flow
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST "$BASE_URL/worker/oauth/device/code")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/oauth/device/code" 200 "$STATUS"
assert_json_exists "  device_code present" "$BODY" "['device_code']"
assert_json_exists "  user_code present" "$BODY" "['user_code']"
assert_json_exists "  verification_uri present" "$BODY" "['verification_uri']"

DEVICE_CODE=$(echo "$BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")
USER_CODE=$(echo "$BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['user_code'])")
echo "    device_code=$DEVICE_CODE"
echo "    user_code=$USER_CODE"

# Device info API
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/oauth/device/info?user_code=$USER_CODE")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/oauth/device/info?user_code=..." 200 "$STATUS"
assert_json_field "  status=pending" "$BODY" "['status']" "pending"
assert_json_field "  authenticated=False" "$BODY" "['authenticated']" "False"

# Poll before approval — should get authorization_pending
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d "{\"grant_type\":\"urn:ietf:params:oauth:grant-type:device_code\",\"device_code\":\"$DEVICE_CODE\"}" \
  "$BASE_URL/worker/oauth/token")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/oauth/token (pending)" 400 "$STATUS"
assert_json_field "  error=authorization_pending" "$BODY" "['error']" "authorization_pending"

# install_nonce exchange provides a non-interactive token for the rest of this
# API smoke test. Full device approval requires a browser cookie session and is
# covered by the SPA/device-flow integration path, not this headless script.
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d '{"grant_type":"install_nonce","install_nonce":"test-nonce-123"}' \
  "$BASE_URL/worker/oauth/token")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/oauth/token (install_nonce)" 200 "$STATUS"
assert_json_exists "  access_token present" "$BODY" "['access_token']"
assert_json_exists "  refresh_token present" "$BODY" "['refresh_token']"
assert_json_field "  token_type=Bearer" "$BODY" "['token_type']" "Bearer"

ACCESS_TOKEN=$(echo "$BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
REFRESH_TOKEN=$(echo "$BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['refresh_token'])")
echo "    access_token=${ACCESS_TOKEN:0:20}..."
AUTH_HEADER="Authorization: Bearer $ACCESS_TOKEN"

# Refresh token exchange
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d "{\"grant_type\":\"refresh_token\",\"refresh_token\":\"$REFRESH_TOKEN\"}" \
  "$BASE_URL/worker/oauth/token")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/oauth/token (refresh)" 200 "$STATUS"
assert_json_exists "  new access_token" "$BODY" "['access_token']"

# Deny flow
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST "$BASE_URL/worker/oauth/device/code")
DENY_BODY=$(cat /tmp/smoke_body)
DENY_USER_CODE=$(echo "$DENY_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['user_code'])")
DENY_DEVICE_CODE=$(echo "$DENY_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['device_code'])")
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d "{\"user_code\":\"$DENY_USER_CODE\"}" \
  "$BASE_URL/api/oauth/device/deny")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /api/oauth/device/deny" 200 "$STATUS"
assert_json_field "  status=denied" "$BODY" "['status']" "denied"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d "{\"grant_type\":\"urn:ietf:params:oauth:grant-type:device_code\",\"device_code\":\"$DENY_DEVICE_CODE\"}" \
  "$BASE_URL/worker/oauth/token")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/oauth/token (denied)" 400 "$STATUS"
assert_json_field "  error=access_denied" "$BODY" "['error']" "access_denied"

echo ""

# ═══════════════════════════════════════════
bold "③ JWT Protected Endpoints"
echo ""
# ═══════════════════════════════════════════

AUTH_HEADER="Authorization: Bearer $ACCESS_TOKEN"

# /api/me without token → 401
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/me")
assert_status "GET /api/me (no token) → 401" 401 "$STATUS"

# /api/me with token → 200
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/me")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/me (with token)" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"
assert_json_exists "  user.id present" "$BODY" "['user']['id']"

echo ""

# ═══════════════════════════════════════════
bold "④ CAS Upload & Read"
echo ""
# ═══════════════════════════════════════════

# Upload via worker endpoint (JSON objects)
CAS_HASH="deadbeef0123456789abcdef0123456789abcdef0123456789abcdef01234567"
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d "{\"objects\":[{\"hash\":\"$CAS_HASH\",\"content\":{\"prompt\":\"hello world\",\"model\":\"gpt-4\"}}]}" \
  "$BASE_URL/worker/cas/upload")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/cas/upload (JSON)" 200 "$STATUS"
assert_json_field "  success_count=1" "$BODY" "['success_count']" "1"

# Read back via worker endpoint
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" \
  "$BASE_URL/worker/cas?hashes=$CAS_HASH")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /worker/cas?hashes=..." 200 "$STATUS"
assert_json_field "  success_count=1" "$BODY" "['success_count']" "1"

# Verify content roundtrip
CAS_CONTENT=$(echo "$BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['results'][0]['content']['prompt'])" 2>/dev/null || echo "__MISSING__")
TOTAL=$((TOTAL + 1))
if [ "$CAS_CONTENT" = "hello world" ]; then
  PASS=$((PASS + 1))
  printf "  %-50s %s\n" "  CAS content roundtrip" "$(green "✓ prompt=hello world")"
else
  FAIL=$((FAIL + 1))
  printf "  %-50s %s\n" "  CAS content roundtrip" "$(red "✗ got $CAS_CONTENT")"
fi

# Checkout single object
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" \
  "$BASE_URL/worker/cas/checkout?hash=$CAS_HASH")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /worker/cas/checkout?hash=..." 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

# /api/cas/* requires worker auth (JWT or X-API-Key). Confirm 401 first.
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d '{"content":"unauthenticated","contentType":"text/plain"}' \
  "$BASE_URL/api/cas/upload")
assert_status "POST /api/cas/upload (no auth) → 401" 401 "$STATUS"

# Upload via /api/cas/upload with auth
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"content":"test plain text content","contentType":"text/plain"}' \
  "$BASE_URL/api/cas/upload")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /api/cas/upload" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"
API_CAS_HASH=$(echo "$BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['hash'])")

# Read back via /api/cas/read/:hash (auth required)
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/cas/read/$API_CAS_HASH")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/cas/read/:hash" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

echo ""

# ═══════════════════════════════════════════
bold "⑤ Metrics Upload"
echo ""
# ═══════════════════════════════════════════

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -H "x-distinct-id: test-machine-001" \
  -d '{
    "v": 1,
    "events": [
      {"t": 1712000000, "e": 1, "v": {"2": 100, "5": [42], "10": 1711999000, "11": "feat: smoke", "12": "sync latest metrics schema", "13": "authorship note", "14": "[]"}, "a": {"0": "1.2.8", "1": "https://github.com/test/repo", "2": "dev@example.com", "3": "abc123", "4": "base456", "5": "main", "20": "claude-code", "21": "gpt-5.4", "23": "external-session-123", "24": "session-123", "25": "trace-123", "30": "{\"workspace\":\"smoke\"}"}},
      {"t": 1712000001, "e": 2, "v": {}, "a": {"0": "1.2.8", "20": "cursor", "21": "claude-3.5", "23": "external-session-456", "24": "session-456", "30": "{\"source\":\"smoke-test\"}"}},
      {"t": 1712000002, "e": 4, "v": {"0": 1712000002, "1": "ai_agent", "2": "src/main.rs", "3": 5, "4": 1, "5": 4, "6": 1, "7": "tool-use-1", "8": "file_edit"}, "a": {"0": "0.1.0", "23": "external-session-789", "24": "session-789"}},
      {"t": 1712000003, "e": 5, "v": {"0": {"type": "message", "text": "hello"}, "1": "event-1", "2": "parent-event-1", "3": "tool-use-1"}, "a": {"0": "1.4.7", "20": "claude-code", "21": "gpt-5.4", "23": "external-session-789", "24": "session-789", "25": "trace-789"}}
    ]
  }' \
  "$BASE_URL/worker/metrics/upload")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /worker/metrics/upload (4 events)" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

# Also test /workers/* plural path
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"v": 1, "events": [{"t": 1712000003, "e": 1, "v": {"2": 50}, "a": {"0": "0.1.0"}}]}' \
  "$BASE_URL/workers/metrics/upload")
assert_status "POST /workers/metrics/upload (plural)" 200 "$STATUS"

echo ""

# ═══════════════════════════════════════════
bold "⑥ Dashboard Stats"
echo ""
# ═══════════════════════════════════════════

USER_ROLE=$(echo "$ACCESS_TOKEN" | python3 -c "
import sys,json,base64
token = sys.stdin.read().strip().split('.')[1]
token += '=' * (4 - len(token) % 4)
print(json.loads(base64.b64decode(token)).get('role', ''))
")

# Dashboard stats require auth; query ?userId= is ignored server-side.
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/dashboard/stats")
assert_status "GET /api/dashboard/stats (no auth) → 401" 401 "$STATUS"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/dashboard/stats")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/dashboard/stats (with token)" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/dashboard/public")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/dashboard/public" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/dashboard/global")
assert_status "GET /api/dashboard/global (no auth) → 401" 401 "$STATUS"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/dashboard/global")
BODY=$(cat /tmp/smoke_body)
if [ "$USER_ROLE" = "admin" ]; then
  assert_status "GET /api/dashboard/global (admin token)" 200 "$STATUS"
  assert_json_field "  success=True" "$BODY" "['success']" "True"
else
  assert_status "GET /api/dashboard/global (non-admin token) → 403" 403 "$STATUS"
fi

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"range":"7d","format":"json"}' \
  "$BASE_URL/api/dashboard/generate-report")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /api/dashboard/generate-report → 501" 501 "$STATUS"
assert_json_field "  error=not_implemented" "$BODY" "['error']" "not_implemented"

echo ""

# ═══════════════════════════════════════════
bold "⑧ Config CRUD (JWT protected)"
echo ""
# ═══════════════════════════════════════════

# Without token → 401
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/config")
assert_status "GET /api/config (no token) → 401" 401 "$STATUS"

# Create config
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"key":"test.setting","value":"hello","description":"test config","category":"general","is_sensitive":false}' \
  "$BASE_URL/api/config")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /api/config (create)" 201 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

# Create sensitive config
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"key":"test.secret","value":"super-secret-value","description":"a secret","category":"security","is_sensitive":true}' \
  "$BASE_URL/api/config")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /api/config (sensitive)" 201 "$STATUS"

# Get all configs
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/config")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/config (list)" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

# Get specific config
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/config/test.setting")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/config/test.setting" 200 "$STATUS"

# Get sensitive config — value should be [REDACTED]
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" "$BASE_URL/api/config/test.secret")
BODY=$(cat /tmp/smoke_body)
assert_status "GET /api/config/test.secret" 200 "$STATUS"
assert_json_field "  value=[REDACTED]" "$BODY" "['data']['value']" "[REDACTED]"

# Update config
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X PATCH \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"value":"updated-value"}' \
  "$BASE_URL/api/config/test.setting")
BODY=$(cat /tmp/smoke_body)
assert_status "PATCH /api/config/test.setting" 200 "$STATUS"

# Delete config
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X DELETE \
  -H "$AUTH_HEADER" "$BASE_URL/api/config/test.setting")
assert_status "DELETE /api/config/test.setting" 200 "$STATUS"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X DELETE \
  -H "$AUTH_HEADER" "$BASE_URL/api/config/test.secret")
assert_status "DELETE /api/config/test.secret" 200 "$STATUS"

echo ""

# ═══════════════════════════════════════════
bold "⑨ Device Flow API Edge Cases"
echo ""
# ═══════════════════════════════════════════

# SPA routes such as /me and /oauth/device are served by nginx/static hosting
# in production. This script targets the Go API process directly.
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" "$BASE_URL/api/oauth/device/info")
assert_status "GET /api/oauth/device/info (no code) → 400" 400 "$STATUS"

echo ""

# ═══════════════════════════════════════════
bold "⑩ Edge Cases & Error Handling"
echo ""
# ═══════════════════════════════════════════

# Invalid grant_type
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d '{"grant_type":"invalid"}' \
  "$BASE_URL/worker/oauth/token")
assert_status "POST /worker/oauth/token (bad grant) → 400" 400 "$STATUS"

# Metrics: bad schema version
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"v": 99, "events": []}' \
  "$BASE_URL/worker/metrics/upload")
assert_status "POST metrics (bad version) → 400" 400 "$STATUS"

# CAS: read non-existent hash (auth required)
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -H "$AUTH_HEADER" \
  "$BASE_URL/api/cas/read/nonexistent")
assert_status "GET /api/cas/read/nonexistent → 404" 404 "$STATUS"

# Bundles: auth required
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "Content-Type: application/json" \
  -d '{"title":"smoke","data":{"prompts":{"p1":{}}}}' \
  "$BASE_URL/api/bundles")
assert_status "POST /api/bundles (no auth) → 401" 401 "$STATUS"

STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST \
  -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"title":"smoke","data":{"prompts":{"p1":{}}}}' \
  "$BASE_URL/api/bundles")
BODY=$(cat /tmp/smoke_body)
assert_status "POST /api/bundles (with auth)" 200 "$STATUS"
assert_json_field "  success=True" "$BODY" "['success']" "True"

# Config: delete non-existent
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X DELETE \
  -H "$AUTH_HEADER" "$BASE_URL/api/config/nonexistent.key")
assert_status "DELETE /api/config/nonexistent → 500" 500 "$STATUS"

# Workers plural path
STATUS=$(curl -s -o /tmp/smoke_body -w "%{http_code}" -X POST "$BASE_URL/workers/oauth/device/code")
assert_status "POST /workers/oauth/device/code (plural)" 200 "$STATUS"

echo ""

# ═══════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════
echo ""
bold "═══════════════════════════════════════════════════════════════"
echo ""
if [ $FAIL -eq 0 ]; then
  green "  ALL $TOTAL TESTS PASSED ✓"
else
  red "  $FAIL / $TOTAL TESTS FAILED"
  echo ""
  green "  $PASS passed"
fi
echo ""
bold "═══════════════════════════════════════════════════════════════"
echo ""

exit $FAIL
