// Exemplo para colocar no WebView/HTML do DTunnel.
// Ele envia dados tecnicos para o CheckUserDT, permitindo a opcao "Usuarios online" no chk.
// Troque CHECKUSER_URL pelo link exibido na instalacao, exemplo: http://SEU_IP:555

const CHECKUSER_URL = 'http://SEU_IP:555';
const DTUNNEL_ID = '2a5fd843-98bc-4c33-810c-2803ebbfe654';

const sdk = new window.DTunnelSDK({
  strict: false,
  autoRegisterNativeEvents: true,
});

let connectedAt = null;
let lastConfigId = null;
let lastConfigName = null;

function detectConfigName(configs, id) {
  if (!Array.isArray(configs)) return null;
  for (const category of configs) {
    const items = category?.items || category?.configs || [];
    for (const item of items) {
      if (String(item?.id) === String(id)) return item?.name || item?.title || null;
    }
  }
  return null;
}

function readSnapshot() {
  const uuid = sdk.config.getUuid?.() || null;
  const username = sdk.config.getUsername?.() || null;
  const networkData = sdk.android.getNetworkData?.() || null;
  const configs = sdk.config.getConfigs?.() || null;

  if (!lastConfigId) {
    const def = sdk.config.getDefaultConfig?.() || null;
    lastConfigId = def?.id || def?.config_id || null;
    lastConfigName = def?.name || def?.title || detectConfigName(configs, lastConfigId);
  }

  return {
    dtunnel_id: DTUNNEL_ID,
    username,
    uuid,
    device_id: sdk.android.getDeviceId?.() || null,
    vpn_state: sdk.main.getVpnState?.() || null,
    local_ip: sdk.main.getLocalIp?.() || null,
    network_name: sdk.main.getNetworkName?.() || null,
    network_type: networkData?.type_name || networkData?.type || null,
    network_extra_info: networkData?.extra_info || networkData?.detailed_state || null,
    operator: sdk.main.getNetworkName?.() || null,
    config_id: lastConfigId,
    config_name: lastConfigName,
    download_bytes: sdk.android.getNetworkDownloadBytes?.() || 0,
    upload_bytes: sdk.android.getNetworkUploadBytes?.() || 0,
    app_version: sdk.android.getAppVersion?.() || null,
    connected_at: connectedAt,
  };
}

async function sendHeartbeat() {
  try {
    const snapshot = readSnapshot();
    if (!snapshot.username && !snapshot.uuid) return;

    await fetch(`${CHECKUSER_URL}/telemetry/heartbeat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(snapshot),
    });
  } catch (error) {
    console.log('heartbeat error', error);
  }
}

sdk.on('vpnState', (event) => {
  const state = event.payload;
  if (state === 'CONNECTED' && !connectedAt) connectedAt = Date.now();
  if (state === 'DISCONNECTED' || state === 'NO_NETWORK' || state === 'AUTH_FAILED') connectedAt = null;
  sendHeartbeat();
});

sdk.on('newDefaultConfig', () => {
  const def = sdk.config.getDefaultConfig?.() || null;
  lastConfigId = def?.id || def?.config_id || lastConfigId;
  lastConfigName = def?.name || def?.title || lastConfigName;
  sendHeartbeat();
});

setInterval(sendHeartbeat, 30000);
sendHeartbeat();
