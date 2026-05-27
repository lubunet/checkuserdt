/*
  CheckUserDT - Heartbeat para opcao 5 (Usuarios online)
  Cole este script no HTML/WebView do seu app DTunnel, ajustando CHECKUSER_BASE.

  O servidor sozinho NAO consegue descobrir config, categoria, IP local, operadora ou Wi-Fi.
  Esses dados existem no WebView pela bridge do DTunnelSDK e precisam ser enviados para:
  POST /telemetry/heartbeat
*/

const CHECKUSER_BASE = 'http://IP_DA_VPS:555';
const DTUNNEL_ID = '2a5fd843-98bc-4c33-810c-2803ebbfe654';

function safeCall(objectName, methodName, ...args) {
  try {
    const obj = window[objectName];
    if (!obj || typeof obj[methodName] !== 'function') return null;
    return obj[methodName](...args);
  } catch (_) {
    return null;
  }
}

function parseMaybeJson(value) {
  if (value == null) return null;
  if (typeof value !== 'string') return value;
  try { return JSON.parse(value); } catch (_) { return value; }
}

function getConfigInfo() {
  const configs = parseMaybeJson(safeCall('DtGetConfigs', 'execute')) || [];
  const def = parseMaybeJson(safeCall('DtGetDefaultConfig', 'execute')) || null;
  const savedId = Number(localStorage.getItem('checkuserdt_config_id') || 0);
  const configId = savedId || (def && def.id) || 0;

  let configName = def && def.name ? def.name : '';
  let categoryName = '';

  if (Array.isArray(configs)) {
    for (const category of configs) {
      const items = Array.isArray(category.items) ? category.items : [];
      for (const item of items) {
        if (Number(item.id) === Number(configId)) {
          configName = item.name || configName;
          categoryName = category.name || categoryName;
        }
      }
      if (!categoryName && def && Number(category.id) === Number(def.category_id)) {
        categoryName = category.name || '';
      }
    }
  }

  return {
    config_id: configId ? String(configId) : '',
    config_name: configName || '',
    config_category: categoryName || 'Sem categoria'
  };
}

function getNetworkInfo() {
  const networkData = parseMaybeJson(safeCall('DtGetNetworkData', 'execute')) || {};
  const networkName = safeCall('DtGetNetworkName', 'execute') || '';

  return {
    network_name: networkName || networkData.extra_info || networkData.type_name || '',
    network_type: networkData.type_name || networkData.type || '',
    network_extra_info: networkData.extra_info || '',
    operator: networkName || networkData.extra_info || ''
  };
}

function collectSnapshot() {
  const config = getConfigInfo();
  const net = getNetworkInfo();

  return {
    dtunnel_id: DTUNNEL_ID,
    username: safeCall('DtUsername', 'get') || '',
    uuid: safeCall('DtUuid', 'get') || '',
    device_id: safeCall('DtGetDeviceID', 'execute') || '',
    vpn_state: safeCall('DtGetVpnState', 'execute') || '',
    local_ip: safeCall('DtGetLocalIP', 'execute') || '',
    app_version: safeCall('DtAppVersion', 'execute') || '',
    download_bytes: Number(safeCall('DtGetNetworkDownloadBytes', 'execute') || 0),
    upload_bytes: Number(safeCall('DtGetNetworkUploadBytes', 'execute') || 0),
    connected_at: Number(localStorage.getItem('checkuserdt_connected_at') || 0),
    ...config,
    ...net
  };
}

function sendHeartbeat() {
  const payload = collectSnapshot();
  if (!payload.username && !payload.uuid) return;

  fetch(`${CHECKUSER_BASE}/telemetry/heartbeat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    keepalive: true
  }).catch(() => {});
}

// Guarda tempo inicial quando a VPN conecta.
const oldVpnEvent = window.DtVpnStateEvent;
window.DtVpnStateEvent = function (state) {
  if (state === 'CONNECTED' && !localStorage.getItem('checkuserdt_connected_at')) {
    localStorage.setItem('checkuserdt_connected_at', String(Date.now()));
  }
  if (state === 'DISCONNECTED' || state === 'AUTH_FAILED' || state === 'NO_NETWORK') {
    localStorage.removeItem('checkuserdt_connected_at');
  }
  sendHeartbeat();
  if (typeof oldVpnEvent === 'function') oldVpnEvent.apply(this, arguments);
};

// Guarda a config escolhida quando o app chamar DtSetConfig.execute(id).
if (window.DtSetConfig && typeof window.DtSetConfig.execute === 'function' && !window.DtSetConfig.__checkuserdtPatched) {
  const originalSetConfig = window.DtSetConfig.execute.bind(window.DtSetConfig);
  window.DtSetConfig.execute = function (id) {
    try { localStorage.setItem('checkuserdt_config_id', String(id)); } catch (_) {}
    const result = originalSetConfig(id);
    setTimeout(sendHeartbeat, 500);
    return result;
  };
  window.DtSetConfig.__checkuserdtPatched = true;
}

sendHeartbeat();
setInterval(sendHeartbeat, 30000);
