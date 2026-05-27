#!/usr/bin/env bash
set -euo pipefail
SERVICE_NAME="checkuser555.service"
APP_DIR="/root/checkuserdt"
BIN_PATH="/usr/local/bin/checkuserdt"
CHK_PATH="/usr/local/bin/chk"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"
C_RESET='\033[0m'; C_GREEN='\033[38;5;46m'; C_RED='\033[38;5;196m'; C_YELLOW='\033[38;5;220m'; C_CYAN='\033[38;5;51m'
[[ "${EUID}" -eq 0 ]] || { printf "${C_RED}Execute como root.${C_RESET}\n"; exit 1; }
printf "${C_YELLOW}Removendo CheckUserDT...${C_RESET}\n"
systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
systemctl disable "${SERVICE_NAME}" >/dev/null 2>&1 || true
rm -f "${SERVICE_PATH}" "${BIN_PATH}" "${CHK_PATH}"
systemctl daemon-reload >/dev/null 2>&1 || true
rm -rf "${APP_DIR}"
printf "${C_GREEN}CheckUserDT removido com sucesso.${C_RESET}\n"
