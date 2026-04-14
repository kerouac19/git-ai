#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ADMIN_USERNAME=admin ADMIN_PASSWORD='...' server-go/scripts/register-users.sh users.csv [base_url]

Arguments:
  users.csv   CSV file with header: username,password,email,display_name
              email and display_name may be empty.
  base_url    Optional. Defaults to http://127.0.0.1:3000

Example users.csv:
  username,password,email,display_name
  alice,change-me-123,alice@example.com,Alice
  bob,change-me-456,bob@example.com,Bob
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  usage
  exit 0
fi

CSV_FILE="${1:-}"
BASE_URL="${2:-http://127.0.0.1:3000}"

if [ -z "$CSV_FILE" ]; then
  usage >&2
  exit 2
fi

if [ ! -f "$CSV_FILE" ]; then
  echo "CSV file not found: $CSV_FILE" >&2
  exit 2
fi

if [ -z "${ADMIN_USERNAME:-}" ] || [ -z "${ADMIN_PASSWORD:-}" ]; then
  echo "ADMIN_USERNAME and ADMIN_PASSWORD must be set." >&2
  exit 2
fi

command -v curl >/dev/null || { echo "curl is required." >&2; exit 2; }
command -v python3 >/dev/null || { echo "python3 is required." >&2; exit 2; }

# Bypass local HTTP proxies that can interfere with localhost/private deployments.
export no_proxy="${no_proxy:-*}"

login_body="$(mktemp)"
register_body="$(mktemp)"
payloads_file="$(mktemp)"
cleanup() {
  rm -f "$login_body" "$register_body" "$payloads_file"
}
trap cleanup EXIT

login_payload="$(
  python3 - <<'PY'
import json
import os

print(json.dumps({
    "username": os.environ["ADMIN_USERNAME"],
    "password": os.environ["ADMIN_PASSWORD"],
}))
PY
)"

login_status="$(
  curl -sS -o "$login_body" -w "%{http_code}" \
    -X POST "$BASE_URL/api/user/login" \
    -H "Content-Type: application/json" \
    -d "$login_payload"
)"

if [ "$login_status" != "200" ]; then
  echo "Admin login failed with HTTP $login_status:" >&2
  cat "$login_body" >&2
  echo >&2
  exit 1
fi

TOKEN="$(
  python3 - "$login_body" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    data = json.load(f)
token = data.get("access_token", "")
if not token:
    raise SystemExit("missing access_token in login response")
print(token)
PY
)"

echo "Admin login ok. Registering users from $CSV_FILE ..."

CSV_FILE="$CSV_FILE" python3 - <<'PY' > "$payloads_file"
import csv
import json
import os
import sys

required = {"username", "password"}
optional = {"email", "display_name"}
path = os.environ["CSV_FILE"]

with open(path, newline="", encoding="utf-8-sig") as f:
    reader = csv.DictReader(f)
    fields = set(reader.fieldnames or [])
    missing = required - fields
    if missing:
        print(f"CSV header missing required column(s): {', '.join(sorted(missing))}", file=sys.stderr)
        raise SystemExit(2)

    for line_no, row in enumerate(reader, start=2):
        username = (row.get("username") or "").strip()
        password = row.get("password") or ""
        if not username and not password:
            continue
        if not username or not password:
            print(f"Skipping line {line_no}: username and password are required", file=sys.stderr)
            continue

        payload = {
            "username": username,
            "password": password,
        }
        for key in optional:
            value = (row.get(key) or "").strip()
            if value:
                payload[key] = value
        print(json.dumps(payload, ensure_ascii=False))
PY

success=0
failed=0

while IFS= read -r payload; do
  username="$(
    PAYLOAD="$payload" python3 - <<'PY'
import json
import os

print(json.loads(os.environ["PAYLOAD"]).get("username", ""))
PY
  )"

  status="$(
    curl -sS -o "$register_body" -w "%{http_code}" \
      -X POST "$BASE_URL/api/user/register" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$payload"
  )"

  if [ "$status" = "201" ]; then
    success=$((success + 1))
    printf "  %-32s created\n" "$username"
  else
    failed=$((failed + 1))
    printf "  %-32s failed HTTP %s: " "$username" "$status"
    python3 - "$register_body" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as f:
        data = json.load(f)
    print(data.get("message") or data.get("error") or data)
except Exception:
    with open(sys.argv[1], encoding="utf-8") as f:
        print(f.read().strip())
PY
  fi
done < "$payloads_file"

echo "Done. success=$success failed=$failed"

if [ "$failed" -gt 0 ]; then
  exit 1
fi
