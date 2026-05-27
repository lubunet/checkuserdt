#!/usr/bin/env bash
set -euo pipefail

APP_NAME="checkuserdt"
SERVICE_NAME="checkuser555.service"
APP_DIR="/root/checkuserdt"
BIN_PATH="/usr/local/bin/checkuserdt"
CHK_PATH="/usr/local/bin/chk"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"
PORT="${PORT:-2052}"
LISTEN="${LISTEN:-0.0.0.0}"
XRAY_CONFIG="${XRAY_CONFIG:-/usr/local/etc/xray/config.json}"
XRAY_API="${XRAY_API:-127.0.0.1:1085}"
XRAY_BIN="${XRAY_BIN:-/usr/local/bin/xray}"
DATA_DIR="${APP_DIR}/data"
APP_URL="${APP_URL:-http://vps.lubunet.shop:${PORT}}"
REPO_ZIP_URL="${REPO_ZIP_URL:-https://codeload.github.com/lubunet/checkuserdt/zip/refs/heads/main}"
RAW_INSTALL_URL="https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
fail(){ echo -e "${RED}[ERRO]${NC} $*"; exit 1; }
progress(){
  local pct="$1"; shift
  local msg="$*"
  local filled=$((pct / 5))
  local empty=$((20 - filled))
  local bar=""
  for ((i=0;i<filled;i++)); do bar+="="; done
  for ((i=0;i<empty;i++)); do bar+=" "; done
  echo -e "${BLUE}[${bar}] ${pct}%${NC} (${msg})"
}

[[ "${EUID}" -eq 0 ]] || fail "execute como root"

clear 2>/dev/null || true
cat <<BANNER
============================================================
                  CHECKUSERDT LUBUNET
        Usuario normal + UUID Xray/V2ray
============================================================
BANNER

progress 5 "preparando ambiente"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y >/dev/null
apt-get install -y curl unzip ca-certificates golang-go >/dev/null

progress 15 "criando pasta ${APP_DIR}"
mkdir -p "${APP_DIR}" "${DATA_DIR}" "${APP_DIR}/limits"

progress 25 "baixando codigo sem usar git"
SCRIPT_PATH="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "$(dirname "${SCRIPT_PATH}")" 2>/dev/null && pwd || echo /tmp)"
LOCAL_SOURCE="no"
if [[ -f "${SCRIPT_DIR}/cmd/checkuserdt/main.go" && -f "${SCRIPT_DIR}/go.mod" ]]; then
  LOCAL_SOURCE="yes"
fi

if [[ "${LOCAL_SOURCE}" == "yes" ]]; then
  tmpcopy="$(mktemp -d)"
  tar -C "${SCRIPT_DIR}" --exclude='.git' -cf - . | tar -C "${tmpcopy}" -xf -
  rm -rf "${APP_DIR:?}/cmd" "${APP_DIR:?}/scripts" "${APP_DIR:?}/go.mod" "${APP_DIR:?}/README.md" "${APP_DIR:?}/install.sh" "${APP_DIR:?}/uninstall.sh" "${APP_DIR:?}/.gitattributes"
  tar -C "${tmpcopy}" -cf - . | tar -C "${APP_DIR}" -xf -
  rm -rf "${tmpcopy}"
else
  tmpdir="$(mktemp -d)"
  curl -fL --connect-timeout 15 --max-time 120 "${REPO_ZIP_URL}" -o "${tmpdir}/repo.zip" >/dev/null 2>&1
  unzip -q "${tmpdir}/repo.zip" -d "${tmpdir}"
  extracted="$(find "${tmpdir}" -maxdepth 1 -type d -name 'checkuserdt-*' | head -n 1)"
  [[ -n "${extracted}" ]] || fail "nao encontrei a pasta extraida do ZIP"
  if [[ -f "${extracted}/checkuserdt/cmd/checkuserdt/main.go" ]]; then
    src="${extracted}/checkuserdt"
  elif [[ -f "${extracted}/cmd/checkuserdt/main.go" ]]; then
    src="${extracted}"
  else
    fail "estrutura invalida no GitHub"
  fi
  rm -rf "${APP_DIR:?}/cmd" "${APP_DIR:?}/scripts" "${APP_DIR:?}/go.mod" "${APP_DIR:?}/README.md" "${APP_DIR:?}/install.sh" "${APP_DIR:?}/uninstall.sh" "${APP_DIR:?}/.gitattributes"
  tar -C "${src}" --exclude='.git' -cf - . | tar -C "${APP_DIR}" -xf -
  rm -rf "${tmpdir}"
fi

progress 40 "ajustando permissoes"
chmod +x "${APP_DIR}/install.sh" "${APP_DIR}/uninstall.sh" 2>/dev/null || true
chmod +x "${APP_DIR}/scripts/chk" 2>/dev/null || true

progress 55 "compilando checkuserdt"
cd "${APP_DIR}"
go mod tidy >/dev/null 2>&1 || true
go build -ldflags='-w -s' -o "${BIN_PATH}" ./cmd/checkuserdt
chmod +x "${BIN_PATH}"

progress 70 "instalando painel chk"
cp "${APP_DIR}/scripts/chk" "${CHK_PATH}"
chmod +x "${CHK_PATH}"

progress 80 "criando servico ${SERVICE_NAME}"
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
Environment=APP_URL=${APP_URL}

[Install]
WantedBy=multi-user.target
SERVICE

progress 90 "iniciando servico"
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}" >/dev/null
systemctl restart "${SERVICE_NAME}"
sleep 1

progress 97 "verificando inicializacao"
if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
  journalctl -u "${SERVICE_NAME}" -n 60 --no-pager || true
  fail "o servico nao iniciou corretamente"
fi

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi active; then
  ufw allow "${PORT}/tcp" >/dev/null || true
fi

progress 100 "instalacao concluida"
cat <<FINAL

============================================================
${GREEN}CHECKUSERDT INSTALADO COM SUCESSO${NC}
============================================================

Link para usar no app:
${APP_URL}

Comando para acessar o painel no terminal:
chk

Instalacao pelo GitHub sem git:
bash <(curl -sL ${RAW_INSTALL_URL})

============================================================
FINAL
