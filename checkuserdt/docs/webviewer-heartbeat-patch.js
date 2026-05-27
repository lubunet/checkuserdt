
    /*
     * CHECKUSERDT ONLINE / HEARTBEAT
     * Troque somente o endereco abaixo para o IP ou dominio da sua VPS.
     * Nao use 127.0.0.1 aqui, porque no celular 127.0.0.1 aponta para o proprio aparelho.
     */
    const CHECKUSERDT_BASE_URL = "http://SEU_IP_DA_VPS:555";
    const DTUNNEL_APP_ID = "2a5fd843-98bc-4c33-810c-2803ebbfe654";

    let checkuserdtHeartbeatTimer = null;
    let checkuserdtLastVpnState = "";
    let checkuserdtConnectedAt = 0;
    let checkuserdtLastSentAt = 0;

    function getCurrentAuthIdentity() {
      const uuid = sanitizeCredentialValue((uuidInput && uuidInput.value) || dtGetUUID() || "");
      const username = sanitizeCredentialValue((usernameInput && usernameInput.value) || dtGetUsername() || "");

      if (uuid && isValidUuidValue(uuid)) {
        return { uuid, username: "" };
      }

      return { uuid: "", username };
    }

    function getCurrentTelemetryConfig() {
      try {
        const nativeConfigId = getNativeSelectedConfigId();
        const cfg = selectedConfig || (nativeConfigId ? findConfigById(nativeConfigId) : null) || getNativeDefaultConfig() || null;

        if (!cfg) {
          return {
            config_id: nativeConfigId ? String(nativeConfigId) : "",
            config_name: "",
            config_category: "Sem categoria"
          };
        }

        const enriched = enrichConfigWithNativeDefaults(cfg);

        return {
          config_id: enriched?.id != null ? String(enriched.id) : "",
          config_name: enriched?.name || "",
          config_category: enriched?.categoryName || enriched?.category || "Sem categoria"
        };
      } catch (e) {
        return { config_id: "", config_name: "", config_category: "Sem categoria" };
      }
    }

    function buildCheckUserDTPayload(stateOverride) {
      const state = normalizeVpnState(stateOverride || dtVpnState());
      const identity = getCurrentAuthIdentity();
      const cfg = getCurrentTelemetryConfig();
      const networkData = dtGetNetworkData();

      let networkType = "";
      let networkExtra = "";

      if (networkData && typeof networkData === "object") {
        networkType = String(networkData.type_name || networkData.type || "");
        networkExtra = String(networkData.extra_info || networkData.detailed_state || networkData.reason || "");
      }

      if (state === "CONNECTED" && !checkuserdtConnectedAt) {
        checkuserdtConnectedAt = Date.now();
      }

      if (!["CONNECTED", "CONNECTING", "AUTH"].includes(state)) {
        checkuserdtConnectedAt = 0;
      }

      return {
        dtunnel_id: DTUNNEL_APP_ID,
        username: identity.username,
        uuid: identity.uuid,
        vpn_state: state,
        config_id: cfg.config_id,
        config_name: cfg.config_name,
        config_category: cfg.config_category,
        local_ip: dtGetLocalIP(),
        network_name: dtGetNetworkName(),
        network_type: networkType,
        network_extra_info: networkExtra,
        download_bytes: Number(dtGetNetworkDownloadBytes() || 0),
        upload_bytes: Number(dtGetNetworkUploadBytes() || 0),
        connected_at: checkuserdtConnectedAt,
        app_version: appGetConfigVersion(),
        collected_at: Date.now()
      };
    }

    function sendCheckUserDTHeartbeat(stateOverride, forceSend = false) {
      const state = normalizeVpnState(stateOverride || dtVpnState());
      const activeStates = ["CONNECTED", "CONNECTING", "AUTH"];
      const inactiveStates = ["DISCONNECTED", "STOPPED", "STOPPING", "AUTH_FAILED", "NO_NETWORK", "ERROR"];

      const identity = getCurrentAuthIdentity();
      if (!identity.username && !identity.uuid) {
        return;
      }

      const now = Date.now();

      if (!forceSend && activeStates.includes(state) && now - checkuserdtLastSentAt < 12000) {
        return;
      }

      if (!forceSend && inactiveStates.includes(state) && checkuserdtLastVpnState === state) {
        return;
      }

      const payload = buildCheckUserDTPayload(state);
      checkuserdtLastVpnState = state;
      checkuserdtLastSentAt = now;

      fetch(CHECKUSERDT_BASE_URL.replace(/\/+$/, "") + "/telemetry/heartbeat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
        cache: "no-store",
        keepalive: true
      }).catch(() => {});
    }

    function startCheckUserDTTelemetry() {
      if (checkuserdtHeartbeatTimer) {
        clearInterval(checkuserdtHeartbeatTimer);
      }

      sendCheckUserDTHeartbeat(dtVpnState(), true);

      checkuserdtHeartbeatTimer = setInterval(() => {
        const state = normalizeVpnState(dtVpnState());

        if (state !== checkuserdtLastVpnState) {
          sendCheckUserDTHeartbeat(state, true);
          return;
        }

        if (["CONNECTED", "CONNECTING", "AUTH"].includes(state)) {
          sendCheckUserDTHeartbeat(state, false);
        }
      }, 5000);

      window.addEventListener("beforeunload", () => {
        sendCheckUserDTHeartbeat("DISCONNECTED", true);
      });

      document.addEventListener("visibilitychange", () => {
        if (document.visibilityState === "visible") {
          sendCheckUserDTHeartbeat(dtVpnState(), true);
        }
      });
    }
