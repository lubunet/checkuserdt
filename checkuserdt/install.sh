#!/usr/bin/env bash
set -euo pipefail

APP_NAME="checkuserdt"
SERVICE_NAME="checkuser555.service"
APP_DIR="/root/checkuserdt"
BIN_PATH="/usr/local/bin/checkuserdt"
CHK_PATH="/usr/local/bin/chk"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"
PORT="${PORT:-555}"
LISTEN="${LISTEN:-0.0.0.0}"
XRAY_CONFIG="${XRAY_CONFIG:-/usr/local/etc/xray/config.json}"
DATA_DIR="${DATA_DIR:-${APP_DIR}/data}"
APP_LINK_FILE="${APP_DIR}/app_link.txt"
REPO_ZIP_URL="${REPO_ZIP_URL:-https://github.com/lubunet/checkuserdt/archive/refs/heads/main.zip}"

C_RESET='\033[0m'
C_GREEN='\033[38;5;46m'
C_CYAN='\033[38;5;51m'
C_BLUE='\033[38;5;39m'
C_YELLOW='\033[38;5;220m'
C_RED='\033[38;5;196m'
C_GRAY='\033[38;5;240m'
C_BOLD='\033[1m'

progress() {
  local percent="$1"
  local msg="$2"
  local width=30
  local filled=$(( percent * width / 100 ))
  local empty=$(( width - filled ))
  local bar=""
  local i
  for ((i=0; i<filled; i++)); do bar+="█"; done
  for ((i=0; i<empty; i++)); do bar+="░"; done
  printf "\r${C_CYAN}[${C_GREEN}%s${C_CYAN}]${C_YELLOW} %3s%%${C_RESET} ${C_GRAY}(%s)${C_RESET}" "$bar" "$percent" "$msg"
}

line() { printf "\n${C_GRAY}────────────────────────────────────────────────────────────${C_RESET}\n"; }
finish_progress() { printf "\n"; }
fail() { finish_progress; printf "${C_RED}[ERRO]${C_RESET} %s\n" "$*"; exit 1; }

[[ "${EUID}" -eq 0 ]] || fail "execute como root: sudo bash install.sh"

clear || true
printf "${C_CYAN}${C_BOLD}============================================================${C_RESET}\n"
printf "${C_GREEN}${C_BOLD}                  CHECKUSERDT LUBUNET${C_RESET}\n"
printf "${C_CYAN}        Usuario normal + UUID Xray/V2ray | Porta ${PORT}${C_RESET}\n"
printf "${C_CYAN}${C_BOLD}============================================================${C_RESET}\n"

progress 4 "preparando ambiente"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y >/dev/null 2>&1 || true
apt-get install -y curl unzip ca-certificates golang-go python3 >/dev/null 2>&1 || fail "falha ao instalar dependencias"

progress 14 "criando pasta ${APP_DIR}"
mkdir -p "${APP_DIR}" "${DATA_DIR}"

progress 24 "baixando codigo sem usar git"
TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "${TMP_DIR}"; }
trap cleanup EXIT

SOURCE_DIR=""
if [[ -f "./go.mod" && -d "./cmd/checkuserdt" ]]; then
  SOURCE_DIR="$(pwd)"
else
  curl -fsSL "${REPO_ZIP_URL}" -o "${TMP_DIR}/repo.zip" || fail "nao foi possivel baixar o codigo do GitHub"
  unzip -q "${TMP_DIR}/repo.zip" -d "${TMP_DIR}" || fail "nao foi possivel descompactar o codigo"
  if [[ -d "${TMP_DIR}/checkuserdt-main/checkuserdt" ]]; then
    SOURCE_DIR="${TMP_DIR}/checkuserdt-main/checkuserdt"
  elif [[ -d "${TMP_DIR}/checkuserdt-main" ]]; then
    SOURCE_DIR="${TMP_DIR}/checkuserdt-main"
  else
    fail "estrutura do ZIP do GitHub nao encontrada"
  fi
fi

progress 36 "copiando arquivos para ${APP_DIR}"
if [[ "$(readlink -f "${SOURCE_DIR}")" != "$(readlink -f "${APP_DIR}" 2>/dev/null || echo '')" ]]; then
  find "${APP_DIR}" -mindepth 1 -maxdepth 1 ! -name data -exec rm -rf {} +
  cp -a "${SOURCE_DIR}/." "${APP_DIR}/"
fi
mkdir -p "${DATA_DIR}"

progress 48 "ajustando permissoes"
chmod +x "${APP_DIR}/install.sh" "${APP_DIR}/uninstall.sh" 2>/dev/null || true
chmod +x "${APP_DIR}/scripts/chk" 2>/dev/null || true

progress 60 "compilando checkuserdt"
cd "${APP_DIR}"
go build -ldflags='-w -s' -o "${BIN_PATH}" ./cmd/checkuserdt || fail "falha ao compilar o checkuserdt"
chmod +x "${BIN_PATH}"

progress 72 "instalando painel chk"
cp "${APP_DIR}/scripts/chk" "${CHK_PATH}" || fail "falha ao instalar chk"
chmod +x "${CHK_PATH}"

progress 82 "criando servico ${SERVICE_NAME}"
cat > "${SERVICE_PATH}" <<SERVICE
[Unit]
Description=CheckUserDT Lubunet porta 555
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${APP_DIR}
ExecStart=${BIN_PATH} --start --listen ${LISTEN} --port ${PORT} --xray-config ${XRAY_CONFIG} --data-dir ${DATA_DIR}
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SERVICE

progress 92 "iniciando servico"
systemctl daemon-reload >/dev/null 2>&1
systemctl enable --now "${SERVICE_NAME}" >/dev/null 2>&1 || fail "falha ao iniciar ${SERVICE_NAME}"

progress 98 "detectando ip da vps"
VPS_IP="$(curl -4 -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"
if [[ -z "${VPS_IP}" ]]; then
  VPS_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
fi
if [[ -z "${VPS_IP}" ]]; then
  VPS_IP="IP_DA_VPS"
fi
printf "http://%s:%s\n" "${VPS_IP}" "${PORT}" > "${APP_LINK_FILE}" || true

progress 100 "instalacao concluida"
finish_progress
line
printf "${C_GREEN}${C_BOLD}CHECKUSERDT INSTALADO COM SUCESSO${C_RESET}\n"
line
printf "${C_YELLOW}Link para usar no app:${C_RESET}\n"
printf "${C_GREEN}${C_BOLD}http://${VPS_IP}:${PORT}${C_RESET}\n\n"
printf "${C_YELLOW}Comando para acessar o painel:${C_RESET}\n"
printf "${C_GREEN}${C_BOLD}chk${C_RESET}\n"
line
