#!/bin/bash

set -euo pipefail

SERVER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_ENV_FILE="$SERVER_DIR/.env.local-https.example"
ENV_FILE="${ENV_FILE:-$DEFAULT_ENV_FILE}"
WATCH_MODE="${WATCH_MODE:-0}"
SKIP_BUILD="${SKIP_BUILD:-0}"

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "Error: missing required command: $1" >&2
        exit 1
    fi
}

is_truthy() {
    case "${1:-}" in
        1|true|TRUE|yes|YES|on|ON) return 0 ;;
        *) return 1 ;;
    esac
}

load_env_file() {
    if [[ ! -f "$ENV_FILE" ]]; then
        echo "Error: env file not found: $ENV_FILE" >&2
        exit 1
    fi

    set -a
    # shellcheck disable=SC1090
    source "$ENV_FILE"
    set +a
}

ensure_certificates() {
    local key_path cert_path
    key_path="${DEV_HTTPS_KEY_PATH:-certs/localhost-key.pem}"
    cert_path="${DEV_HTTPS_CERT_PATH:-certs/localhost.pem}"

    if [[ "$key_path" != /* ]]; then
        key_path="$SERVER_DIR/$key_path"
    fi

    if [[ "$cert_path" != /* ]]; then
        cert_path="$SERVER_DIR/$cert_path"
    fi

    export DEV_HTTPS_KEY_PATH="$key_path"
    export DEV_HTTPS_CERT_PATH="$cert_path"

    mkdir -p "$(dirname "$key_path")" "$(dirname "$cert_path")"

    if [[ -f "$key_path" && -f "$cert_path" ]]; then
        echo "Using existing HTTPS certificate:"
        echo "  cert: $cert_path"
        echo "  key:  $key_path"
        return
    fi

    require_cmd mkcert

    echo "Generating local HTTPS certificate with mkcert..."
    mkcert -install
    mkcert \
        -key-file "$key_path" \
        -cert-file "$cert_path" \
        git-ai.localhost localhost 127.0.0.1 ::1
}

main() {
    require_cmd pnpm
    load_env_file

    export DEV_HTTPS="${DEV_HTTPS:-true}"
    export HTTPS_REDIRECT="${HTTPS_REDIRECT:-false}"
    export PORT="${PORT:-3443}"

    if ! is_truthy "$DEV_HTTPS"; then
        echo "Error: DEV_HTTPS must be enabled in $ENV_FILE for this script." >&2
        exit 1
    fi

    ensure_certificates

    echo "Starting Git-AI server with local HTTPS..."
    echo "  env:  $ENV_FILE"
    echo "  url:  https://git-ai.localhost:$PORT"

    cd "$SERVER_DIR"

    if ! is_truthy "$SKIP_BUILD"; then
        pnpm build
    fi

    if is_truthy "$WATCH_MODE"; then
        exec pnpm start:dev
    fi

    exec pnpm start
}

main "$@"
