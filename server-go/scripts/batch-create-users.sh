#!/usr/bin/env bash
#
# 批量创建用户脚本（纯 bash，无 python 依赖）
#
# 用法:
#   ADMIN_USERNAME=admin ADMIN_PASSWORD='...' bash batch-create-users.sh users.csv [base_url]
#
# CSV 格式（第一行为表头）:
#   username,password,email,display_name
#   alice,change-me-123,alice@example.com,Alice
#   bob,,bob@example.com,Bob           ← 密码为空时自动生成
#
# 可选环境变量:
#   AUTO_PASSWORD_LEN  自动生成密码长度（默认 16）
#   DRY_RUN=1          仅打印将要创建的用户，不实际调用 API
#
set -euo pipefail

# ─────────────────── 配置 ───────────────────
CSV_FILE="${1:-}"
BASE_URL="${2:-https://gitai.tongbaninfo.com/}"
AUTO_PASSWORD_LEN="${AUTO_PASSWORD_LEN:-16}"

# ─────────────────── 颜色 ───────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[ OK ]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()  { echo -e "${RED}[FAIL]${NC} $*"; }
die()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ─────────────────── 帮助 ───────────────────
usage() {
    cat <<'EOF'
用法:
  ADMIN_USERNAME=admin ADMIN_PASSWORD='...' bash batch-create-users.sh users.csv [base_url]

参数:
  users.csv   CSV 文件，表头: username,password,email,display_name
              password 为空时自动生成随机密码
  base_url    可选，默认 https://gitai.tongbaninfo.com

环境变量:
  ADMIN_USERNAME     管理员用户名（必填）
  ADMIN_PASSWORD     管理员密码（必填）
  AUTO_PASSWORD_LEN  自动生成密码长度（默认 16）
  DRY_RUN=1          仅预览，不实际创建

CSV 示例:
  username,password,email,display_name
  alice,change-me-123,alice@example.com,Alice
  bob,,bob@example.com,Bob
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
fi

# ─────────────────── 校验 ───────────────────
[[ -n "$CSV_FILE" ]]                         || { usage >&2; exit 2; }
[[ -f "$CSV_FILE" ]]                         || die "CSV 文件不存在: $CSV_FILE"
[[ -n "${ADMIN_USERNAME:-}" ]]               || die "请设置 ADMIN_USERNAME 环境变量"
[[ -n "${ADMIN_PASSWORD:-}" ]]               || die "请设置 ADMIN_PASSWORD 环境变量"
command -v curl >/dev/null                   || die "需要 curl"

# 绕过本地代理
export no_proxy="${no_proxy:-*}"

# ─────────────────── 工具函数 ───────────────────

# 生成随机密码
gen_password() {
    LC_ALL=C tr -dc 'A-Za-z0-9!@#$%&*' </dev/urandom | head -c "${AUTO_PASSWORD_LEN}"
}

# JSON 转义字符串
json_escape() {
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    s="${s//$'\r'/\\r}"
    s="${s//$'\t'/\\t}"
    printf '%s' "$s"
}

# 从 JSON 响应中提取字段值（简易解析，无 jq 依赖）
json_value() {
    local key="$1" body="$2"
    # 匹配 "key":"value" 或 "key": "value"
    echo "$body" | grep -o "\"${key}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed 's/.*:[ ]*"//;s/"$//'
}

# ─────────────────── 登录获取 Token ───────────────────
info "正在以管理员 ${ADMIN_USERNAME} 登录 ${BASE_URL} ..."

login_payload=$(printf '{"username":"%s","password":"%s"}' \
    "$(json_escape "$ADMIN_USERNAME")" \
    "$(json_escape "$ADMIN_PASSWORD")")

login_resp_file="$(mktemp)"
trap 'rm -f "$login_resp_file"' EXIT

login_status=$(curl -sS -o "$login_resp_file" -w "%{http_code}" \
    -X POST "${BASE_URL}/api/user/login" \
    -H "Content-Type: application/json" \
    -d "$login_payload")

if [[ "$login_status" != "200" ]]; then
    die "管理员登录失败 (HTTP $login_status): $(cat "$login_resp_file")"
fi

TOKEN=$(json_value "access_token" "$(cat "$login_resp_file")")
[[ -n "$TOKEN" ]] || die "登录响应中未找到 access_token"

ok "管理员登录成功"

# ─────────────────── 解析 CSV 表头 ───────────────────
header_line=$(head -1 "$CSV_FILE" | tr -d '\r' | sed 's/^[[:space:]]*//')

# 解析列索引
col_username=-1
col_password=-1
col_email=-1
col_display_name=-1

IFS=',' read -ra headers <<< "$header_line"
for i in "${!headers[@]}"; do
    col=$(echo "${headers[$i]}" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
    case "$col" in
        username)     col_username=$i ;;
        password)     col_password=$i ;;
        email)        col_email=$i ;;
        display_name) col_display_name=$i ;;
    esac
done

[[ $col_username -ge 0 ]] || die "CSV 表头缺少 username 列"
[[ $col_password -ge 0 ]] || die "CSV 表头缺少 password 列"

# ─────────────────── 批量创建 ───────────────────
if [[ "${DRY_RUN:-}" == "1" ]]; then
    warn "DRY_RUN 模式，仅预览不创建"
fi

info "从 $CSV_FILE 批量创建用户..."
echo ""
printf "  ${CYAN}%-24s %-32s %-24s %s${NC}\n" "用户名" "密码" "邮箱" "状态"
printf "  %-24s %-32s %-24s %s\n" "────────" "────────" "────────" "────────"

success=0
failed=0
skipped=0
generated_passwords=()
line_no=1

while IFS= read -r line; do
    line_no=$((line_no + 1))

    # 去除 BOM 和首尾空白
    line=$(echo "$line" | tr -d '\r' | sed 's/^[[:space:]]*//')
    [[ -n "$line" ]] || continue

    # 解析 CSV 行
    IFS=',' read -ra fields <<< "$line"
    username=$(echo "${fields[$col_username]:-}" | xargs)
    password="${fields[$col_password]:-}"
    password=$(echo "$password" | xargs)
    email=""
    display_name=""
    [[ $col_email -ge 0 ]]        && email=$(echo "${fields[$col_email]:-}" | xargs)
    [[ $col_display_name -ge 0 ]] && display_name=$(echo "${fields[$col_display_name]:-}" | xargs)

    # 跳过空行
    if [[ -z "$username" ]]; then
        skipped=$((skipped + 1))
        continue
    fi

    # 自动生成密码
    auto_gen=""
    if [[ -z "$password" ]]; then
        password=$(gen_password)
        auto_gen=" (自动生成)"
    fi

    # 构造 JSON
    payload=$(printf '{"username":"%s","password":"%s","email":"%s","display_name":"%s"}' \
        "$(json_escape "$username")" \
        "$(json_escape "$password")" \
        "$(json_escape "$email")" \
        "$(json_escape "$display_name")")

    if [[ "${DRY_RUN:-}" == "1" ]]; then
        printf "  %-24s %-32s %-24s %s\n" "$username" "${password}${auto_gen}" "$email" "预览"
        success=$((success + 1))
        continue
    fi

    # 调用注册 API
    resp_file="$(mktemp)"
    status=$(curl -sS -o "$resp_file" -w "%{http_code}" \
        -X POST "${BASE_URL}/api/user/register" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "$payload")

    if [[ "$status" == "201" ]]; then
        success=$((success + 1))
        printf "  %-24s %-32s %-24s " "$username" "${password}${auto_gen}" "$email"
        ok "创建成功"
        if [[ -n "$auto_gen" ]]; then
            generated_passwords+=("${username}:${password}")
        fi
    else
        failed=$((failed + 1))
        msg=$(json_value "message" "$(cat "$resp_file")")
        [[ -z "$msg" ]] && msg=$(cat "$resp_file")
        printf "  %-24s %-32s %-24s " "$username" "********" "$email"
        fail "HTTP $status - $msg"
    fi

    rm -f "$resp_file"
done < <(tail -n +2 "$CSV_FILE")

# ─────────────────── 结果汇总 ───────────────────
echo ""
echo "════════════════════════════════════════"
printf "  成功: ${GREEN}%d${NC}  失败: ${RED}%d${NC}  跳过: ${YELLOW}%d${NC}\n" "$success" "$failed" "$skipped"
echo "════════════════════════════════════════"

# 输出自动生成的密码供记录
if [[ ${#generated_passwords[@]} -gt 0 ]]; then
    echo ""
    warn "以下用户的密码为自动生成，请妥善保存："
    for entry in "${generated_passwords[@]}"; do
        echo "  $entry"
    done
fi

if [[ "$failed" -gt 0 ]]; then
    exit 1
fi
