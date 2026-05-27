#!/usr/bin/env bash
set -euo pipefail
SERVICE_NAME="checkuser555.service"
systemctl stop "$SERVICE_NAME" 2>/dev/null || true
systemctl disable "$SERVICE_NAME" 2>/dev/null || true
rm -f "/etc/systemd/system/${SERVICE_NAME}"
rm -f /usr/local/bin/checkuserdt /usr/local/bin/chk
systemctl daemon-reload
printf '\nRemovido. A pasta /root/checkuserdt foi preservada para não apagar dados.\n'
