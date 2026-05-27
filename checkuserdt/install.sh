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
REPO_URL="${REPO_URL:-https://github.com/SEU_USUARIO/checkuserdt.git}"
DATA_DIR="${APP_DIR}/data"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
info(){ echo -e "${BLUE}[INFO]${NC} $*"; }
ok(){ echo -e "${GREEN}[OK]${NC} $*"; }
warn(){ echo -e "${YELLOW}[AVISO]${NC} $*"; }
fail(){ echo -e "${RED}[ERRO]${NC} $*"; exit 1; }

[[ "${EUID}" -eq 0 ]] || fail "Execute como root: sudo bash install.sh"

clear || true
cat <<BANNER
============================================================
            CHECKUSERDT / CHECKUSER 555
  Usuario normal + UUID Xray/V2ray na porta ${PORT}
============================================================
BANNER

info "[1/9] Preparando pacotes necessários..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -y >/dev/null
apt-get install -y curl git ca-certificates golang-go >/dev/null
ok "Pacotes verificados."

info "[2/9] Criando pasta ${APP_DIR}..."
mkdir -p "${APP_DIR}" "${DATA_DIR}" "${APP_DIR}/limits"
ok "Pastas criadas."

info "[3/9] Obtendo código do projeto..."
if [[ -f "./go.mod" && -d "./cmd/checkuserdt" ]]; then
  rsync -a --delete --exclude '.git' ./ "${APP_DIR}/" 2>/dev/null || cp -a ./. "${APP_DIR}/"
  ok "Código copiado do diretório atual."
elif [[ "${REPO_URL}" == *"SEU_USUARIO"* ]]; then
  warn "REPO_URL ainda está como exemplo. Como não achei código local, não posso clonar automaticamente."
  fail "Edite install.sh e troque REPO_URL pelo seu GitHub, ou rode o install.sh dentro da pasta do projeto."
else
  if [[ -d "${APP_DIR}/.git" ]]; then
    git -C "${APP_DIR}" pull --ff-only
  else
    rm -rf "${APP_DIR:?}/"*
    git clone "${REPO_URL}" "${APP_DIR}"
  fi
  ok "Código baixado do GitHub."
fi

info "[4/9] Compilando binário Go..."
cd "${APP_DIR}"
go build -ldflags="-w -s" -o "${BIN_PATH}" ./cmd/checkuserdt
chmod +x "${BIN_PATH}"
ok "Binário instalado em ${BIN_PATH}."

info "[5/9] Criando serviço systemd ${SERVICE_NAME}..."
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
ok "Serviço criado em ${SERVICE_PATH}."

info "[6/9] Instalando painel de terminal: comando chk..."
cat > "${CHK_PATH}" <<'CHK'
#!/usr/bin/env bash
SERVICE_NAME="checkuser555.service"
PORT="555"
LOCAL="http://127.0.0.1:${PORT}"
PUBLIC_IP=$(curl -4 -s --max-time 3 https://api.ipify.org || hostname -I | awk '{print $1}')
PUBLIC="http://${PUBLIC_IP}:${PORT}"
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'

pause(){ echo; read -rp "Pressione ENTER para voltar..." _; }
header(){ clear; echo -e "${BLUE}========== CHECKUSERDT / PORTA 555 ==========${NC}"; echo "Link do app: ${PUBLIC}/check/USUARIO_OU_UUID"; echo; }

while true; do
  header
  echo "1) Status do serviço"
  echo "2) Testar health"
  echo "3) Consultar usuário ou UUID"
  echo "4) Listar usuários Xray"
  echo "5) Ver stats por usuário ou UUID"
  echo "6) Ver logs ao vivo"
  echo "7) Reiniciar serviço"
  echo "8) Parar serviço"
  echo "9) Iniciar serviço"
  echo "10) Mostrar links para colocar no app"
  echo "0) Sair"
  echo
  read -rp "Escolha: " op
  case "$op" in
    1) systemctl status "$SERVICE_NAME" -l --no-pager; pause ;;
    2) curl -sS "${LOCAL}/health" | python3 -m json.tool 2>/dev/null || curl -sS "${LOCAL}/health"; pause ;;
    3) read -rp "Digite usuário ou UUID: " q; curl -sS "${LOCAL}/check/${q}" | python3 -m json.tool 2>/dev/null || curl -sS "${LOCAL}/check/${q}"; pause ;;
    4) curl -sS "${LOCAL}/users" | python3 -m json.tool 2>/dev/null || curl -sS "${LOCAL}/users"; pause ;;
    5) read -rp "Digite usuário ou UUID: " q; curl -sS "${LOCAL}/stats/${q}" | python3 -m json.tool 2>/dev/null || curl -sS "${LOCAL}/stats/${q}"; pause ;;
    6) echo "CTRL+C para sair dos logs"; journalctl -u "$SERVICE_NAME" -f ;;
    7) systemctl restart "$SERVICE_NAME" && echo -e "${GREEN}Reiniciado.${NC}"; pause ;;
    8) systemctl stop "$SERVICE_NAME" && echo -e "${YELLOW}Parado.${NC}"; pause ;;
    9) systemctl start "$SERVICE_NAME" && echo -e "${GREEN}Iniciado.${NC}"; pause ;;
    10) echo; echo "Link principal para o app:"; echo "${PUBLIC}/check/USUARIO_OU_UUID"; echo; echo "Exemplos:"; echo "${PUBLIC}/check/isja"; echo "${PUBLIC}/check/cb4c6db6-0712-42ca-a9ae-976fbc7b6041"; echo; echo "Painel web simples:"; echo "${PUBLIC}/panel"; pause ;;
    0) exit 0 ;;
    *) echo -e "${RED}Opção inválida.${NC}"; sleep 1 ;;
  esac
done
CHK
chmod +x "${CHK_PATH}"
ok "Comando chk instalado."

info "[7/9] Recarregando systemd..."
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}" >/dev/null
ok "Serviço habilitado no boot."

info "[8/9] Iniciando ${SERVICE_NAME}..."
systemctl restart "${SERVICE_NAME}"
sleep 1
systemctl is-active --quiet "${SERVICE_NAME}" || { systemctl status "${SERVICE_NAME}" -l --no-pager; fail "Serviço não iniciou."; }
ok "Serviço rodando na porta ${PORT}."

info "[9/9] Testando endpoint local..."
if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null; then
  ok "Health OK."
else
  warn "Serviço iniciou, mas o /health não respondeu agora. Veja: journalctl -u ${SERVICE_NAME} -f"
fi

PUBLIC_IP=$(curl -4 -s --max-time 3 https://api.ipify.org || hostname -I | awk '{print $1}')
APP_LINK="http://${PUBLIC_IP}:${PORT}/check/USUARIO_OU_UUID"
PANEL_LINK="http://${PUBLIC_IP}:${PORT}/panel"

cat <<FINAL

============================================================
${GREEN}INSTALAÇÃO CONCLUÍDA${NC}
============================================================
Serviço: ${SERVICE_NAME}
Pasta:   ${APP_DIR}
Porta:   ${PORT}

Link para colocar no app:
${YELLOW}${APP_LINK}${NC}

Painel web simples:
${YELLOW}${PANEL_LINK}${NC}

Comando do painel no terminal:
${YELLOW}chk${NC}

Testes:
curl http://127.0.0.1:${PORT}/health
curl http://127.0.0.1:${PORT}/check/USUARIO_OU_UUID

Se for acessar de fora, libere TCP ${PORT} no firewall/security group.
============================================================
FINAL
