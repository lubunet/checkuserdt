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
XRAY_API="${XRAY_API:-127.0.0.1:1085}"
XRAY_BIN="${XRAY_BIN:-/usr/local/bin/xray}"
DATA_DIR="${APP_DIR}/data"
REPO_ZIP_URL="${REPO_ZIP_URL:-https://codeload.github.com/lubunet/checkuserdt/zip/refs/heads/main}"

BOLD=$'\033[1m'
DIM=$'\033[2m'
RESET=$'\033[0m'
RED=$'\033[38;5;203m'
GREEN=$'\033[38;5;82m'
YELLOW=$'\033[38;5;221m'
BLUE=$'\033[38;5;39m'
CYAN=$'\033[38;5;51m'
MAGENTA=$'\033[38;5;213m'
BAR_FULL=$'\033[38;5;82m'
BAR_EMPTY=$'\033[38;5;238m'

fail(){
  printf "\n%b[ERRO]%b %s\n" "${RED}${BOLD}" "${RESET}" "$*" >&2
  exit 1
}

public_ip(){
  local ip=""
  ip="$(curl -4 -fsS --connect-timeout 3 --max-time 6 https://api.ipify.org 2>/dev/null || true)"
  if [[ -z "${ip}" ]]; then
    ip="$(curl -4 -fsS --connect-timeout 3 --max-time 6 https://ifconfig.me 2>/dev/null || true)"
  fi
  if [[ -z "${ip}" ]]; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  fi
  [[ -n "${ip}" ]] && printf '%s' "${ip}" || printf 'IP_DA_VPS'
}

if [[ -n "${APP_URL:-}" ]]; then
  FINAL_APP_URL="${APP_URL}"
elif [[ -n "${APP_HOST:-}" ]]; then
  FINAL_APP_URL="http://${APP_HOST}:${PORT}"
else
  FINAL_APP_URL="http://$(public_ip):${PORT}"
fi

bar(){
  local pct="$1" msg="$2" width=28
  local filled=$(( pct * width / 100 ))
  local empty=$(( width - filled ))
  local full="" blank=""
  local i
  for ((i=0;i<filled;i++)); do full+="█"; done
  for ((i=0;i<empty;i++)); do blank+="░"; done
  printf "\r%b[%b%s%b%s%b] %b%3d%%%b %b%s%b" \
    "${CYAN}" "${BAR_FULL}" "${full}" "${BAR_EMPTY}" "${blank}" "${CYAN}" \
    "${BOLD}${GREEN}" "${pct}" "${RESET}" "${DIM}" "${msg}" "${RESET}"
}

run_step(){
  local start="$1" end="$2" msg="$3" fn="$4"
  local log_file pid pct
  log_file="$(mktemp)"
  bar "${start}" "${msg}"
  ( set +e; "${fn}" >"${log_file}" 2>&1; echo $? >"${log_file}.code" ) &
  pid=$!
  pct=${start}
  while kill -0 "${pid}" 2>/dev/null; do
    bar "${pct}" "${msg}"
    sleep 0.12
    if (( pct < end - 1 )); then pct=$((pct + 1)); fi
  done
  wait "${pid}" 2>/dev/null || true
  local code="0"
  [[ -f "${log_file}.code" ]] && code="$(cat "${log_file}.code")"
  if [[ "${code}" != "0" ]]; then
    printf "\n%bFalhou em:%b %s\n" "${RED}${BOLD}" "${RESET}" "${msg}"
    tail -n 100 "${log_file}" || true
    rm -f "${log_file}" "${log_file}.code"
    exit "${code}"
  fi
  bar "${end}" "${msg}"
  rm -f "${log_file}" "${log_file}.code"
}

prepare_env(){
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -y >/dev/null 2>&1
  apt-get install -y curl unzip ca-certificates golang-go >/dev/null 2>&1
}

create_dirs(){
  mkdir -p "${APP_DIR}" "${DATA_DIR}" "${APP_DIR}/limits"
}

get_code(){
  local script_path script_dir local_source tmpcopy tmpdir extracted src
  script_path="${BASH_SOURCE[0]}"
  script_dir="$(cd "$(dirname "${script_path}")" 2>/dev/null && pwd || echo /tmp)"
  local_source="no"
  if [[ -f "${script_dir}/cmd/checkuserdt/main.go" && -f "${script_dir}/go.mod" ]]; then
    local_source="yes"
  fi

  if [[ "${local_source}" == "yes" ]]; then
    tmpcopy="$(mktemp -d)"
    tar -C "${script_dir}" --exclude='.git' -cf - . | tar -C "${tmpcopy}" -xf -
    rm -rf "${APP_DIR:?}/cmd" "${APP_DIR:?}/scripts" "${APP_DIR:?}/go.mod" "${APP_DIR:?}/README.md" "${APP_DIR:?}/install.sh" "${APP_DIR:?}/uninstall.sh" "${APP_DIR:?}/.gitattributes"
    tar -C "${tmpcopy}" -cf - . | tar -C "${APP_DIR}" -xf -
    rm -rf "${tmpcopy}"
  else
    tmpdir="$(mktemp -d)"
    curl -fL --connect-timeout 15 --max-time 120 "${REPO_ZIP_URL}" -o "${tmpdir}/repo.zip" >/dev/null 2>&1
    unzip -q "${tmpdir}/repo.zip" -d "${tmpdir}"
    extracted="$(find "${tmpdir}" -maxdepth 1 -type d -name 'checkuserdt-*' | head -n 1)"
    [[ -n "${extracted}" ]] || { echo "nao encontrei a pasta extraida do ZIP"; return 1; }
    if [[ -f "${extracted}/checkuserdt/cmd/checkuserdt/main.go" ]]; then
      src="${extracted}/checkuserdt"
    elif [[ -f "${extracted}/cmd/checkuserdt/main.go" ]]; then
      src="${extracted}"
    else
      echo "estrutura invalida no GitHub"
      return 1
    fi
    rm -rf "${APP_DIR:?}/cmd" "${APP_DIR:?}/scripts" "${APP_DIR:?}/go.mod" "${APP_DIR:?}/README.md" "${APP_DIR:?}/install.sh" "${APP_DIR:?}/uninstall.sh" "${APP_DIR:?}/.gitattributes"
    tar -C "${src}" --exclude='.git' -cf - . | tar -C "${APP_DIR}" -xf -
    rm -rf "${tmpdir}"
  fi
}

set_permissions(){
  chmod +x "${APP_DIR}/install.sh" "${APP_DIR}/uninstall.sh" 2>/dev/null || true
  chmod +x "${APP_DIR}/scripts/chk" 2>/dev/null || true
  printf '%s\n' "${FINAL_APP_URL}" > "${APP_DIR}/app_url"
}

build_app(){
  cd "${APP_DIR}"
  go mod tidy >/dev/null 2>&1 || true
  go build -ldflags='-w -s' -o "${BIN_PATH}" ./cmd/checkuserdt
  chmod +x "${BIN_PATH}"
}

install_chk(){
  cp "${APP_DIR}/scripts/chk" "${CHK_PATH}"
  chmod +x "${CHK_PATH}"
}

create_service(){
  cat > "${SERVICE_PATH}" <<SERVICE
[Unit]
Description=CheckUserDT Lubunet - usuario normal e UUID Xray
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${APP_DIR}
ExecStart=${BIN_PATH} --start --listen ${LISTEN} --port ${PORT} --xray-config ${XRAY_CONFIG} --xray-api ${XRAY_API} --xray-bin ${XRAY_BIN} --data-dir ${DATA_DIR}
Restart=always
RestartSec=3
LimitNOFILE=65535
Environment=APP_URL=${FINAL_APP_URL}

[Install]
WantedBy=multi-user.target
SERVICE
}

start_service(){
  systemctl daemon-reload >/dev/null 2>&1
  systemctl enable "${SERVICE_NAME}" >/dev/null 2>&1
  systemctl restart "${SERVICE_NAME}" >/dev/null 2>&1
}

verify_service(){
  sleep 1
  systemctl is-active --quiet "${SERVICE_NAME}"
}

open_firewall(){
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi active; then
    ufw allow "${PORT}/tcp" >/dev/null 2>&1 || true
  fi
}

[[ "${EUID}" -eq 0 ]] || fail "execute como root"

clear 2>/dev/null || true
printf "%b\n" "${MAGENTA}${BOLD}╔════════════════════════════════════════════════════╗${RESET}"
printf "%b\n" "${MAGENTA}${BOLD}║              CHECKUSERDT  LUBUNET                 ║${RESET}"
printf "%b\n" "${MAGENTA}${BOLD}║        Usuário normal + UUID Xray/V2ray            ║${RESET}"
printf "%b\n\n" "${MAGENTA}${BOLD}╚════════════════════════════════════════════════════╝${RESET}"

run_step 3 16 "preparando ambiente" prepare_env
run_step 16 25 "criando pasta /root/checkuserdt" create_dirs
run_step 25 42 "baixando código sem usar git" get_code
run_step 42 50 "ajustando permissões" set_permissions
run_step 50 68 "compilando checkuserdt" build_app
run_step 68 76 "instalando comando chk" install_chk
run_step 76 86 "criando serviço checkuser555.service" create_service
run_step 86 96 "iniciando serviço na porta ${PORT}" start_service
run_step 96 99 "verificando inicialização" verify_service
run_step 99 100 "ajustando firewall se necessário" open_firewall
printf "\n\n"

printf "%b\n" "${GREEN}${BOLD}CHECKUSERDT INSTALADO COM SUCESSO${RESET}"
printf "%b\n" "${CYAN}────────────────────────────────────────────────────${RESET}"
printf "%b %s\n" "${BOLD}Link para usar no app:${RESET}" "${FINAL_APP_URL}"
printf "%b %s\n" "${BOLD}Comando para acessar o painel:${RESET}" "chk"
printf "%b\n" "${CYAN}────────────────────────────────────────────────────${RESET}"
