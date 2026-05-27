# Usuarios online no `chk`

A opcao 5 do painel `chk` depende de dados enviados pelo WebView/app.

O servidor CheckUserDT consegue validar usuario/UUID sozinho, mas nao consegue descobrir sozinho:

- categoria/config do app;
- IP local do aparelho;
- operadora, Wi-Fi ou dados moveis;
- estado real da VPN no aparelho;
- bytes de upload/download do aparelho.

Esses dados existem na bridge do DTunnelSDK. O SDK expõe `getConfigs()`, `getDefaultConfig()`, `getUsername()`, `getUuid()`, `getLocalIp()`, `getNetworkName()`, `getNetworkData()`, `getNetworkDownloadBytes()` e `getNetworkUploadBytes()`.

Para ativar a opcao 5 com dados completos, cole o conteudo de `docs/dtunnel-heartbeat-example.js` no WebView/HTML do app e troque:

```js
const CHECKUSER_BASE = 'http://IP_DA_VPS:555';
```

pelo IP real da VPS.
