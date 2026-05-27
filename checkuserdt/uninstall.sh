#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="checkuser555.service"
APP_DIR="/root/checkuserdt"
BIN_PATH="/usr/local/bin/checkuserdt"
CHK_PATH="/usr/local/bin/chk"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"

BOLD=$'\033[1m'
RESET=$'\033[0m'
RED=$'\033[38;5;203m'
GREEN=$'\033[38;5;82m'
YELLOW=$'\033[38;5;221m'
CYAN=$'\033[38;5;51m'
BAR_FULL=$'\033[38;5;82m'
BAR_EMPTY=$'\033[38;5;238m'

[[ "${EUID}" -eq 0 ]] || { printf "%bExecute como root.%b\n" "${RED}" "${RESET}"; exit 1; }

bar(){
  local pct="$1" msg="$2" width=24 filled empty full="" blank="" i
  filled=$(( pct * width / 100 )); empty=$(( width - filled ))
  for ((i=0;i<filled;i++)); do full+="█"; done
  for ((i=0;i<empty;i++)); do blank+="░"; done
  printf "\r%b[%b%s%b%s%b] %b%3d%%%b %s" "${CYAN}" "${BAR_FULL}" "${full}" "${BAR_EMPTY}" "${blank}" "${CYAN}" "${GREEN}${BOLD}" "${pct}" "${RESET}" "${msg}"
}

bar 15 "parando serviço"
systemctl stop "${SERVICE_NAME}" >/dev/null 2>&1 || true
systemctl disable "${SERVICE_NAME}" >/dev/null 2>&1 || true
sleep 0.1
bar 40 "removendo systemd"
rm -f "${SERVICE_PATH}"
systemctl daemon-reload >/dev/null 2>&1 || true
sleep 0.1
bar 65 "removendo binários"
rm -f "${BIN_PATH}" "${CHK_PATH}"
sleep 0.1
bar 85 "removendo pasta /root/checkuserdt"
rm -rf "${APP_DIR}"
sleep 0.1
bar 100 "remoção concluída"
printf "\n%bCheckUserDT removido.%b\n" "${GREEN}${BOLD}" "${RESET}"
