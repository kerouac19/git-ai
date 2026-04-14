#!/usr/bin/env bash
#
# Git-AI Server 裸机部署脚本
# 系统用户: devops（已存在）
# 用法:
#   1. 在构建机: cd server-go && bash scripts/deploy.sh build
#   2. 传到目标机后: sudo bash deploy.sh install
#   3. 首次部署需要: sudo bash deploy.sh init-db
#   4. 更新版本:     sudo bash deploy.sh upgrade
#
set -euo pipefail

# ─────────────────── 配置 ───────────────────
DEPLOY_USER="devops"
DEPLOY_GROUP="devops"
INSTALL_DIR="/opt/git-ai/server-go/current"
ENV_FILE="/opt/git-ai/.env"
LOG_DIR="/opt/git-ai/logs"
SERVICE_NAME="git-ai"
BINARY_NAME="git-ai-server"
PORT=5000
STOP_TIMEOUT=15

# 构建产物路径（相对于 server-go/）
BUILD_OUTPUT="bin/${BINARY_NAME}"

# ─────────────────── 颜色 ───────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()   { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# 优雅停止服务：发送 SIGTERM 并等待进程退出
graceful_stop() {
    if ! systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        return 0
    fi

    info "正在优雅停止服务（等待最多 ${STOP_TIMEOUT}s）..."
    systemctl stop "${SERVICE_NAME}" 2>/dev/null

    local waited=0
    while systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; do
        if (( waited >= STOP_TIMEOUT )); then
            warn "优雅停止超时，强制终止..."
            systemctl kill -s SIGKILL "${SERVICE_NAME}" 2>/dev/null
            sleep 1
            break
        fi
        sleep 1
        (( waited++ ))
    done

    ok "服务已停止"
}

# ─────────────────── build ───────────────────
cmd_build() {
    info "构建 ${BINARY_NAME} (linux/amd64)..."

    if ! command -v go &>/dev/null; then
        die "未找到 go，请先安装 Go 1.26+"
    fi

    # 确保在 server-go 目录下
    if [[ ! -f "go.mod" ]]; then
        if [[ -f "server-go/go.mod" ]]; then
            cd server-go
        else
            die "请在 server-go/ 目录下运行，或在项目根目录运行"
        fi
    fi

    mkdir -p bin
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags="-s -w" -o "${BUILD_OUTPUT}" ./cmd/server

    ok "构建完成: $(pwd)/${BUILD_OUTPUT} ($(du -h "${BUILD_OUTPUT}" | cut -f1))"
    echo ""
    info "下一步: 将 ${BUILD_OUTPUT} 和此脚本传到目标服务器，然后运行:"
    echo "  sudo bash deploy.sh install"
}

# ─────────────────── init-db ───────────────────
cmd_init_db() {
    info "初始化数据库..."

    if ! command -v psql &>/dev/null; then
        die "未找到 psql，请先安装 PostgreSQL 客户端"
    fi

    if psql -U postgres -lqt 2>/dev/null | cut -d'|' -f1 | grep -qw git_ai; then
        warn "数据库 git_ai 已存在，跳过创建"
    else
        psql -U postgres -c "CREATE DATABASE git_ai" 2>/dev/null \
            || psql -d postgres -c "CREATE DATABASE git_ai"
        ok "数据库 git_ai 已创建"
    fi

    info "表结构将在服务首次启动时自动迁移"
}

# ─────────────────── install ───────────────────
cmd_install() {
    [[ $EUID -eq 0 ]] || die "请使用 sudo 运行"

    # 检查用户
    if ! id "${DEPLOY_USER}" &>/dev/null; then
        die "系统用户 ${DEPLOY_USER} 不存在"
    fi

    info "开始安装..."

    # 1. 查找二进制
    local binary=""
    for candidate in \
        "./${BUILD_OUTPUT}" \
        "./${BINARY_NAME}" \
        "$(dirname "$0")/../${BUILD_OUTPUT}" \
        "$(dirname "$0")/${BINARY_NAME}"; do
        if [[ -f "${candidate}" ]]; then
            binary="${candidate}"
            break
        fi
    done
    [[ -n "${binary}" ]] || die "未找到 ${BINARY_NAME}，请先运行 bash deploy.sh build"

    # 2. 创建目录
    mkdir -p "${INSTALL_DIR}"
    mkdir -p "${LOG_DIR}"

    # 3. 停止旧服务（如果存在）
    graceful_stop

    # 4. 部署二进制
    cp "${binary}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"
    ok "二进制已部署到 ${INSTALL_DIR}/${BINARY_NAME}"

    # 5. 生成 .env（仅首次）
    if [[ ! -f "${ENV_FILE}" ]]; then
        info "生成 ${ENV_FILE}..."
        local jwt_secret encryption_master_key cas_key
        jwt_secret=$(openssl rand -base64 48)
        encryption_master_key=$(openssl rand -hex 32)
        cas_key=$(openssl rand -base64 48)

        cat > "${ENV_FILE}" <<ENVEOF
PORT=${PORT}
APP_ENV=production
JWT_SECRET=${jwt_secret}
ENCRYPTION_MASTER_KEY=${encryption_master_key}
CAS_ENCRYPTION_KEY=${cas_key}
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=
DB_NAME=git_ai
DB_SSL=false
CORS_ORIGIN=*
TRUST_PROXY=1
HTTPS_REDIRECT=false
ENVEOF
        chmod 600 "${ENV_FILE}"
        ok ".env 已生成（密钥已自动创建）"
        warn "请根据实际环境修改 ${ENV_FILE} 中的数据库和 CORS 配置"
    else
        ok ".env 已存在，保留现有配置"
    fi

    # 6. 设置所有权
    chown -R "${DEPLOY_USER}:${DEPLOY_GROUP}" /opt/git-ai

    # 7. 安装 systemd service
    info "安装 systemd 服务..."
    cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<SVCEOF
[Unit]
Description=Git-AI Go Server
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=${DEPLOY_USER}
Group=${DEPLOY_GROUP}

WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
ExecStartPre=/usr/bin/test -x ${INSTALL_DIR}/${BINARY_NAME}
ExecStartPre=/usr/bin/test -f ${ENV_FILE}

Restart=always
RestartSec=5
TimeoutStartSec=30
TimeoutStopSec=15
KillSignal=SIGTERM

LimitNOFILE=65536
UMask=0077
SyslogIdentifier=${SERVICE_NAME}

StandardOutput=journal
StandardError=journal

NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
ReadWritePaths=${LOG_DIR}

[Install]
WantedBy=multi-user.target
SVCEOF

    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}"
    ok "systemd 服务已安装并设为开机启动"

    # 8. 启动
    info "启动服务..."
    systemctl start "${SERVICE_NAME}"
    sleep 2

    if systemctl is-active --quiet "${SERVICE_NAME}"; then
        ok "服务已启动"
    else
        die "服务启动失败，请检查: journalctl -u ${SERVICE_NAME} -n 50"
    fi

    # 9. 健康检查
    info "执行健康检查..."
    sleep 1
    local health
    if health=$(curl -sf "http://127.0.0.1:${PORT}/health" 2>/dev/null); then
        ok "健康检查通过: ${health}"
    else
        warn "健康检查未通过，服务可能还在初始化，请稍后手动检查:"
        echo "  curl http://127.0.0.1:${PORT}/health"
    fi

    local db_health
    if db_health=$(curl -sf "http://127.0.0.1:${PORT}/api/health/database" 2>/dev/null); then
        ok "数据库连接正常: ${db_health}"
    else
        warn "数据库检查未通过，请确认 PostgreSQL 已启动且 .env 配置正确"
    fi

    echo ""
    ok "═══════════════════════════════════════"
    ok "  部署完成！"
    ok "═══════════════════════════════════════"
    echo ""
    echo "  服务地址:  http://127.0.0.1:${PORT}"
    echo "  配置文件:  ${ENV_FILE}"
    echo "  二进制:    ${INSTALL_DIR}/${BINARY_NAME}"
    echo "  日志查看:  journalctl -u ${SERVICE_NAME} -f"
    echo "  服务管理:  systemctl {start|stop|restart|status} ${SERVICE_NAME}"
    echo ""
}

# ─────────────────── upgrade ───────────────────
cmd_upgrade() {
    [[ $EUID -eq 0 ]] || die "请使用 sudo 运行"

    info "升级 ${BINARY_NAME}..."

    # 查找新二进制
    local binary=""
    for candidate in \
        "./${BUILD_OUTPUT}" \
        "./${BINARY_NAME}" \
        "$(dirname "$0")/../${BUILD_OUTPUT}" \
        "$(dirname "$0")/${BINARY_NAME}"; do
        if [[ -f "${candidate}" ]]; then
            binary="${candidate}"
            break
        fi
    done
    [[ -n "${binary}" ]] || die "未找到新的 ${BINARY_NAME}"

    # 备份旧版本
    if [[ -f "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        cp "${INSTALL_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}.bak"
        ok "旧版本已备份为 ${BINARY_NAME}.bak"
    fi

    # 停止 → 替换 → 启动
    graceful_stop
    cp "${binary}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"
    chown "${DEPLOY_USER}:${DEPLOY_GROUP}" "${INSTALL_DIR}/${BINARY_NAME}"
    systemctl start "${SERVICE_NAME}"
    sleep 2

    if systemctl is-active --quiet "${SERVICE_NAME}"; then
        ok "升级完成，服务已重新启动"
        curl -sf "http://127.0.0.1:${PORT}/health" 2>/dev/null && ok "健康检查通过"
    else
        warn "新版本启动失败，正在回滚..."
        if [[ -f "${INSTALL_DIR}/${BINARY_NAME}.bak" ]]; then
            cp "${INSTALL_DIR}/${BINARY_NAME}.bak" "${INSTALL_DIR}/${BINARY_NAME}"
            systemctl start "${SERVICE_NAME}"
            die "已回滚到旧版本，请检查新二进制"
        else
            die "无备份可回滚，请手动处理: journalctl -u ${SERVICE_NAME} -n 50"
        fi
    fi
}

# ─────────────────── status ───────────────────
cmd_status() {
    echo "═══ 服务状态 ═══"
    systemctl status "${SERVICE_NAME}" --no-pager 2>/dev/null || warn "服务未安装"
    echo ""

    echo "═══ 健康检查 ═══"
    curl -sf "http://127.0.0.1:${PORT}/health" 2>/dev/null \
        && echo "" \
        || warn "服务未响应"

    curl -sf "http://127.0.0.1:${PORT}/api/health/database" 2>/dev/null \
        && echo "" \
        || warn "数据库未连接"

    echo ""
    echo "═══ 版本信息 ═══"
    curl -sf "http://127.0.0.1:${PORT}/api/version" 2>/dev/null \
        && echo "" \
        || warn "无法获取版本"
}

# ─────────────────── 入口 ───────────────────
case "${1:-}" in
    build)    cmd_build    ;;
    init-db)  cmd_init_db  ;;
    install)  cmd_install  ;;
    upgrade)  cmd_upgrade  ;;
    status)   cmd_status   ;;
    *)
        echo "用法: $0 {build|init-db|install|upgrade|status}"
        echo ""
        echo "  build    - 编译 linux/amd64 二进制"
        echo "  init-db  - 创建 PostgreSQL 数据库"
        echo "  install  - 首次部署（创建目录、.env、systemd 服务并启动）"
        echo "  upgrade  - 升级二进制（自动备份 + 回滚）"
        echo "  status   - 查看服务状态和健康检查"
        exit 1
        ;;
esac
