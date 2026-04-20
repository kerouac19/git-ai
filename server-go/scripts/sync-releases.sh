#!/usr/bin/env bash
# Sync git-ai release metadata from GitHub into a private server's file store.
#
# Required environment variables:
#   PRIVATE_SERVER         e.g. https://gitai.example.com
#   RELEASE_UPLOAD_TOKEN   Bearer token accepted by the private server
# Optional:
#   GITHUB_REPO            defaults to git-ai-project/git-ai
#   GITHUB_TOKEN           authenticated GitHub API requests (optional)

set -euo pipefail

: "${PRIVATE_SERVER:?PRIVATE_SERVER must be set}"
: "${RELEASE_UPLOAD_TOKEN:?RELEASE_UPLOAD_TOKEN must be set}"
REPO="${GITHUB_REPO:-git-ai-project/git-ai}"

AUTH_GH=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
    AUTH_GH=(-H "Authorization: Bearer $GITHUB_TOKEN")
fi

put() {
    local url="$1" file="$2" ctype="${3:-application/octet-stream}"
    curl -sSL --fail-with-body -X PUT \
        -H "Authorization: Bearer $RELEASE_UPLOAD_TOKEN" \
        -H "Content-Type: $ctype" \
        --data-binary "@$file" \
        "$url"
}

sync_channel() {
    local channel="$1" api="$2" filter="$3"

    local meta tag published
    meta=$(curl -sfL "${AUTH_GH[@]}" "$api")
    tag=$(jq -r "$filter" <<< "$meta")
    if [ -z "$tag" ] || [ "$tag" = "null" ]; then
        echo "$channel: no release available"
        return 0
    fi

    # Pull matching published_at timestamp when available.
    local published_filter
    published_filter=$(sed 's/\.tag_name/.published_at/' <<< "$filter")
    published=$(jq -r "$published_filter" <<< "$meta" 2>/dev/null || echo "")
    if [ "$published" = "null" ]; then
        published=""
    fi

    # Idempotency: skip when the server already points at the same tag.
    local cur
    cur=$(curl -sfL -H "Authorization: Bearer $RELEASE_UPLOAD_TOKEN" \
            "$PRIVATE_SERVER/api/releases/$channel/current.json" 2>/dev/null \
            | jq -r .tag 2>/dev/null || echo "")
    if [ "$cur" = "$tag" ]; then
        echo "$channel: already at $tag"
        return 0
    fi

    local work
    work=$(mktemp -d)
    trap "rm -rf '$work'" RETURN

    local base="https://github.com/$REPO/releases/download/$tag"
    local f
    for f in SHA256SUMS install.sh install.ps1; do
        curl -sfL -o "$work/$f" "$base/$f"
    done

    for f in SHA256SUMS install.sh install.ps1; do
        put "$PRIVATE_SERVER/api/releases/$channel/artifacts/$tag/$f" "$work/$f"
    done

    local checksum
    checksum=$(sha256sum "$work/SHA256SUMS" | awk '{print $1}')

    cat > "$work/current.json" <<EOF
{"tag":"$tag","checksum":"$checksum","updated_at":"$published"}
EOF

    put "$PRIVATE_SERVER/api/releases/$channel/current.json" \
        "$work/current.json" "application/json"

    echo "$channel: synced ${cur:-<none>} -> $tag"
}

sync_channel latest \
    "https://api.github.com/repos/$REPO/releases/latest" \
    '.tag_name'

sync_channel next \
    "https://api.github.com/repos/$REPO/releases?per_page=20" \
    '[.[] | select(.prerelease)] | first | .tag_name // empty'
