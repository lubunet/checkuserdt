#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="checkuser555.service"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"
APP_DIR="/root/checkuserdt"
BIN_PATH="/usr/local/bin/checkuserdt"
CHK_PATH="/usr/local/bin/chk"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
info(){ echo -e "${BLUE}[INFO]${NC} $*"; }
ok(){ echo -e "${GREEN}[OK]${NC} $*"; }
warn(){ echo -e "${YELLOW}[AVISO]${NC} $*"; }
fail(){ echo -e "${RED}[ERRO]${NC} $*"; exit 1; }

[[ "${EUID}" -eq 0 ]] || fail "Execute como root: sudo bash /root/checkuserdt/uninstall.sh"

read -rp "Tem certeza que deseja remover o CheckUserDT? [s/N]: " confirm
case "${confirm}" in
  s|S|sim|SIM|y|Y|yes|YES) ;;
  *) warn "Remocao cancelada."; exit 0 ;;
esac

info "Parando e desativando ${SERVICE_NAME}..."
systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
rm -f "${SERVICE_PATH}"
systemctl daemon-reload 2>/dev/null || true

info "Removendo binarios e comando chk..."
rm -f "${BIN_PATH}" "${CHK_PATH}"

read -rp "Remover tambem a pasta ${APP_DIR}? [s/N]: " remove_dir
case "${remove_dir}" in
  s|S|sim|SIM|y|Y|yes|YES)
    rm -rf "${APP_DIR}"
    ok "Pasta ${APP_DIR} removida."
    ;;
  *) warn "Pasta ${APP_DIR} mantida." ;;
esac

ok "CheckUserDT removido."
