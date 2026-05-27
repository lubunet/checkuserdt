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
RAW_INSTALL_URL="https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
info(){ echo -e "${BLUE}[INFO]${NC} $*"; }
ok(){ echo -e "${GREEN}[OK]${NC} $*"; }
warn(){ echo -e "${YELLOW}[AVISO]${NC} $*"; }
fail(){ echo -e "${RED}[ERRO]${NC} $*"; exit 1; }

[[ "${EUID}" -eq 0 ]] || fail "Execute como root: sudo bash install.sh"

clear 2>/dev/null || true
cat <<BANNER
============================================================
             CHECKUSERDT / CHECKUSER 555
       Usuario normal + UUID Xray/V2ray sem usar git
============================================================
BANNER

info "[1/10] Verificando dependencias sem instalar git..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y >/dev/null
apt-get install -y curl unzip ca-certificates golang-go >/dev/null
ok "Dependencias instaladas: curl, unzip, ca-certificates, golang-go. Git nao e necessario."

info "[2/10] Criando pasta ${APP_DIR}..."
mkdir -p "${APP_DIR}" "${DATA_DIR}" "${APP_DIR}/limits"
ok "Pasta criada."

SCRIPT_PATH="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "$(dirname "${SCRIPT_PATH}")" 2>/dev/null && pwd || echo /tmp)"
LOCAL_SOURCE="no"
if [[ -f "${SCRIPT_DIR}/cmd/checkuserdt/main.go" && -f "${SCRIPT_DIR}/go.mod" ]]; then
  LOCAL_SOURCE="yes"
fi

info "[3/10] Copiando codigo do projeto..."
if [[ "${LOCAL_SOURCE}" == "yes" ]]; then
  info "Instalador executado a partir da pasta local do projeto. Copiando arquivos locais..."
  tmpcopy="$(mktemp -d)"
  tar -C "${SCRIPT_DIR}" --exclude='.git' -cf - . | tar -C "${tmpcopy}" -xf -
  rm -rf "${APP_DIR:?}/cmd" "${APP_DIR:?}/scripts" "${APP_DIR:?}/go.mod" "${APP_DIR:?}/README.md" "${APP_DIR:?}/install.sh" "${APP_DIR:?}/uninstall.sh" "${APP_DIR:?}/.gitattributes"
  tar -C "${tmpcopy}" -cf - . | tar -C "${APP_DIR}" -xf -
  rm -rf "${tmpcopy}"
else
  info "Baixando ZIP do GitHub: ${REPO_ZIP_URL}"
  tmpdir="$(mktemp -d)"
  curl -fL --connect-timeout 15 --max-time 120 "${REPO_ZIP_URL}" -o "${tmpdir}/repo.zip"
  unzip -q "${tmpdir}/repo.zip" -d "${tmpdir}"
  extracted="$(find "${tmpdir}" -maxdepth 1 -type d -name 'checkuserdt-*' | head -n 1)"
  [[ -n "${extracted}" ]] || fail "Nao encontrei a pasta extraida do ZIP."

  if [[ -f "${extracted}/checkuserdt/cmd/checkuserdt/main.go" ]]; then
    src="${extracted}/checkuserdt"
  elif [[ -f "${extracted}/cmd/checkuserdt/main.go" ]]; then
    src="${extracted}"
  else
    fail "Estrutura invalida no GitHub. Precisa existir cmd/checkuserdt/main.go."
  fi

  rm -rf "${APP_DIR:?}/cmd" "${APP_DIR:?}/scripts" "${APP_DIR:?}/go.mod" "${APP_DIR:?}/README.md" "${APP_DIR:?}/install.sh" "${APP_DIR:?}/uninstall.sh" "${APP_DIR:?}/.gitattributes"
  tar -C "${src}" --exclude='.git' -cf - . | tar -C "${APP_DIR}" -xf -
  rm -rf "${tmpdir}"
fi
ok "Codigo copiado para ${APP_DIR}."

info "[4/10] Ajustando permissoes dos scripts..."
chmod +x "${APP_DIR}/install.sh" "${APP_DIR}/uninstall.sh" 2>/dev/null || true
chmod +x "${APP_DIR}/scripts/chk" 2>/dev/null || true
ok "Permissoes ajustadas."

info "[5/10] Compilando binario Go..."
cd "${APP_DIR}"
go mod tidy >/dev/null 2>&1 || true
go build -ldflags='-w -s' -o "${BIN_PATH}" ./cmd/checkuserdt
chmod +x "${BIN_PATH}"
ok "Binario instalado em ${BIN_PATH}."

info "[6/10] Instalando comando do painel: chk..."
cp "${APP_DIR}/scripts/chk" "${CHK_PATH}"
chmod +x "${CHK_PATH}"
ok "Comando chk instalado em ${CHK_PATH}."

info "[7/10] Criando servico systemd ${SERVICE_NAME}..."
cat > "${SERVICE_PATH}" <<SERVICE
[Unit]
Description=CheckUserDT porta 555 - usuario normal e UUID Xray
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

[Install]
WantedBy=multi-user.target
SERVICE
ok "Servico criado em ${SERVICE_PATH}."

info "[8/10] Reiniciando systemd e iniciando servico..."
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}" >/dev/null
systemctl restart "${SERVICE_NAME}"
sleep 1
ok "Servico iniciado."

info "[9/10] Testando porta local..."
if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null; then
  ok "CheckUserDT respondeu em http://127.0.0.1:${PORT}/health"
else
  warn "Nao consegui testar /health. Veja os logs com: journalctl -u ${SERVICE_NAME} -n 80 --no-pager"
fi

info "[10/10] Gerando links para o app..."
PUBLIC_IP="$(curl -4fsS --connect-timeout 3 --max-time 8 https://api.ipify.org 2>/dev/null || true)"
if [[ -z "${PUBLIC_IP}" ]]; then
  PUBLIC_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
fi
[[ -z "${PUBLIC_IP}" ]] && PUBLIC_IP="IP_DA_VPS"

if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi active; then
  ufw allow "${PORT}/tcp" >/dev/null || true
  ok "UFW ativo: porta ${PORT}/tcp liberada."
fi

cat <<FINAL

============================================================
 ${GREEN}INSTALACAO FINALIZADA${NC}
============================================================

Servico:
  ${SERVICE_NAME}

Status:
  sudo systemctl status ${SERVICE_NAME} -l --no-pager

Painel no terminal:
  chk

Painel web:
  http://${PUBLIC_IP}:${PORT}/panel

Link para colocar no app:
  http://${PUBLIC_IP}:${PORT}/check/USUARIO_OU_UUID

Exemplo usuario normal:
  http://${PUBLIC_IP}:${PORT}/check/usuario

Exemplo UUID Xray:
  http://${PUBLIC_IP}:${PORT}/check/UUID_DO_XRAY

Teste local:
  curl http://127.0.0.1:${PORT}/health

Instalacao pelo GitHub, sem git:
  bash <(curl -sL ${RAW_INSTALL_URL})

Observacao:
  Se local funcionar e externo nao abrir, libere a porta ${PORT}/tcp no firewall/security group da VPS.
============================================================
FINAL
