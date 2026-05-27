#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="checkuser555.service"
APP_DIR="/root/checkuserdt"
BIN_PATH="/usr/local/bin/checkuserdt"
CHK_PATH="/usr/local/bin/chk"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
progress(){
  local pct="$1"; shift
  local msg="$*"
  local filled=$((pct / 5)); local empty=$((20 - filled)); local bar=""
  for ((i=0;i<filled;i++)); do bar+="="; done
  for ((i=0;i<empty;i++)); do bar+=" "; done
  echo -e "${BLUE}[${bar}] ${pct}%${NC} (${msg})"
}

[[ "${EUID}" -eq 0 ]] || { echo -e "${RED}Execute como root.${NC}"; exit 1; }

progress 15 "parando servico"
systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
systemctl disable "${SERVICE_NAME}" 2>/dev/null || true

progress 40 "removendo systemd"
rm -f "${SERVICE_PATH}"
systemctl daemon-reload 2>/dev/null || true

progress 65 "removendo binarios"
rm -f "${BIN_PATH}" "${CHK_PATH}"

progress 85 "removendo pasta ${APP_DIR}"
rm -rf "${APP_DIR}"

progress 100 "remocao concluida"
echo -e "${GREEN}CheckUserDT removido.${NC}"
